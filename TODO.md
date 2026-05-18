# TODO

This backlog is based on the current repository state. The checkout contains the Go backend, deployment scripts, docs, and a helper tool; the React frontend described in `docs/architecture.md` is not present here, so frontend-specific issues need a separate review of `long-web`.

## P0 - Security And Correctness

- [x] Actually use the configured Redis password and DB number.
  - `cmd/long/main.go` now passes `config.Redis.Password` and `config.Redis.DB` into `auth.InitRedis`.

- [x] Make Redis initialization fail fast when Redis is unavailable.
  - `auth.InitRedis` now returns the Redis ping error, closes the failed client, and only assigns `GlobalTokenStore` after a successful connection.
  - `cmd/long/main.go` now treats Redis initialization failure as fatal startup failure.

- [x] Fix refresh token lifetime mismatch.
  - `auth.RefreshTokenTTL` now defines the shared 7-day lifetime for the refresh JWT, refresh cookie, revoked-token TTL, and active-token TTL.

- [ ] Make refresh token rotation atomic.
  - `handler/auth.go:99-134` checks revocation, checks active token, revokes old token, and registers the new token as separate Redis operations.
  - Concurrent refresh requests can race and produce inconsistent results. Use a Redis transaction or Lua script for compare-and-swap rotation.

- [ ] Return correct HTTP statuses for authorization failures.
  - `service/llm.go:56`, `service/llm.go:69`, `service/llm.go:97`, and `service/llm.go:178` detect unauthorized conversation access, but handlers convert those errors into `500`.
  - Add typed/domain errors and map unauthorized access to `403`, missing conversations to `404`, validation failures to `400`, and true server failures to `500`.

- [ ] Change conversation deletion to a proper HTTP method.
  - `router/router.go:31` deletes data via `GET /conversations/:id/delete`.
  - Use `DELETE /conversations/:id` to avoid accidental deletion by crawlers, browser prefetching, or copied links.

- [ ] Escape or encode SSE chunks correctly.
  - `handler/gemini.go:51-65` writes raw chunks into `data: ...` frames. Newlines or special content can break event framing.
  - Emit JSON-encoded payloads or split multiline SSE data according to the SSE spec.

## P1 - Architecture And Code Structure

- [ ] Introduce an application container and dependency injection.
  - Handlers currently construct repositories and `LLMService` per request (`handler/gemini.go:31-32`, `handler/gemini.go:76-77`, `handler/gemini.go:113-114`, `handler/gemini.go:150-151`).
  - Build DB, Redis token store, repositories, and LLM clients once at startup, then pass them into handlers through a struct.

- [ ] Remove global mutable singletons.
  - `db.Instance`, `auth.GlobalTokenStore`, and `logger.Log` make tests harder and couple unrelated packages to process-wide state.
  - Prefer explicit dependencies: `App{DB, TokenStore, Logger, LLMService}`.

- [ ] Split config loading into a dedicated package.
  - `cmd/long/main.go:39-53` hardcodes `env.yaml` and only supports YAML.
  - Add `config.Load()` with environment override support, validation, defaults for non-secret values, and a configurable config path.

- [ ] Separate provider-agnostic chat logic from Gemini-specific provider code.
  - `handler.HandleGemini`, `GEMINI_API`, and `googleai.WithDefaultModel("gemini-3.1-flash-lite")` are hardcoded in request flow.
  - For a transit-station style app that can access multiple LLM platforms, define provider interfaces such as `ChatProvider`, `ModelRegistry`, and `ProviderConfig`.

- [ ] Make model/provider selection part of the request and config.
  - `domain.LLMRequest` only has `conversation_id` and `prompt`; no provider, model, temperature, system prompt, or tool options.
  - Add a minimal explicit model/provider contract before adding more platforms.

- [ ] Reduce duplicated chat flow.
  - `service.ProcessPrompt` and `service.StreamPrompt` duplicate conversation creation, ownership checks, message persistence, and history construction.
  - Extract shared helpers or make streaming the primary path and adapt non-streaming to it.

- [ ] Add transaction boundaries around chat persistence.
  - `service/llm.go:161-229` creates conversations and saves the user message before the provider call. If streaming fails, the DB may contain a user message with no assistant response.
  - Decide the desired behavior and make it explicit: transaction, pending/failed assistant message state, or retryable job record.

- [ ] Introduce a prompt/history policy.
  - `service/llm.go:193-208` sends the full conversation history every time with no token budget, summarization, truncation, or message count limit.
  - Add a policy for max context, summarization, and provider-specific token counting.

- [ ] Move SQL schema management out of `.github/scripts`.
  - `.github/scripts/database.SQL` is not an application migration system.
  - Add migrations with a tool such as goose, golang-migrate, or a small internal migration runner; run migrations in deployment or startup with clear ownership.

## P1 - API And Data Model

- [ ] Version the API routes.
  - Current routes are top-level (`/login`, `/gemini`, `/conversations`), while docs mention reverse proxy path `/api`.
  - Consider grouping backend routes under `/api/v1` and keeping proxy behavior consistent with docs.

