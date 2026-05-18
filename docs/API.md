# Backend API

This document describes the API implemented by the current Go backend. Routes are shown exactly as registered in the Gin router. If production exposes the backend behind a reverse proxy path such as `/api`, prepend that proxy prefix in the frontend base URL.

## Base URL

Local/direct backend:

```text
http://localhost:9001
```

Production example if proxied under `/api`:

```text
https://llm.pphui8.com/api
```

Current CORS configuration allows:

```text
https://llm.pphui8.com
```

with credentials enabled.

## Authentication Model

The backend uses two tokens:

- Access token: returned in JSON by `/login` and `/refresh`; send it in the `Authorization` header for protected endpoints.
- Refresh token: stored by the backend as an HttpOnly cookie named `refresh_token`; the frontend cannot read it directly, but the browser sends it when requests use credentials.

Protected request header:

```http
Authorization: Bearer <access_token>
```

Frontend `fetch` calls that need cookies, especially `/login` and `/refresh`, should include:

```js
credentials: "include"
```

Recommended frontend flow:

1. Call `POST /login` with username/password.
2. Store the returned `access_token` in frontend memory.
3. Use `Authorization: Bearer <access_token>` for protected API calls.
4. If a protected call returns `401`, call `POST /refresh` with `credentials: "include"`.
5. Replace the in-memory access token with the new one from `/refresh`.
6. Retry the original request once.
7. If refresh fails, return the user to the login screen.

Avoid storing the access token in `localStorage` unless you accept the XSS risk. In-memory storage is enough for this personal-use app if the frontend refreshes on page reload.

## Error Shape

Most JSON errors use:

```json
{
  "error": "message"
}
```

Some endpoints return raw arrays on success. The streaming endpoint sends errors as SSE events after streaming has started.

## Public Endpoints

### `GET /ping`

Health check for Redis and Postgres.

Authentication: not required.

Success response:

```json
{
  "message": "pong",
  "status": {
    "redis": "up",
    "postgres": "up"
  }
}
```

Failure response status: `500`

Example failure response:

```json
{
  "message": "service unavailable",
  "status": {
    "redis": "down",
    "redis_error": "connection refused",
    "postgres": "up"
  }
}
```

### `POST /login`

Authenticates an existing user.

Authentication: not required.

Request:

```json
{
  "username": "alice",
  "password": "password"
}
```

Success response:

```json
{
  "access_token": "eyJ..."
}
```

Side effect:

- Sets an HttpOnly cookie named `refresh_token`.

Common error responses:

| Status | Meaning |
| :--- | :--- |
| `400` | Invalid or missing JSON fields. |
| `401` | Invalid username or password. |
| `500` | Database, token generation, or server error. |

Frontend example:

```js
export async function login(baseUrl, username, password) {
  const res = await fetch(`${baseUrl}/login`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });

  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "Login failed");
  return data.access_token;
}
```

### `POST /refresh`

Rotates the refresh token and returns a new access token.

Authentication: refresh cookie required.

Request body: none.

Success response:

```json
{
  "access_token": "eyJ..."
}
```

Side effect:

- Replaces the `refresh_token` HttpOnly cookie.

Common error responses:

| Status | Meaning |
| :--- | :--- |
| `401` | Missing, invalid, expired, reused, or inactive refresh token. |
| `500` | Token generation or server error. |

Frontend example:

```js
export async function refreshAccessToken(baseUrl) {
  const res = await fetch(`${baseUrl}/refresh`, {
    method: "POST",
    credentials: "include",
  });

  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "Refresh failed");
  return data.access_token;
}
```

## Protected Endpoints

All endpoints in this section require:

```http
Authorization: Bearer <access_token>
```

### `GET /resource`

Simple endpoint for testing access-token authentication.

Success response:

```json
{
  "message": "Welcome to the protected resource!",
  "user": "alice"
}
```

Common error responses:

| Status | Meaning |
| :--- | :--- |
| `401` | Missing, malformed, invalid, or expired access token. |

### `GET /conversations`

Lists conversations owned by the authenticated user.

Success response:

```json
[
  {
    "id": 1,
    "username": "alice",
    "title": "Where is Kyoto located?",
    "summary": "",
    "created_at": "2026-05-18T10:00:00Z"
  }
]
```

Notes:

- Results are ordered by `created_at DESC`.
- There is currently no pagination.

Common error responses:

| Status | Meaning |
| :--- | :--- |
| `401` | Missing or invalid access token. |
| `500` | Service initialization or database error. |

Frontend example:

```js
export async function getConversations(baseUrl, accessToken) {
  const res = await fetch(`${baseUrl}/conversations`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });

  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "Failed to fetch conversations");
  return data;
}
```

### `GET /conversations/:id/messages`

Lists messages in a conversation owned by the authenticated user.

Path parameters:

| Name | Type | Description |
| :--- | :--- | :--- |
| `id` | integer | Conversation ID. |

Success response:

```json
[
  {
    "id": 1,
    "conversation_id": 1,
    "role": "user",
    "content": "Where is Kyoto located?",
    "token_count": 0,
    "created_at": "2026-05-18T10:00:00Z"
  },
  {
    "id": 2,
    "conversation_id": 1,
    "role": "assistant",
    "content": "Kyoto is located in Japan...",
    "token_count": 0,
    "created_at": "2026-05-18T10:00:03Z"
  }
]
```

Notes:

