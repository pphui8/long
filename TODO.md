# TODO

This backlog is based on the current repository state. The checkout contains the Go backend, deployment scripts, docs, and a helper tool; the React frontend described in `docs/architecture.md` is not present here, so frontend-specific issues need a separate review of `long-web`.

## P0 - Security And Correctness


- [ ] Make refresh token rotation atomic.
  - `handler/auth.go:99-134` checks revocation, checks active token, revokes old token, and registers the new token as separate Redis operations.
  - Concurrent refresh requests can race and produce inconsistent results. Use a Redis transaction or Lua script for compare-and-swap rotation.

## P1 - Architecture And Code Structure
- [ ] Split config loading into a dedicated package.
  - `cmd/long/main.go:39-53` hardcodes `env.yaml` and only supports YAML.
  - Add `config.Load()` with environment override support, validation, defaults for non-secret values, and a configurable config path.

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
  - Current startup never closes the app DB connection or Redis client and does not handle SIGTERM.
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
