# Project Architecture

This document describes the architecture implemented by the current backend code in this repository. The React frontend and reverse proxy configuration are not included in this checkout, so those parts are described only as deployment expectations.

## Runtime Overview

The backend is a Go service built with Gin. It provides login, refresh-token rotation, health checking, a small protected resource endpoint, and provider-backed chat/conversation endpoints.

Expected deployment shape:

- Browser frontend talks to the backend over HTTPS.
- Nginx or another reverse proxy may sit in front of the backend.
- The Go backend listens on the port configured in `env.yaml` (`9001` by default).
- Postgres stores users, conversations, and messages.
- Redis stores refresh-token state for rotation/revocation.
- Chat generation is accessed through a provider interface; the current startup wires a Gemini provider using `GEMINI_API`.

Current router paths are not prefixed with `/api` in code. If production exposes the backend under `/api`, that prefix is added by the reverse proxy, not by the Gin router.

## Request Flow

1. A request reaches the Gin router from the frontend or reverse proxy.
2. Global middleware runs:
   - `logger.GinLogger(app.Logger)` writes structured access logs.
   - `gin.Recovery()` handles panics.
   - `router.CORSMiddleware()` allows `https://llm.pphui8.com` with credentials.
3. Public routes are handled directly.
4. Protected routes pass through `auth.AuthMiddleware()`.
5. Handlers use dependencies from `handler.App` and call the service layer.
6. Repositories use the startup-created Postgres connection.
7. Auth refresh-token state is stored through the startup-created Redis token store.
8. Chat requests call Gemini and persist conversation history.

## Package Responsibilities

| Package | Responsibility |
| :--- | :--- |
| `cmd/long` | Application entry point. Initializes logger, loads `env.yaml`, initializes Redis/Postgres/repositories/services, builds the Gin router, and starts the server. |
| `auth` | JWT generation/validation, password HMAC hashing, auth middleware, and Redis token store. |
| `db` | Opens and verifies the Postgres connection pool. |
| `domain` | Request/response structs, config structs, and database-facing domain models. |
| `handler` | Application container and Gin HTTP handlers for auth, health checks, protected resource access, and chat/conversation APIs. |
| `logger` | Zap logger construction and Gin request logging middleware. |
| `repository` | Postgres access for users, conversations, and messages. |
| `router` | Route registration and CORS middleware. |
| `provider` | Concrete chat provider adapters, currently Gemini through `langchaingo`. |
| `service` | Provider-agnostic chat business logic: conversation ownership checks, message persistence, history assembly, provider calls, and streaming. |
| `test/gemini` | Optional Gemini integration test. Skips when `GEMINI_API` is not set. |
| `tool` | Local helper tooling, currently a password hash HTML page. |

## Configuration

The backend reads `env.yaml` from the process working directory.

Configured in `env.yaml`:

- `app.port`: HTTP listen port.
- `redis.host`, `redis.port`, `redis.db`, `redis.password`: Redis connection settings.
- `postgres.host`, `postgres.port`, `postgres.user`, `postgres.password`, `postgres.dbname`, `postgres.sslmode`: Postgres connection settings.

The checked-in `env.yaml` uses `127.0.0.1` for Redis and Postgres because the deployment runs the Docker container with host networking.

Configured through environment variables:

- `GEMINI_API`: Gemini API key. Required by the Gemini provider configured at startup.
- `MCP_SERVER_URL`: Optional MCP streamable HTTP endpoint. Defaults to `http://127.0.0.1:9002/mcp`. The deployment runs the app container with host networking so this reaches the host MCP server.
- `TABILY_API_KEY`: Tavily/Tabily web search API key. When set, chat also enables the `web_search` tool. The code also accepts `TAVILY_API_KEY`.
- `JWT_KEY`: HMAC key used to sign JWTs. If absent, current code generates a process-local random key.
- `PASSWORD_HASH`: HMAC key used to hash/verify passwords. If absent, current code uses a default fallback key.
- `GIN_MODE`: Gin mode, set to `release` in the deployment workflow.
- `LOG_LEVEL`: Optional log level: `debug`, `info`, `warn`, or `error`. Defaults to `info`. Use `debug` to include provider decision previews and tool result previews.

## Authentication

Login uses a fixed user table in Postgres. There is no sign-up or user management flow.

Password verification:

- The submitted password is HMAC-SHA256 hashed with `PASSWORD_HASH`.
- The resulting hex digest is compared to `users.password_hash`.

JWTs:

- Access token:
  - Signed with HMAC-SHA256.
  - Audience: `long-api`.
  - Issuer: `long-server`.
  - Lifetime: 30 minutes.
  - Sent to the frontend in the JSON login/refresh response.
- Refresh token:
  - Signed with HMAC-SHA256.
  - Audience: `long-refresh`.
  - Issuer: `long-server`.
  - JWT lifetime: 7 days.
  - Stored in an HttpOnly cookie named `refresh_token`.
  - Cookie max age: 7 days.

Refresh-token rotation:

- On login, the refresh token JTI is registered in Redis as the active token for the user.
- On refresh, the backend validates the cookie JWT, checks whether the JTI was revoked, verifies it is still the active token, revokes the old JTI, generates a new token pair, and registers the new JTI.
- If a revoked token is reused, active sessions for the user are invalidated.

