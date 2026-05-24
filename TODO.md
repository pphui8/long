

## P1 - Architecture And Code Structure

- [x] Make model/provider selection part of the request and config.
  - `domain.LLMRequest` now includes `model`; no temperature, system prompt, or tool options yet.
  - Add more providers by registering more `ChatProvider` instances at startup.

## P1 - API And Data Model

- [x] Add `updated_at` or `last_message_at` to conversations.
  - Conversations now expose `last_message_at`, refresh it when messages are saved, and list by recent message activity.

- [ ] Add DB constraints for roles and ownership assumptions.
  - `.github/scripts/database.SQL:25` documents valid roles in a comment only.
  - Add a check constraint for `role IN ('system', 'user', 'assistant')`, `NOT NULL` on required foreign keys, and indexes for common queries.

- [ ] Store provider metadata with messages.
  - The schema has only `role`, `content`, and `token_count`.
  - Add provider/model, finish reason, latency, token usage, and error state if multi-platform usage matters.
