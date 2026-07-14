# Audit 2 — Adversarial Code Review (kita-be)

Document Role: Historical adversarial review snapshot  
Scope: Backend only  
Status: Archived historical document

> This document intentionally captures issues that existed before the current backend hardening work. It is useful as historical audit evidence, but it is not the canonical description of the current codebase state. For the current backend summary, see `audit.md`. For architecture rationale, see `SUBMISSION_NARRATIVE.md`.

> Persona: Staff/Principal Go engineer. README is marketing copy until the code proves otherwise. Every claim below was verified by reading the implementing source, not by trusting file names.

> Update 2026-07-14: this document captures the pre-fix review state. The borrow/return stock-compensation issues called out below were later fixed in the current codebase.

---

## Executive Verdict

The README is **mostly honest with two structural consistency bugs in the stock-mutation ordering and several nuance gaps**. The core engineering is solid: the transactional outbox is real, publisher confirms are real, the DLQ is wired and bound, idempotency replay is implemented correctly including hash-mismatch rejection, refresh tokens are hashed before storage, and `go vet`, `gofmt`, `golangci-lint`, and `go test ./...` all pass cleanly. The headline problems are: **(1)** the borrow flow calls `DecreaseStock` *before* the advisory lock is held, and the compensating `IncreaseStock` silently discards its error (`borrow.go:114`), creating a stock-leak window; **(2)** the return flow calls `IncreaseStock` *before* the conditional `UPDATE ... WHERE status='ACTIVE'` and has **no compensating call at all** if the DB update fails, creating a worse stock-leak on concurrent returns. The `fine_amount` column is `DECIMAL(12,2)` in Postgres but all application-layer paths use `FineAmountCents int64` — the "integer cents" claim is true at the application layer but misleading about the storage column type. The reconnect loop's outer consumer uses a flat 2-second sleep between attempts; only the inner `Reconnect()` function uses exponential backoff. These are the things most likely to be caught in a live walkthrough.

---

## Part A — README Claims Ledger

