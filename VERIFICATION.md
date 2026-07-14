# Verification Evidence

Document Role: Detailed verification evidence  
Scope: Backend only  
Status: Supporting current document

> This file preserves detailed evidence and sample outputs from verification runs. For the concise reviewer-facing summary of what currently passes, use `BACKEND_SUBMISSION_CHECKLIST.md` and `audit.md`.

This document contains the verification logs proving the compilation readiness, test correctness, data-race safety, and database concurrency guarantees of the `kita-be` codebase.

---

## 1. Compilation Verification

### Command
```bash
go build ./...
```

### Result
**PASS**: The command completes successfully with exit code `0` and no compilation warnings or errors.

---

## 2. Standard Test Suite Execution

### Command
```bash
go test -count=1 ./...
```

### Output
```text
?   	kita-be/cmd/book-api	[no test files]
?   	kita-be/cmd/book-worker	[no test files]
?   	kita-be/cmd/identity-api	[no test files]
?   	kita-be/cmd/transaction-api	[no test files]
ok  	kita-be/internal/auth/jwt	0.003s
?   	kita-be/internal/auth/middleware	[no test files]
ok  	kita-be/internal/auth/password	0.402s
ok  	kita-be/internal/book/delivery/http	0.004s
ok  	kita-be/internal/book/domain	0.002s
ok  	kita-be/internal/book/messaging	0.004s
?   	kita-be/internal/book/repository/postgres	[no test files]
ok  	kita-be/internal/book/usecase	0.003s
ok  	kita-be/internal/identity/delivery/http	0.218s
?   	kita-be/internal/identity/domain	[no test files]
?   	kita-be/internal/identity/repository/postgres	[no test files]
ok  	kita-be/internal/identity/usecase	1.044s
?   	kita-be/internal/platform/apperror	[no test files]
ok  	kita-be/internal/platform/config	0.003s
?   	kita-be/internal/platform/database	[no test files]
?   	kita-be/internal/platform/httpserver	[no test files]
?   	kita-be/internal/platform/logger	[no test files]
?   	kita-be/internal/platform/middleware	[no test files]
?   	kita-be/internal/platform/pagination	[no test files]
?   	kita-be/internal/platform/rabbitmq	[no test files]
ok  	kita-be/internal/platform/response	0.013s
?   	kita-be/internal/platform/validation	[no test files]
ok  	kita-be/internal/transaction/client/book	0.020s
ok  	kita-be/internal/transaction/delivery/http	0.014s
?   	kita-be/internal/transaction/domain	[no test files]
ok  	kita-be/internal/transaction/messaging	0.105s
ok  	kita-be/internal/transaction/repository/postgres	0.003s
ok  	kita-be/internal/transaction/usecase	0.005s
?   	kita-be/scripts	[no test files]
ok  	kita-be/tests/integration	0.003s
```

---

## 3. Go Race Detector Safety Check

### Command
```bash
go test -count=1 -race ./...
```

### Result
**PASS**: The entire workspace is clean. The Go race detector does not identify any data races across JWT, hashing, use cases, messaging workers, or handlers.

---

## 4. PostgreSQL Concurrency & Lock Verification

These tests use a real PostgreSQL database instance to verify database-level isolation, unique constraint guarantees under load, and Fiber/PostgreSQL advisory lock concurrency control.

### Command
```bash
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/kita_transaction?sslmode=disable" \
go test -count=1 -race -v ./internal/transaction/repository/postgres/...
```

### Output
```text
=== RUN   TestIdempotencyRepositoryConcurrentCheckOrCreatePostgres
--- PASS: TestIdempotencyRepositoryConcurrentCheckOrCreatePostgres (0.05s)
=== RUN   TestIdempotencyRepositoryRejectsConflictingPostgres
--- PASS: TestIdempotencyRepositoryRejectsConflictingPostgres (0.02s)
=== RUN   TestTransactionRepositoryConcurrencyAdvisoryLock
--- PASS: TestTransactionRepositoryConcurrencyAdvisoryLock (0.06s)
=== RUN   TestTransactionRepositoryReturnIfActiveConcurrency
--- PASS: TestTransactionRepositoryReturnIfActiveConcurrency (0.05s)
PASS
ok  	kita-be/internal/transaction/repository/postgres	1.189s
```