- Results are ordered by `created_at ASC`.
- There is currently no pagination.
- If the conversation does not belong to the user, current code returns `500` with an error string rather than `403`.

Common error responses:

| Status | Meaning |
| :--- | :--- |
| `400` | Invalid conversation ID. |
| `401` | Missing or invalid access token. |
| `500` | Service initialization, database error, not found, or unauthorized conversation access. |

### `GET /conversations/:id/delete`

Deletes a conversation owned by the authenticated user.

Current implementation uses `GET` for deletion. Frontend should call this route only as an explicit user action.

Path parameters:

| Name | Type | Description |
| :--- | :--- | :--- |
| `id` | integer | Conversation ID. |

Success response:

```json
{
  "message": "Conversation deleted successfully"
}
```

Common error responses:

| Status | Meaning |
| :--- | :--- |
| `400` | Invalid conversation ID. |
| `401` | Missing or invalid access token. |
| `500` | Service initialization, database error, not found, or unauthorized conversation access. |

Frontend example:

```js
export async function deleteConversation(baseUrl, accessToken, conversationId) {
  const res = await fetch(`${baseUrl}/conversations/${conversationId}/delete`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });

  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "Failed to delete conversation");
  return data;
}
```

### `POST /gemini`

Streams a Gemini chat response for a new or existing conversation.

Response type:

```http
Content-Type: text/event-stream
```

Request for a new conversation:

```json
{
  "prompt": "Where is Kyoto located?"
}
```

Request for an existing conversation:

```json
{
  "conversation_id": 1,
  "prompt": "What is it famous for?"
}
```

SSE events:

Normal chunks are sent as default `message` events:

```text
data: Kyoto

data:  is located

data:  in Japan.

```

When streaming completes, the backend sends a named `done` event:

```text
event: done
data: {"conversation_id": 1}

```

If an error occurs after the stream has started, the backend sends:

```text
event: error
data: failed to generate streaming content: ...

```

Important frontend parsing notes:

- The backend streams plain text chunks as SSE `data:` lines.
- Multiline chunks are split into multiple `data:` lines according to the SSE format.
- The final `done` event data is JSON.
- `EventSource` cannot send custom `Authorization` headers, so use `fetch` streaming instead of `EventSource`.

Frontend streaming example:

```js
export async function streamGemini({
  baseUrl,
  accessToken,
  prompt,
  conversationId,
  onChunk,
  onDone,
}) {
  const res = await fetch(`${baseUrl}/gemini`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${accessToken}`,
    },
    body: JSON.stringify({
      prompt,
      ...(conversationId ? { conversation_id: conversationId } : {}),
    }),
  });

  if (!res.ok) {
    let message = "Gemini request failed";
    try {
      const data = await res.json();
      message = data.error || message;
    } catch {
      // Response may not be JSON.
    }
    throw new Error(message);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });

    let boundaryIndex;
    while ((boundaryIndex = buffer.indexOf("\n\n")) !== -1) {
      const rawEvent = buffer.slice(0, boundaryIndex);
      buffer = buffer.slice(boundaryIndex + 2);

      const event = parseSseEvent(rawEvent);
      if (!event) continue;

      if (event.event === "done") {
        onDone?.(JSON.parse(event.data));
      } else if (event.event === "error") {
        throw new Error(event.data || "Stream error");
      } else {
        onChunk?.(event.data);
      }
    }
  }
}

function parseSseEvent(rawEvent) {
  const lines = rawEvent.split("\n");
  let event = "message";
  const data = [];

  for (const line of lines) {
    if (line.startsWith("event:")) {
      event = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      data.push(line.slice("data:".length).replace(/^ /, ""));
    }
  }

  if (data.length === 0 && event === "message") return null;
  return { event, data: data.join("\n") };
}
```

Common error responses before streaming starts:

| Status | Meaning |
| :--- | :--- |
| `400` | Invalid or missing JSON fields. |
| `401` | Missing or invalid access token. |
| `500` | LLM service initialization error, database error, or unsupported streaming writer. |

Errors after streaming starts are sent as SSE `error` events because the HTTP status has already been committed.

## Shared Frontend Helper Pattern

A small wrapper can centralize access-token refresh:

```js
export function createApiClient(baseUrl) {
  let accessToken = null;

  async function refresh() {
    accessToken = await refreshAccessToken(baseUrl);
    return accessToken;
  }

  async function request(path, options = {}, retry = true) {
    const res = await fetch(`${baseUrl}${path}`, {
      ...options,
      credentials: options.credentials ?? "include",
      headers: {
        ...(options.body ? { "Content-Type": "application/json" } : {}),
        ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
        ...options.headers,
      },
    });

    if (res.status === 401 && retry) {
      await refresh();
      return request(path, options, false);
    }

    return res;
  }

  return {
    setAccessToken(token) {
      accessToken = token;
    },
    getAccessToken() {
      return accessToken;
    },
    request,
    refresh,
  };
}
```

Use a custom streaming function for `/gemini`, because retrying after a partially started stream is not safe.

## Current Backend Quirks To Account For

- There is no sign-up API. Users must already exist in Postgres.
- Access token is returned in JSON, not a cookie.
- Refresh token is HttpOnly and requires `credentials: "include"`.
- `/conversations/:id/delete` deletes with `GET`.
- `/gemini` streams plain text SSE data chunks, not JSON chunk payloads.
- There is no pagination for conversations or messages.
- Provider/model selection is not exposed; `/gemini` always uses the backend-configured Gemini model.
