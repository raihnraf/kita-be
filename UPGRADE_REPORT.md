# Backend Upgrade Report

Document Role: Engineering change log  
Scope: Backend only  
Status: Supporting current document

> This file is accurate for the current codebase, but it is not the primary reviewer summary. Use `BACKEND_SUBMISSION_CHECKLIST.md` and `audit.md` first if you want the shortest current-state view.

Scope: backend only, aligned to the backend portion of `soal.md`.

This report summarizes the main upgrades applied after review, using wording that matches the current codebase exactly.

## 1. Money Model Alignment

Previous issue:

- The application domain used `FineAmountCents int64`, but the database previously stored fines using a decimal money representation.
- That required conversion logic on both the read and write paths.

Current state:

- PostgreSQL now stores fines directly as `fine_amount_cents BIGINT`.
- The repository reads and writes integer cents directly, without decimal conversion helpers.

Verified in:

- `migrations/transaction/001_create_borrow_transactions.up.sql`
- `internal/transaction/repository/postgres/transaction_repository.go`
- `internal/transaction/delivery/http/dto.go`

Why this is better:

- The domain model, API contract, and database schema now use the same representation.
- This removes conversion overhead and avoids money-format drift between layers.

## 2. PostgreSQL Concurrency Coverage

Previous issue:

- Unit tests already existed, but they did not prove the PostgreSQL advisory lock behavior or concurrent transaction state transitions against a real database.

Current state:

- The repository package now includes real PostgreSQL concurrency tests behind `TEST_DATABASE_URL`.
- These tests exercise the actual transaction repository with concurrent goroutines.

Verified tests:

- `TestTransactionRepositoryConcurrencyAdvisoryLock`
- `TestTransactionRepositoryReturnIfActiveConcurrency`

What they prove:

- The borrow repository path allows only `maxActive` successful inserts for the same user under concurrent load.
- The return repository path allows only one successful return for the same active transaction under concurrent load.

Important scope note:

- These tests validate repository-level concurrency guarantees.
- They do not, by themselves, prove the full cross-service stock-mutation flow.

## 3. Cross-Service Request Tracing

Previous issue:

- Request IDs were generated at the HTTP edge, but were not consistently propagated across process boundaries.
- That made log correlation harder when `transaction-api` called `book-api`.

Current state:

- Request ID middleware stores the request ID in the Go user context via `c.SetUserContext(...)`.
- HTTP handlers now pass `c.UserContext()` into use cases.
- The internal Book client propagates `X-Request-ID` to downstream requests when a request ID exists in context.

Verified in:

- `internal/platform/middleware/middleware.go`
- `internal/identity/delivery/http/handler.go`
- `internal/book/delivery/http/handler.go`
- `internal/transaction/delivery/http/handler.go`
- `internal/transaction/client/book/client.go`

Why this is better:

- Logs across Identity, Book, and Transaction services can now be correlated more reliably for a single user request.

## 4. RabbitMQ Startup Backoff

Previous issue:

- Runtime reconnect already used backoff, but the initial startup retry path used a flat delay.

Current state:

- Startup connection retry in `book-worker` and `transaction-api` now uses exponential backoff with a cap.

Verified in:

- `cmd/book-worker/main.go`
- `cmd/transaction-api/main.go`

Why this is better:

- Startup behaves more gracefully when RabbitMQ is temporarily unavailable.

## 5. Borrow and Return Reliability Upgrades

### Borrow path

Current behavior:

- Borrow first commits a `PENDING` transaction plus an internal stock-decrease outbox row in one DB transaction.
- The outbox dispatcher publishes the stock command to RabbitMQ.
- Book Worker applies the stock mutation idempotently and publishes a result event.
- Transaction Service activates or cancels the borrow only after consuming that result event.

Verified in:

- `internal/transaction/usecase/borrow.go`
- `internal/transaction/messaging/reconciler.go`
- `internal/transaction/repository/postgres/transaction_repository.go`
- `internal/transaction/domain/outbox.go`

### Return path

Current behavior:

- Return first commits `RETURN_PENDING` plus an internal stock-increase outbox row in one database transaction.
- The outbox dispatcher publishes the stock restore command to RabbitMQ.
- Book Worker applies the stock mutation idempotently and publishes a result event.
- Transaction Service finalizes the return only after consuming that result event.

Verified in:

- `internal/transaction/usecase/return.go`

Why this is better:

- The previous double-return inflation bug is removed.
- Borrow and return now use the same consistency model instead of mixing sync stock mutation with async fallback.
- Result delivery is replay-safe because Book Service persists the outcome of each stock command per transaction.
- Reconciliation now re-dispatches stuck commands instead of guessing business compensation.

Tradeoff note:

- The public API returns immediately with `PENDING` or `RETURN_PENDING` using HTTP `202 Accepted` status code (previously returning `201 Created` or `200 OK`), explicitly notifying clients that the request has been accepted for asynchronous processing and eventual finalization.
- The RabbitMQ contract now uses explicit command/result event names such as `DecreaseStockRequested`, `DecreaseStockSucceeded`, and `IncreaseStockRejected`, so consumers do not need to infer business meaning from a generic `event_type + result` combination.
- The design is cleaner and safer, but UX now depends on polling or refresh to observe final status transitions.
- For this take-home backend requested by `soal.md`, that tradeoff is reasonable because it keeps service boundaries clean while demonstrating meaningful RabbitMQ usage.

## 6. Verification

Verified in this repository:

```bash
go test ./...
```

Additional verification recommended before final delivery:

```bash
go test -race ./...
go build ./...
docker compose config
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/kita?sslmode=disable" go test -race -v ./internal/transaction/repository/postgres/...
```

The PostgreSQL repository concurrency suite is intended to cover:

- `TestIdempotencyRepositoryConcurrentCheckOrCreatePostgres`
- `TestIdempotencyRepositoryRejectsConflictingPostgres`
- `TestTransactionRepositoryConcurrencyAdvisoryLock`
- `TestTransactionRepositoryReturnIfActiveConcurrency`

## 7. Best-Practice Assessment

For the backend scope requested by `soal.md`, the codebase now follows best practice well enough for a strong take-home submission:

- required backend stack is satisfied
- service boundaries are clear
- SOLID and Clean Architecture are genuinely applied
- Docker Compose, RabbitMQ, and tests are present
- concurrency and failure handling are significantly stronger than the baseline implementation

Remaining tradeoffs worth acknowledging openly in a review:

- borrow and return are now eventually consistent from the API caller perspective
- mobile or frontend clients should refresh detail/history to observe final status after `PENDING` or `RETURN_PENDING`

Those tradeoffs are reasonable for this take-home backend and are documented rather than hidden.