- [ ] Normalize response shapes.
  - Some endpoints return raw slices, some return `{message: ...}`, and stream errors are raw text events.
  - Define consistent JSON envelopes or a documented response contract for success, validation errors, auth errors, and provider errors.

- [ ] Validate request sizes and prompt content.
  - `domain.LLMRequest.Prompt` is only `binding:"required"`.
  - Add max length/body-size limits, trim rules, and clear handling for empty or whitespace-only prompts.

- [ ] Add pagination for conversations and messages.
  - `repository/llm.go:45` and `repository/llm.go:70` return all rows.
  - Add `limit`, `cursor`/`before`, and indexes that match expected ordering.

- [ ] Add `updated_at` or `last_message_at` to conversations.
  - Conversations are ordered by `created_at` only, so active conversations will not move to the top after new messages.

- [ ] Add DB constraints for roles and ownership assumptions.
  - `.github/scripts/database.SQL:25` documents valid roles in a comment only.
  - Add a check constraint for `role IN ('system', 'user', 'assistant')`, `NOT NULL` on required foreign keys, and indexes for common queries.

- [ ] Store provider metadata with messages.
  - The schema has only `role`, `content`, and `token_count`.
  - Add provider/model, finish reason, latency, token usage, and error state if multi-platform usage matters.

## P1 - Reliability And Operations

- [ ] Configure HTTP server timeouts.
  - `cmd/long/main.go:34` uses `r.Run`, which uses default server settings.
  - Create an `http.Server` with read/write/header timeouts and graceful shutdown handling.

- [ ] Add graceful shutdown for DB, Redis, and in-flight streams.
  - Current startup never closes `db.Instance` or Redis and does not handle SIGTERM.
  - This matters for Docker deployment and long-running SSE responses.

- [ ] Add structured request IDs.
  - Logs include path/IP but not a request ID that can tie handler logs, provider calls, DB operations, and client errors together.

- [ ] Avoid logging sensitive operational detail to clients.
  - Several handlers return `err.Error()` directly (`handler/gemini.go:59`, `handler/gemini.go:87`, `handler/gemini.go:124`, `handler/gemini.go:161`).
  - Log detailed errors server-side and return stable client-facing error codes/messages.

- [ ] Make CORS environment-specific.
  - `router/router.go:39` hardcodes `https://llm.pphui8.com`.
  - Move allowed origins to config and support localhost during development without code changes.

- [ ] Add rate limiting or at least single-user abuse protection.
  - Even for personal use, LLM endpoints can spend money quickly if a token leaks.
  - Add per-user/IP request limits and provider timeout/cancel behavior.

## P2 - Testing And Quality

- [ ] Add unit tests for auth token behavior.
  - Cover missing/invalid audience, expiry, issuer, refresh token reuse, and required-secret startup validation.

- [ ] Add handler tests with mocked services.
  - Current handlers depend on globals and concrete DB/LLM construction, which makes them hard to test.
  - Dependency injection should make login, refresh, conversation ownership, and SSE tests straightforward.

- [ ] Add repository tests using a real Postgres test container or local test DB.
  - Cover conversation ordering, message ordering, deletion cascade, pagination, and unauthorized access scenarios.

- [ ] Replace the Gemini integration test with an explicit integration test tag.
  - `test/gemini/gemini_test.go` calls a real external API when `GEMINI_API` exists.
  - Use `//go:build integration` so normal `go test ./...` never spends external API quota accidentally.

- [ ] Add CI checks beyond Docker build.
  - `.github/workflows/go.yml` builds and pushes an image but does not run `go test ./...`, `go vet`, formatting checks, or static analysis.
  - Add these before image publishing.

- [ ] Add linting/static analysis.
  - Start with `gofmt`, `go vet`, and `staticcheck` if acceptable.
  - This would catch some unchecked errors and code health issues early.

## P2 - Documentation And Repository Hygiene

- [ ] Update `README.md`.
  - It currently says only "LLM implemetation for a specialized chatbot".
  - Add setup instructions, required environment variables, local run commands, migration steps, API examples, and deployment notes.

- [ ] Bring `docs/architecture.md` in sync with the repo.
  - It references `cmd/long`, Postgres, Redis, React frontend, and `/api` proxying, but the current checkout does not contain the frontend and router paths are not `/api`-prefixed.

- [ ] Add `.gitignore` coverage for logs and local config.
  - `log/logs/app.log` is currently tracked.
  - Ignore runtime logs, local config files, built binaries, and editor/system files.

- [ ] Replace `tool/hash.html` with a safer operational path or document it clearly.
  - A browser-based HMAC password hash helper can be useful for a personal app, but it should be documented as an admin-only local tool with key handling caveats.

- [ ] Revisit Go version and image choices.
  - `go.mod` and `Dockerfile` use Go `1.26.2`. Confirm this is intentional and supported by the deployment environment.
  - Consider pinning base images by digest for reproducible builds if deployment reliability matters.

## Frontend Review Needed

- [ ] Locate or add the React frontend repository/package.
  - No `package.json`, Vite/Next config, or frontend source is present in this checkout.
  - Review frontend auth storage, refresh handling, SSE parsing, route protection, error states, loading/cancel behavior, and chat transcript rendering once the frontend code is available.