| # | Claim | Where to Look | Verdict | Evidence |
|---|-------|--------------|---------|----------|
| 1 | Idempotency with response replay for borrow & return | `borrow.go:56-74`, `return.go:52-72`, `idempotency_repository.go` | **VERIFIED** | `CheckOrCreate` uses `ON CONFLICT DO NOTHING RETURNING id`; `GetRecord` fetches stored payload; `json.Unmarshal` into domain struct and returns verbatim. Hash-mismatch causes error return (rejection). |
| 2 | Advisory lock per user prevents borrow-limit race | `transaction_repository.go:54`, `borrow.go:77-83` | **PARTIAL — BROKEN ORDERING** | Lock IS per-user via `hashtext($1)` at `transaction_repository.go:54`. BUT `borrow.go:77` performs `CountActiveByUser` *before* the lock, and `borrow.go:104` calls `DecreaseStock` (HTTP) *before* `CreateBorrowWithOutbox` where the lock is acquired. The lock only protects the second count check inside the DB transaction. Stock is already mutated before the lock is held. See Part B. |
| 3 | Unique `(transaction_id, event_type)` on `book_stock_events` prevents double stock mutation | `002_create_book_stock_events.up.sql:16`, `book_repository.go:28` | **VERIFIED** | `CREATE UNIQUE INDEX idx_book_stock_events_transaction_type ON book_stock_events(transaction_id, event_type)`. `ApplyStockEvent` uses `ON CONFLICT DO NOTHING` and falls back to `FindStockEventByEventID` / `FindStockEventByTransactionID` on duplicate. |
| 4 | RabbitMQ publisher confirms (waits for broker ack) | `publisher.go:86-113` | **VERIFIED** | `ch.Confirm(false)`, `ch.NotifyPublish(...)`, `select` on confirm channel, errors on `!conf.Ack`. Correct implementation. |
| 5 | Transactional outbox: intent stored in same DB tx as borrow/return | `transaction_repository.go:47-89`, `transaction_repository.go:154-185` | **VERIFIED** | Both `CreateBorrowWithOutbox` and `ReturnIfActiveWithOutbox` call `insertStockEventOutbox` inside the same `dbtx` before `Commit`. Genuine atomicity. |
| 6 | Dispatcher marks rows dispatched only after confirmed publish, retries on failure | `outbox_dispatcher.go:62-79` | **VERIFIED** | `MarkPublished` is called only after `PublishStockEvent` returns nil. `MarkFailed` with quadratic backoff (`attempts*attempts` seconds, capped at 1 minute) is called on error. `ClaimDue` uses `FOR UPDATE SKIP LOCKED` to prevent double-dispatch. |
| 7 | Consumer reconnect loop with exponential backoff | `consumer.go:81-112`, `connection.go:42-62` | **PARTIAL** | Loop exists in `ConsumeWithReconnect`. Exponential backoff is inside `Reconnect()` (doubles delay from `initialDelay`, caps at 30s). However, the outer consumer loop always sleeps a flat 2 seconds (`time.After(2*time.Second)`) at `consumer.go:106` before closing the connection and looping back. Additionally, `connectRabbitMQWithRetry` in both `cmd/book-worker/main.go:73-84` and `cmd/transaction-api/main.go:156-167` uses a flat delay, not exponential. |
| 8 | Dead Letter Queue declared and bound | `consumer.go:56-76`, `publisher.go:42-58`, `connection.go:16-18` | **VERIFIED** | Both `Setup()` methods declare `book.stock.mutation.dlq` with `x-dead-letter-exchange: ""` and `x-dead-letter-routing-key: book.stock.mutation.dlq` on the main queue. DLQ declared as a separate durable queue. Messages nacked with `requeue=false` route there. |
| 9 | `fine_amount_cents` stored as integer, not float | `transaction.go:33` (`FineAmountCents int64`), `001_create_borrow_transactions.up.sql:12` | **PARTIAL** | Domain struct field is `int64`. DB column is `fine_amount DECIMAL(12,2)`. Read path converts via `(fine_amount * 100)::bigint` at `transaction_repository.go:93,115,202,253,292`. Write path converts via `fineAmountDecimal(cents)` at `transaction_repository.go:322-324` which formats as `"N.NN"` string. The "no floating-point" claim is true for all application-layer calculations, but the storage column IS decimal. README says "disimpan dan dihitung sebagai integer cents" — "disimpan" (stored) is technically false for the Postgres column. |
| 10 | Double-return protection (idempotency + conditional update + stock event idempotency) | `return.go:79` (`IsActive` check), `transaction_repository.go:161-163` (`WHERE status='ACTIVE'`), `book_repository.go:26-52` (idempotency on stock event) | **VERIFIED with caveat** | Three-layer defense: idempotency replay, conditional UPDATE with status guard, stock event `ON CONFLICT DO NOTHING`. However, `IncreaseStock` at `return.go:101` executes *before* the conditional UPDATE at line 110. On concurrent returns, both can pass the `IsActive()` check at line 79, both increase stock, but only one succeeds at the DB level. The loser has no compensating `DecreaseStock` call. See Part B. |
| 11 | Book snapshot (title/author/ISBN) saved at borrow time | `004_add_book_snapshot_to_borrow_transactions.up.sql`, `borrow.go:88,102`, `transaction.go:26-28` | **VERIFIED** | Migration adds columns. `GetBook` called at line 88, `SetBookSnapshot` at line 102, INSERT at `transaction_repository.go:67-78` includes `book_isbn`, `book_title`, `book_author`. |
| 12 | Token type enforcement (access vs refresh can't be swapped) | `jwt.go:97-99` | **VERIFIED** | `ValidateAccessToken` explicitly checks `claims.TokenType != "access"` and returns error. Refresh tokens carry `token_type: "refresh"` in a separate `RefreshClaims` struct. |
| 13 | Rate limiting on auth and write endpoints | `cmd/identity-api/main.go:81-85`, `cmd/transaction-api/main.go:114-119` | **VERIFIED** | Identity: `authLimiter` (10/min) on register, token, logout. Transaction: `writeLimiter` (30/min) on borrow and return, `readLimiter` (120/min) on history/active/detail. |
| 14 | golangci-lint clean (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`) | `.golangci.yml`, actual run output | **VERIFIED** | `.golangci.yml` enables exactly those five linters. `go vet ./...` → clean. `gofmt -l .` → clean. `go test ./...` → all PASS (16 test packages, 0 failures). |
| 15 | Database-per-service, no cross-service FKs | All migration files | **VERIFIED** | Three separate DBs: `kita_identity`, `kita_book`, `kita_transaction`. No FK constraints reference foreign DBs. Cross-service references are logical (UUID stored without FK, e.g., `transaction_id UUID` in `book_stock_events`). |
| 16 | Consistent `{success, data}` / `{success, error}` envelope everywhere | `response.go:7-81`, all handlers | **VERIFIED** | Single `APIResponse` struct with `Success bool`, `Data`, `Error *APIError`, `Meta`. All handlers route through `response.OK`, `response.Error`, `response.Paginated`, etc. No handler uses raw `c.JSON`. |
| 17 | Auto-migration via `docker/postgres/init.sql` on fresh volume | `docker/postgres/init.sql` | **VERIFIED** | File creates three databases idempotently via `SELECT 'CREATE DATABASE' WHERE NOT EXISTS ...`, then `\connect` + `\i` all five transaction migrations, both book migrations, both identity migrations in correct order. |
| 18 | OpenAPI spec (`docs/openapi.yaml`) matches real routes | Spec vs `cmd/*/main.go` router registrations | **PARTIAL** | All 17 routes in the README table are present in the spec. Two issues: (a) `GET /api/v1/books/{id}/availability` at spec lines 239-244 incorrectly includes a `requestBody` block with `ReturnRequest` schema — GET requests don't have bodies; (b) `POST /api/v1/transactions/{id}/return` at spec lines 351-380 is missing a `requestBody` definition entirely, but the handler at `handler.go:67-73` requires a JSON body with `idempotency_key`. |
| 19 | `MAX_ACTIVE_BORROWS`, `LOAN_DAYS`, `DAILY_FINE_AMOUNT` read from env, not hardcoded | `config.go:40-48`, `cmd/transaction-api/main.go:61-63` | **VERIFIED** | All three read from env with defaults (3, 7, 500.00). `getEnvInt` and `getEnvMoneyCents` parse from `os.LookupEnv`. Passed into usecases via constructor. No hardcoded values in business logic. |
| 20 | SOLID: Usecase depends on interfaces, not concrete clients | `usecase/repository.go`, `usecase/book_client.go`, `borrow.go:18-25` | **VERIFIED** | `BorrowUsecase` holds `TransactionRepository` (interface), `BookServiceClient` (interface), `IdempotencyRepository` (interface), `AuditRepository` (interface). Concrete impls injected in `cmd/transaction-api/main.go`. |
| 21 | Clean Architecture: domain has zero framework/DB imports | `internal/transaction/domain/*.go`, `internal/book/domain/*.go`, `internal/identity/domain/*.go` | **VERIFIED** | Domain files import only `errors` and `time`. No Fiber, pgx, or amqp imports in any domain package. |
| 22 | Refresh tokens hashed before storage | `register.go:81-83`, `login.go:66`, `refresh.go:39-41` | **VERIFIED** | `hashToken()` at `register.go:81-83` uses `sha256.Sum256`. Login stores `hashToken(refreshTokenStr)` at `login.go:66`. Register stores `hashToken(refreshTokenStr)` at `register.go:69`. Refresh lookup uses `FindByTokenHash(ctx, hashToken(input.RefreshToken))` at `refresh.go:39-41`. Refresh token rotation also hashes the new token at `refresh.go:65`. |
| 23 | Stock decrement is conditional atomic update (not relying on advisory lock) | `book_repository.go:60`, `book_repository.go:236-238` | **VERIFIED** | Both `ApplyStockEvent` (line 60) and `DecreaseStock` (line 236) use `WHERE available_stock >= $2`. The advisory lock guards the borrow-count check, not stock safety. Stock safety is correctly handled by the conditional UPDATE. |

---

## Part B — Deep-Dive on Highest-Risk Claims

### B1. Advisory Lock Correctness — **BROKEN ORDERING**

**The sequence in the actual code:**

```
borrow.go:77  → txnRepo.CountActiveByUser()     ← UNLOCKED pool query
borrow.go:81  → if activeCount >= maxActive: reject
...
borrow.go:88  → bookClient.GetBook()            ← HTTP to Book Service
borrow.go:104 → bookClient.DecreaseStock()      ← HTTP stock mutation, BEFORE lock
...
borrow.go:113 → txnRepo.CreateBorrowWithOutbox()
  └─ transaction_repository.go:54  → pg_advisory_xact_lock(hashtext(userID))
  └─ transaction_repository.go:59  → SELECT COUNT(*) ... WHERE status='ACTIVE'  ← second count
  └─ transaction_repository.go:62  → if count >= maxActive: return ErrActiveBorrowLimitReached
  └─ transaction_repository.go:70  → INSERT borrow_transactions
  └─ transaction_repository.go:80  → INSERT stock_event_outbox
  └─ Commit
```

**Concurrent scenario (User U, 2 requests, maxActive=3, current count=2):**

| Time | Request A | Request B |
|------|-----------|-----------|
| t1 | `CountActiveByUser` → 2 (passes check) | |
| t2 | | `CountActiveByUser` → 2 (passes check) |
| t3 | `GetBook` (HTTP) | |
| t4 | `DecreaseStock` (book stock: 5→4) | `GetBook` (HTTP) |
| t5 | | `DecreaseStock` (book stock: 4→3) |
| t6 | `CreateBorrowWithOutbox` → acquires lock | |
| t7 | (locked) COUNT inside tx → 2, inserts, commits, releases lock | |
| t8 | | `CreateBorrowWithOutbox` → acquires lock |
| t9 | | (locked) COUNT inside tx → 3, **returns ErrActiveBorrowLimitReached** |
| t10 | | borrow.go:114 → `_, _ = bookClient.IncreaseStock(...)` (compensating) |

**Assessment:** The advisory lock *does* prevent the borrow_transactions insert from exceeding the limit — the second count check inside the locked transaction is the true guard. However, Request B has already decreased Book service stock at t5 before discovering at t9 that it can't insert. The compensating `IncreaseStock` at line 114 is fire-and-forget (`_, _ =`). If that call fails (Book service down, network timeout), the stock is permanently decremented without a transaction row. This is a real consistency bug — an unchecked compensation after an optimistic side effect.

**The README claim is therefore PARTIALLY BROKEN:** the advisory lock does prevent double-insertion at the DB level, but the unlocked pre-check + pre-lock stock decrease + unchecked compensating call creates a stock leak window.

---

### B2. Return Flow Stock Leak — **NO COMPENSATION AT ALL**

**The sequence in the actual code:**

```
return.go:74  → txnRepo.FindByID()              ← reads transaction
return.go:79  → txn.IsActive()                  ← in-memory check
return.go:83  → txn.BelongsTo(userID)           ← ownership check
return.go:91  → fineCalculator.Calculate()      ← fine computation
...
return.go:101 → bookClient.IncreaseStock()      ← HTTP stock mutation, BEFORE DB guard
...
return.go:110 → txnRepo.ReturnIfActiveWithOutbox()
  └─ transaction_repository.go:161 → UPDATE ... WHERE id=$1 AND user_id=$2 AND status='ACTIVE'
  └─ transaction_repository.go:172 → if RowsAffected == 0: return ErrTransactionNotActive
  └─ transaction_repository.go:176 → INSERT stock_event_outbox
  └─ Commit
```

**Concurrent scenario (two returns for the same transaction):**

| Time | Return A | Return B |
|------|----------|----------|
| t1 | `FindByID` → status ACTIVE | |
| t2 | `IsActive()` → true | `FindByID` → status ACTIVE |
| t3 | `IncreaseStock` (book stock: 2→3) | `IsActive()` → true |
| t4 | | `IncreaseStock` (book stock: 3→4) |
| t5 | `ReturnIfActiveWithOutbox` → UPDATE WHERE status='ACTIVE' → succeeds, commits | |
| t6 | | `ReturnIfActiveWithOutbox` → UPDATE WHERE status='ACTIVE' → **RowsAffected=0, returns ErrTransactionNotActive** |
| t7 | | return.go:114 → `return nil, fmt.Errorf(...)` — **NO compensating DecreaseStock** |

**Assessment:** Return B has increased stock at t4 but the DB update fails at t6. The error path at `return.go:110-114` simply returns the error — there is no compensating `DecreaseStock` call. Stock is now permanently inflated by 1. This is **worse** than the borrow-flow issue because:
- Borrow at least *attempts* a compensating call (even though it's unchecked)
- Return has zero compensation logic

The idempotency layer (claim #10) prevents this only if the client sends the same `idempotency_key`, but two independent return attempts (different keys, or one without a key) will trigger this bug.

---

### B3. Transactional Outbox Correctness — **VERIFIED CORRECT**

`CreateBorrowWithOutbox` (lines 47-89) and `ReturnIfActiveWithOutbox` (lines 154-185) both:
1. `pool.Begin()` → single DB transaction
2. Acquire advisory lock (borrow only)
3. Count check (borrow only)
4. INSERT borrow_transactions / UPDATE borrow_transactions
5. `insertStockEventOutbox()` — same `dbtx` execer
6. `Commit()`

Both the business row and outbox row are in the same DB transaction. If either fails, both roll back. This is genuine atomicity — the pattern is correctly implemented.

The dispatcher (`outbox_dispatcher.go`) uses `ClaimDue` with `FOR UPDATE SKIP LOCKED` to prevent double-dispatch in concurrent dispatcher runs, marks rows `PROCESSING` before publish (via the `ClaimDue` UPDATE), calls `MarkPublished` only on success, and `MarkFailed` with quadratic backoff (`attempts*attempts` seconds, capped at 1 minute) on error. No fire-and-forget. **Correct.**

---

### B4. Idempotency Replay Correctness — **VERIFIED CORRECT, with one nuance**

**Flow for duplicate request (same key, same body):**
1. `CheckOrCreate` → `ON CONFLICT DO NOTHING` returns `ErrNoRows` → falls into `findExisting` → compares hash → hashes match → returns `(true, nil)`
2. `GetRecord` → fetches record
3. If `Status == "COMPLETED"` and payload non-empty → `json.Unmarshal` → returns stored transaction verbatim
4. If `Status == "PROCESSING"` (in-flight) → falls through to `return nil, apperror.Conflict("duplicate request")`

**Flow for duplicate key, different body:**
`findExisting` returns stored hash → `existing != hash` → `return false, fmt.Errorf("idempotency key conflict: different request body")` → usecase maps this to a Conflict error. **Correct.**

**Nuance (not a bug, but worth noting):** If the first request succeeds but `SaveResponse` fails silently (`_ = uc.idempotencyRepo.SaveResponse(...)` at `borrow.go:139` and `return.go:137`), a retry with the same key will find the record in `PROCESSING` status and return `Conflict("duplicate request")` instead of replaying the completed transaction. The record will stay in `PROCESSING` until it expires (24h). This is a very rare case (DB write for the response payload fails after the main transaction committed) but it means the replay guarantee is not absolute — it's best-effort.

---

### B5. Stock Event Idempotency Under Real Duplication — **VERIFIED CORRECT**

**Simulated duplicate delivery of same RabbitMQ message:**

| Step | First Delivery | Second Delivery (redelivery) |
|------|---------------|------------------------------|
| 1 | `HandleStockEvent` called | `HandleStockEvent` called |
| 2 | `StockUsecase.DecreaseStockEvent` checks for existing event by event_id and transaction_id | Same checks — finds nothing (or finds existing) |
| 3 | `ApplyStockEvent` → `INSERT book_stock_events ... ON CONFLICT DO NOTHING RETURNING id` → inserts, gets id | Same INSERT → conflict, gets `ErrNoRows` |
| 4 | Stock UPDATE executes, Commit | Enters `findExisting` branch, finds existing event by `event_id` or `(transaction_id, event_type)`, returns it |
| 5 | Returns event | Returns existing event (no stock mutation) |
| 6 | Handler returns nil → `delivery.Ack` | Handler returns nil → `delivery.Ack` |

Stock is mutated exactly once. The consumer never crashes or retries-forever. The constraint is correctly used with `ON CONFLICT DO NOTHING` + fallback lookup. **Correct.**

One gap: `book_repository.go:28` uses `ON CONFLICT DO NOTHING` without specifying the constraint name. This works because both unique indexes on `book_stock_events` (`event_id` and `(transaction_id, event_type)`) will catch conflicts, but it means any unique constraint violation silently swallows. Low risk in practice since the fallback lookups cover both paths.

---

### B6. Reconnect + DLQ Interaction — **VERIFIED with caveats**

**Disconnect mid-processing (before ack):**
- Message is unacked, broker redelivers after reconnect
- Redelivery hits `HandleStockEvent` → `ApplyStockEvent` → `ON CONFLICT DO NOTHING` → returns existing event → handler returns nil → acks. **Correct.**

**DLQ routing:**
- Exchange: `library.events` (topic)
- Main queue: `book.stock.mutation` with `x-dead-letter-exchange: ""` (default exchange) and `x-dead-letter-routing-key: book.stock.mutation.dlq`
- DLQ: `book.stock.mutation.dlq` declared as durable queue
- Messages nacked with `requeue=false` → dead-lettered to default exchange with routing key `book.stock.mutation.dlq` → delivered to `book.stock.mutation.dlq` queue. **Correct.**

**DLQ is properly wired.** The only gap: there is no consumer of the DLQ. Messages sit there for inspection but no alerting or re-processing mechanism exists. The README says "masuk ke DLQ agar bisa diperiksa" which is accurate.

**Reconnect sequence:**
After `consume()` returns an error (deliveries channel closed), `ConsumeWithReconnect` sleeps 2 seconds (flat, `consumer.go:106`), calls `c.conn.Close()`, then next iteration checks `IsConnected()` → false → calls `c.conn.Reconnect(30, 2*time.Second)` which internally doubles delay on each attempt (2s→4s→8s→16s→30s cap). The README says "exponential backoff" — technically true inside `Reconnect`, but the outer loop's fixed 2s sleep before each reconnect attempt means the effective backoff pattern is: 2s flat pause, then Reconnect attempt 1 (2s), attempt 2 (4s), attempt 3 (8s)... The outer 2s is not meaningfully harmful but the "exponential backoff" description is loose.

Additionally, `connectRabbitMQWithRetry` in `cmd/book-worker/main.go:73-84` and `cmd/transaction-api/main.go:156-167` uses a flat `time.Sleep(delay)` with no backoff at all — this is the startup connection path, not the runtime reconnect, but it's another place where "reconnect" doesn't actually back off.

---

## Part C — DRY / Code Quality Reality Check

### C1. Duplication across services

**Response envelope:** Single `platform/response/response.go` file. All handlers import it. Zero copy-paste. **Correct.**

**Error mapping:** `platform/apperror` package. All three services use the same error types (`BadRequest`, `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`). **Correct.**

**Pagination logic:** `platform/pagination` package (`Normalize` function) used in all three services. **Correct.**

**Verdict: No cross-service duplication.** The platform layer is used correctly.

### C2. Duplication within a service

Looking at 3 transaction handlers:

```go
// Borrow (handler.go:33-35)
userID, ok := c.Locals("user_id").(string)
if !ok { return response.Unauthorized(...) }

// Return (handler.go:62-64) — identical
userID, ok := c.Locals("user_id").(string)
if !ok { return response.Unauthorized(...) }

// History (handler.go:93-95) — identical
userID, ok := c.Locals("user_id").(string)
if !ok { return response.Unauthorized(...) }
```

This 3-line pattern repeats in every authenticated handler across all services (5 times in transaction handler, 1 time in identity handler). It's not extracted into a helper. Harmless but mildly noisy. Not a correctness issue.

**Pagination total-pages calculation duplicated:**
```go
// History (handler.go:116-119)
totalPages := output.Total / int64(perPage)
if output.Total%int64(perPage) != 0 { totalPages++ }

// InternalTransactions (handler.go:185-188) — identical
totalPages := output.Total / int64(perPage)
if output.Total%int64(perPage) != 0 { totalPages++ }

// Book List (book/handler.go:60-63) — identical
totalPages := output.Total / int64(perPage)
if output.Total%int64(perPage) != 0 { totalPages++ }
```

This 4-line block appears three times across two services. Minor.

### C3. Usecase bloat

`borrow.go:Execute` (150 lines total, ~90 lines of function body) orchestrates: idempotency check → active count pre-check → book lookup → stock decrease → build transaction → insert with outbox → audit → save idempotency response. That is six concerns in one function.

It's readable and linearly structured — not spaghetti — but it's untestable at the advisory-lock level (the real lock is in the repository, not mockable via the interface). The test's `fakeTxnRepo.CreateBorrowWithOutbox` does a count check but has no concurrency guarantees. The `-race` flag would not detect the window described in B1 because the fake doesn't model the lock interleaving.

`return.go:Execute` (148 lines, ~90 lines of body) has a similar structure but with the additional problem that the stock increase and DB update are not wrapped in any compensation logic.

### C4. Dead/unused abstractions

`RecordStockEvent` at `book_repository.go:282-296` is defined in the `BookRepository` interface at `book/usecase/repository.go:18` but is never called by any production usecase code. `ApplyStockEvent` handles event recording internally. `RecordStockEvent` exists only in test fakes (to satisfy the interface) and the concrete implementation. The `unused` linter doesn't catch it because it satisfies an interface method. This is dead code kept alive by an interface contract — minor interface-itis.

`TransactionRepository` interface in `usecase/repository.go` has exactly one production implementation (`postgres.TransactionRepository`) and is used by three usecases. This IS the correct DIP usage — the interface enables the fake implementations in tests. No interface-itis detected for other interfaces.

`StockEventOutboxRepository` and `StockEventPublisher` in `messaging/outbox_dispatcher.go` are interfaces with single implementations but they're needed to test the dispatcher without a real DB/broker. Legitimate.

### C5. Error handling honesty

**Non-trivial ignored errors found:**

| Location | Ignored Error | Risk |
|----------|---------------|------|
| `borrow.go:114` | `_, _ = uc.bookClient.IncreaseStock(...)` (compensating call) | **HIGH** — if this fails, stock is permanently decremented without a transaction row |
| `borrow.go:139` | `_ = uc.idempotencyRepo.SaveResponse(...)` | Low — retry still works, replay becomes "duplicate request" error for 24h |
| `return.go:137` | `_ = uc.idempotencyRepo.SaveResponse(...)` | Low — same as above |
| `return.go:110-114` | No compensating `DecreaseStock` when `ReturnIfActiveWithOutbox` fails | **HIGH** — stock permanently inflated on concurrent returns |
| `identity/delivery/http/handler.go:84` | `_ = c.BodyParser(&req)` | Low — falls back to FormValue parsing |

The compensating `IncreaseStock` at `borrow.go:114` and the missing compensation at `return.go:110-114` are the only ones with real severity. `golangci-lint` doesn't catch the `_, _ =` pattern because it's intentionally blanked with the blank identifier. The linter is not misconfigured — the developer chose to silence it. The README claim of "golangci-lint clean" is true: the linter passes. But the underlying risk is real.

### C6. Static analysis results (actually run)

```
go vet ./...      → clean (0 issues)
gofmt -l .        → clean (0 files)
go test ./...     → all PASS (16 test packages, 0 failures)
```

All three checks pass. The linter config is minimal (5 linters, no custom rules) but appropriate for the project scope.

---

## Part D — Standard Dimensions

| Severity | Dimension | Location | Issue | Recommended Fix |
|----------|-----------|----------|-------|-----------------|
| **Critical** | Return-flow stock leak | `return.go:101`, `return.go:110-114` | `IncreaseStock` executes before the conditional `UPDATE ... WHERE status='ACTIVE'`. On concurrent returns, both pass the in-memory `IsActive()` check, both increase stock, but only one succeeds at the DB level. The loser has **no compensating `DecreaseStock` call**. Stock is permanently inflated. | **Do not fix this by merely reordering the HTTP call.** Best-practice fix: persist the return state change and stock-mutation intent atomically via the existing outbox pattern, then apply the stock increase asynchronously with idempotent consumption, retries, and reconciliation. If synchronous semantics are required, use a saga with durable compensation records and retryable compensations rather than best-effort rollback. |
| **Critical** | Borrow advisory lock ordering + unchecked compensation | `borrow.go:77-83`, `borrow.go:104`, `borrow.go:114`, `transaction_repository.go:54` | `DecreaseStock` (line 104) executes *before* the advisory lock is held. The compensating `IncreaseStock` at line 114 silently discards its error (`_, _ =`). If Book service is down at that moment, stock leaks permanently. | **Do not fix this by simply moving `DecreaseStock` after the DB write.** Best-practice fix: persist the borrow row and stock-decrease intent atomically, then mutate stock via an idempotent worker/outbox flow. If keeping synchronous reservation, persist compensation failures durably and retry them until reconciled instead of logging-and-forgetting. |
| **High** | `fine_amount` column type vs claim | `001_create_borrow_transactions.up.sql:12`, `transaction_repository.go:322-324` | Column is `DECIMAL(12,2)` not integer. Read path uses `(fine_amount * 100)::bigint`. Write path formats int64 as `"N.NN"` string. This works but the README claim "disimpan dan dihitung sebagai integer cents" is false for the DB layer. | Change column to `BIGINT` named `fine_amount_cents` to match the domain struct, or reword README to say "dihitung dan diekspos sebagai integer cents; kolom DB menyimpan sebagai DECIMAL" |
| **Medium** | Reconnect backoff description | `consumer.go:103-110`, `connection.go:47-61`, `cmd/book-worker/main.go:73-84` | README says "exponential backoff" — the inner `Reconnect()` call does back off exponentially (2s→4s→8s…→30s cap), but the outer consumer loop sleeps a flat 2s before each reconnect attempt. Startup `connectRabbitMQWithRetry` in both cmd mains uses flat delay. | Either implement true exponential backoff in the outer loop and startup retry, or clarify in README that "setiap reconnect attempt menggunakan exponential backoff" |
| **Medium** | `CountActiveByUser` redundant unlocked pre-check | `borrow.go:77-83` | The count check at line 77 is redundant — the definitive check is inside the locked transaction at `transaction_repository.go:62`. The unlocked pre-check only avoids the HTTP calls for *obvious* over-limit cases, but doesn't prevent concurrent bypass. It creates a false sense of safety and contributes to the stock-leak window. | Remove the unlocked pre-check entirely, or document it explicitly as an optimization (not a safety guard). The advisory lock + second count is the real guard. |
| **Medium** | OpenAPI spec errors | `docs/openapi.yaml:239-244`, `docs/openapi.yaml:351-380` | (a) `GET /api/v1/books/{id}/availability` specifies a `requestBody` with `ReturnRequest` schema — GET requests cannot have request bodies per HTTP spec. (b) `POST /api/v1/transactions/{id}/return` is missing a `requestBody` definition, but the handler requires `{"idempotency_key": "..."}`. | Remove the `requestBody` from the availability path. Add a `requestBody` referencing `ReturnRequest` to the return path. |
| **Medium** | No race-condition test for borrow or return | `usecase_test.go` | Tests cover sequential borrow limit, idempotency, and outbox, but no concurrent goroutine test that exercises the advisory-lock path or the return-flow race under `-race`. The fake repo's `CreateBorrowWithOutbox` has no actual locking. | Add integration tests that fire N goroutines calling `BorrowUsecase.Execute` and `ReturnUsecase.Execute` concurrently against the same user/transaction with a real Postgres and `-race` flag |
| **Low** | `userID` extraction boilerplate repeated | All authenticated handlers | 3-line `c.Locals("user_id").(string)` + unauthorized response repeated 6+ times across services | Extract to a helper function `getUserID(c *fiber.Ctx) (string, error)` |
| **Low** | Pagination total-pages calculation duplicated | `transaction/handler.go:116-119,185-188`, `book/handler.go:60-63` | 4-line block appears 3 times across 2 services | Extract to `pagination.TotalPages(total int64, perPage int) int64` |
| **Low** | DLQ has no consumer/alerting | `consumer.go`, `connection.go` | DLQ is declared and wired but no process reads it | Document this as a known operational gap; add a simple admin endpoint or monitoring check for DLQ depth |
| **Low** | `RecordStockEvent` dead code | `book_repository.go:282-296`, `book/usecase/repository.go:18` | `RecordStockEvent` is defined in the interface and implemented but never called by any production usecase. Only exists in test fakes to satisfy the interface. | Remove from the interface and delete the implementation, or document why it's kept for future use |
| **Low** | Fine calculation boundary: same-day return after due time | `fine.go:16-28` | If `returnedAt` is after `dueAt` by seconds but on the same calendar day, `lateDays` computes to 0 and status is `RETURNED` (not `RETURNED_LATE`). This is likely correct behavior but not explicitly tested. | Add a test case for same-day return after due time to document the expected behavior |

---

## Bottom Line: 3 Things Most Likely to Be Caught in a Live Walkthrough

### 1. The Return-Flow Stock Leak (Critical)
A senior engineer will trace the return flow top-to-bottom and notice that `IncreaseStock` at `return.go:101` is called *before* the conditional `UPDATE ... WHERE status='ACTIVE'` at line 110. They will then ask "what happens if two returns arrive concurrently?" The answer — that both increase stock, only one succeeds at the DB level, and the loser has **no compensating DecreaseStock** — is the most damaging finding in the codebase. **Fix before submission: do not just reorder the call. The best-practice fix is to record the return and stock-mutation intent atomically, then perform the stock update through the existing idempotent outbox pipeline, with durable retries/reconciliation or a real saga if synchronous semantics are mandatory.**

### 2. The Borrow Advisory Lock Timing Gap + Unchecked Compensation (Critical)
The same engineer will then trace the borrow flow and notice that `DecreaseStock` is called at line 104 *before* the advisory lock is held at line 113. They will ask "what happens if `IncreaseStock` fails on line 114?" The answer — that the error is silently discarded (`_, _ =`) and the stock is permanently wrong — compounds the problem. **Fix before submission: do not just swap the call order. The best-practice fix is to persist the borrow row and stock-decrease intent atomically, then execute stock mutation through an idempotent worker/outbox flow, or else store compensation failures durably and retry until reconciled.**

### 3. The `fine_amount` Column Type vs "Integer Cents" Claim (High)
The README says "disimpan dan dihitung sebagai integer cents." The migration says `DECIMAL(12,2)`. This is a 30-second gotcha that an engineer reading migrations will notice immediately. **Fix: either migrate the column to `BIGINT fine_amount_cents` or reword the README to say "dihitung dan diekspos sebagai integer cents; kolom DB menyimpan sebagai DECIMAL dan dikonversi saat dibaca."**

---

### What to Fix in the README (if the code won't change)

| README Claim | Suggested Reword |
|---|---|
| "denda disimpan dan dihitung sebagai integer cents (`fine_amount_cents`), bukan floating-point" | "denda dihitung dan diekspos sebagai integer cents; kolom DB menyimpan sebagai DECIMAL dan dikonversi saat dibaca" |
| "consumer otomatis reconnect dengan exponential backoff saat broker bermasalah" | "consumer otomatis reconnect dengan retry loop; setiap reconnect attempt menggunakan exponential backoff" |
| "Advisory lock per user prevents borrow-limit race" (implied by Engineering Highlights) | Acknowledge that the advisory lock protects the DB insert, but the pre-lock stock decrease via HTTP is an optimistic call with best-effort compensation |
| "Double-return protection" | This claim is directionally correct for the idempotency + conditional UPDATE layers, but the stock increase happens before the conditional UPDATE guard and has no compensation on failure |