### Concurrency Guarantees Proved:
1. **Advisory Locks**: The lock successfully throttles parallel borrow requests. Under heavy concurrent load, exactly `maxActive` requests succeed, and subsequent requests are safely rejected with `domain.ErrActiveBorrowLimitReached`.
2. **Idempotency Records**: Parallel insertions of the same idempotency key block duplicates. Only the first transaction successfully registers; conflicting hashes or parallel keys are rejected as duplicates.
3. **Double-Return Isolation**: Only a single goroutine can transition an active transaction to returned status concurrently; other concurrent callers receive `domain.ErrTransactionNotActive`.

---

## 5. Docker Compose Configuration Verification

### Command
```bash
docker compose config
```

### Result
**PASS**: The configuration file `docker-compose.yml` parses with zero errors, validating service declarations, network structures, environment settings, port mappings, and bind volume declarations.

---

## 6. End-to-End Live Integration Verification

A live automated verification script was run against the Docker Compose environment to validate user authentication, book queries, eventual consistency in book stock, and Saga pattern status finalization.

### Command
```bash
./scripts/verify_flow.sh
```

### Output
```text
=== 1. Registering New User ===
Register response: {"success":true,"data":{"user":{"id":"4e029f69-f46e-47f3-aea3-c9993d59aeb2","full_name":"Verification User","email":"verify_user_1784010625@example.com","role":"MEMBER","status":"ACTIVE","created_at":"2026-07-14T06:30:25Z"},"access_token":"...","refresh_token":"...","token_type":"Bearer","expires_in":900}}
=== 2. Logging In to Retrieve JWT ===
JWT Token retrieved successfully.
=== 3. Fetching Book Catalog ===
Selected Book: The Lean Startup (a8bf2671-8775-47b3-b7be-ac2ce0418470)
Initial Stock: 5
=== 4. Requesting Borrow ===
Borrow initial response: {"success":true,"data":{"id":"cd57091d-e793-466b-9c69-77d251c65235","transaction_ref":"TXN-1784010626075582312","user_id":"4e029f69-f46e-47f3-aea3-c9993d59aeb2","book_id":"a8bf2671-8775-47b3-b7be-ac2ce0418470","book":{"isbn":"978-623-00-1522-0","title":"The Lean Startup","author":"Eric Ries"},"borrowed_at":"2026-07-14T06:30:26Z","due_at":"2026-07-21T06:30:26Z","returned_at":null,"status":"PENDING","fine_amount_cents":0,"late_days":0}}
Borrow Transaction created with ID: cd57091d-e793-466b-9c69-77d251c65235. Initial status is PENDING.
=== 5. Polling for Borrow Finalization (PENDING -> ACTIVE) ===
Poll #0 - Current status: PENDING
Poll #1 - Current status: ACTIVE
Transaction cd57091d-e793-466b-9c69-77d251c65235 successfully transitioned to ACTIVE.
=== 6. Verifying Stock Decrement ===
Stock after borrow: 4
Stock decremented correctly.
=== 7. Requesting Return ===
Return initial response: {"success":true,"data":{"id":"cd57091d-e793-466b-9c69-77d251c65235","transaction_ref":"TXN-1784010626075582312","user_id":"4e029f69-f46e-47f3-aea3-c9993d59aeb2","book_id":"a8bf2671-8775-47b3-b7be-ac2ce0418470","book":{"isbn":"978-623-00-1522-0","title":"The Lean Startup","author":"Eric Ries"},"borrowed_at":"2026-07-14T06:30:26Z","due_at":"2026-07-21T06:30:26Z","returned_at":"2026-07-14T06:30:28Z","status":"RETURN_PENDING","fine_amount_cents":0,"late_days":0}}
Return request submitted. Initial status is RETURN_PENDING.
=== 8. Polling for Return Finalization (RETURN_PENDING -> RETURNED) ===
Poll #0 - Current status: RETURNED
Transaction cd57091d-e793-466b-9c69-77d251c65235 successfully transitioned to final status: RETURNED.
=== 9. Verifying Stock Recovery ===
Stock after return: 5
Stock restored correctly.
=== SUCCESS: End-to-end integration verified successfully! ===
```

### Verification Result
**PASS**: The system maintains correct eventual consistency under async message processing via RabbitMQ, with correct transaction status updates.
