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
| `handler` | Application container and Gin HTTP handlers for auth, health checks, protected resource access, and Gemini chat/conversation APIs. |
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

Configured through environment variables:

- `GEMINI_API`: Gemini API key. Required by the Gemini provider configured at startup.
- `JWT_KEY`: HMAC key used to sign JWTs. If absent, current code generates a process-local random key.
- `PASSWORD_HASH`: HMAC key used to hash/verify passwords. If absent, current code uses a default fallback key.
- `GIN_MODE`: Gin mode, set to `release` in the deployment workflow.

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
- `idx_msg_conv_id` on `messages(conversation_id)`.

## Chat Flow

`POST /gemini` is the main chat endpoint.

For a new conversation:

1. The request omits `conversation_id`.
2. The service creates a conversation owned by the authenticated username.
3. The conversation title is derived from the prompt.
4. The user message is saved.
5. Full conversation history is loaded.
6. History is converted to LangChain message content.
7. The configured chat provider is called with streaming enabled.
8. Each streamed chunk is sent to the frontend as an SSE `data:` event.
9. The full assistant response is saved after streaming finishes.
10. A final SSE `done` event is sent with the conversation ID.

For an existing conversation:

1. The request includes `conversation_id`.
2. The service loads the conversation and verifies ownership.
3. The same message-save, history-load, stream, and assistant-save flow runs.

Current implementation details:

- Provider selection is wired at startup through a `ChatProvider`; the current default provider is Gemini with model `gemini-3.1-flash-lite`.
- The full conversation history is sent on every request.
- There is no pagination for message loading.
- If provider streaming fails after the user message is saved, the conversation may contain the user message without an assistant reply.

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
| `POST` | `/gemini` | `HandleChat` | Stream a chat response over SSE. |
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
- Publishes `9001:9001`.
- Adds `host.docker.internal` for host access from Linux Docker.
- Sets `GIN_MODE`, `GEMINI_API`, `JWT_KEY`, and `PASSWORD_HASH`.