Protected routes require:

```http
Authorization: Bearer <access_token>
```

The middleware validates the token audience and stores the username in the Gin context.

## Data Model

The SQL schema is currently stored in `.github/scripts/database.SQL`.

### `users`

| Column | Type | Notes |
| :--- | :--- | :--- |
| `username` | `VARCHAR(50)` | Primary key. |
| `password_hash` | `VARCHAR(255)` | HMAC-SHA256 hex digest. |

### `conversations`

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `SERIAL` | Primary key. |
| `username` | `VARCHAR(50)` | References `users(username)`, cascades update/delete. |
| `title` | `VARCHAR(255)` | Defaults to `New Chat`. New chats use the first prompt, truncated to 50 characters. |
| `summary` | `TEXT` | Currently read but not written by service code. |
| `created_at` | `TIMESTAMPTZ` | Defaults to current timestamp. |
| `last_message_at` | `TIMESTAMPTZ` | Defaults to current timestamp and is refreshed whenever a message is saved. |

### `messages`

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `SERIAL` | Primary key. |
| `conversation_id` | `INT` | References `conversations(id)`, cascades delete. |
| `role` | `VARCHAR(20)` | Expected values: `system`, `user`, `assistant`. |
| `content` | `TEXT` | Message text. |
| `token_count` | `INT` | Inserted as zero by current service code. |
| `created_at` | `TIMESTAMPTZ` | Defaults to current timestamp. |

Indexes:

- `idx_conv_username` on `conversations(username)`.
- `idx_conv_username_last_message_at` on `conversations(username, last_message_at DESC)`.
- `idx_msg_conv_id` on `messages(conversation_id)`.

## Chat Flow

`POST /chat` is the main chat endpoint. The request body selects the model with the required `model` field.

Available models:

| Model | Provider | Backend model | Registration |
| :--- | :--- | :--- | :--- |
| `gemini` | Gemini | `gemini-3.1-flash-lite` | Registered in `cmd/long/main.go` through `NewLLMServiceWithProviders`. |

For a new conversation:

1. The request omits `conversation_id`.
2. The requested model is resolved to a configured chat provider.
3. The service creates a conversation owned by the authenticated username.
4. The conversation title is derived from the prompt.
5. The user message is saved.
6. Full conversation history is loaded.
7. History is prepared for the LLM engine.
8. The configured chat provider is called through the LLM engine. At startup the engine discovers tools from the configured MCP server, and when `TABILY_API_KEY` or `TAVILY_API_KEY` is set it can also call the `web_search` tool before streaming the final answer.
9. Each streamed chunk is sent to the frontend as an SSE `data:` event.
10. The full assistant response is saved after streaming finishes.
11. A final SSE `done` event is sent with the conversation ID.

For an existing conversation:

1. The request includes `conversation_id`.
2. The service loads the conversation and verifies ownership.
3. The same message-save, history-load, stream, and assistant-save flow runs.

Current implementation details:

- Provider selection is request-driven through `domain.LLMRequest.Model` and resolved against startup-registered `ChatProvider` instances.
- The full conversation history is sent on every request.
- There is no pagination for message loading.
- Conversation and user-message persistence happens before provider streaming; the assistant message is saved after streaming succeeds.

## HTTP Routes

Public:

| Method | Path | Handler | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/login` | `HandleLogin` | Validate username/password, issue access token, set refresh cookie. |
| `POST` | `/refresh` | `HandleRefresh` | Rotate refresh token and issue a new access token. |
| `GET` | `/ping` | `HandlePing` | Check Redis and Postgres availability. |

Protected:

| Method | Path | Handler | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/resource` | `HandleResource` | Simple protected resource test. |
| `POST` | `/chat` | `HandleChat` | Stream a chat response over SSE. |
| `GET` | `/conversations` | `HandleGetConversations` | List authenticated user's conversations. |
| `GET` | `/conversations/:id/messages` | `HandleGetMessages` | List messages for a conversation owned by the authenticated user. |
| `GET` | `/conversations/:id/delete` | `HandleDeleteConversation` | Delete a conversation owned by the authenticated user. |

See `API.md` for frontend integration details.

## Deployment

`Dockerfile` builds a static Go binary and runs it in Alpine:

- Build stage: `golang:1.26.2-alpine`.
- Runtime stage: `alpine:3.19`.
- Exposes port `9001`.
- Copies `env.yaml` into the image.

`.github/workflows/go.yml` builds and pushes an image to GitHub Container Registry, then deploys over SSH on pushes to `main`.

The deployment command:

- Runs the container as `long`.
- Uses host networking so the app can reach host services on `127.0.0.1`, including the MCP server on port `9002`.
- Sets `GIN_MODE`, `GEMINI_API`, `TABILY_API_KEY`, `JWT_KEY`, `PASSWORD_HASH`, and `MCP_SERVER_URL`.
- Sets `MCP_SERVER_URL=http://127.0.0.1:9002/mcp` in the deploy command. Port `9002` does not need to be exposed by this app container because MCP traffic is outbound.
