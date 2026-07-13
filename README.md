# Kita Library Backend

> **Backend microservices** for the Kita Library Management System.
> Built with Go · Fiber · PostgreSQL · JWT OAuth2 · Clean Architecture · SOLID · RabbitMQ · Docker Compose.

This repository is **backend-only**. The Flutter mobile app lives in a separate repository and calls this as its API server.

---

## Requirements Checklist (soal.md)

Every item from the technical test specification is implemented and verified.

### Technology Stack

| Requirement | Status | Detail |
|---|---|---|
| Backend: **Go + Fiber** | ✅ | Go 1.25, `gofiber/fiber v2` across all services |
| Database: **PostgreSQL** | ✅ | PostgreSQL 16, separate DB per service |
| Auth: **OAuth2 + JWT** | ✅ | `password` and `refresh_token` grant types, HS256 JWT |
| **SOLID + Clean Architecture** | ✅ | Strict domain → usecase → delivery layers, all deps via interfaces |
| Message Broker: **RabbitMQ** *(bonus)* | ✅ | Publisher confirms, consumer reconnect loop, DLQ, retry |
| **Docker Compose** *(bonus)* | ✅ | All 6 containers start on fresh volume, migrations auto-run |
| **Unit Testing** *(bonus)* | ✅ | 60+ tests across all layers, race-safe, `golangci-lint` clean |

### Microservices

| Service | Responsibility | Status |
|---|---|---|
| **Identity Service** | Register, Login, Token (OAuth2), Refresh, Logout, Profile | ✅ |
| **Book Service** | Catalog, search, stock management, availability check | ✅ |
| **Transaction Service** | Borrow, Return, fine calculation, history, active loans | ✅ |
| **Book Worker** *(bonus)* | Async RabbitMQ consumer for stock sync | ✅ |

### Business Rules

| Rule | Status | Implementation |
|---|---|---|
| All Transaction endpoints require valid JWT | ✅ | `JWTAuth` middleware on every protected route |
| Cannot borrow book with stock = 0 | ✅ | Stock check at borrow time; `DecreaseStock` rejects zero stock |
| Borrow reduces stock in Book Service | ✅ | Synchronous HTTP (primary) + async RabbitMQ (bonus) |
| Maximum **3 active borrows** per user | ✅ | PostgreSQL advisory lock + `CreateIfUserBelowActiveLimit` |
| Late returns incur **daily fine** | ✅ | `FineCalculator`, integer cents, configurable via `DAILY_FINE_AMOUNT` |
| Duplicate borrow/return idempotency | ✅ | Idempotency table with response replay; concurrent requests get `409` |

---

## Engineering Highlights

Things this backend does beyond the minimum spec:

- **Idempotency with response replay** — duplicate borrow requests within a window return the original cached response, not an error, making retries safe.
- **Advisory lock for borrow race** — a PostgreSQL per-user advisory lock wraps the active-count check and insert atomically, preventing double-borrows under concurrent load.
- **Stock event idempotency** — a `(transaction_id, event_type)` unique constraint in `book_stock_events` ensures stock never changes twice for the same transaction, even if the Book Worker receives a duplicate RabbitMQ delivery.
- **RabbitMQ publisher confirms** — every publish waits for a broker `ack` before returning. Nacks surface as errors and are logged.
- **RabbitMQ consumer reconnect loop** — `ConsumeWithReconnect` automatically reconnects with exponential backoff if the broker goes down, without restarting the worker process.
- **Dead Letter Queue** — messages that fail after 3 retries are nacked to the DLQ for inspection, not silently dropped.
- **Integer cents for money** — fines use `int64` cents internally (`fine_amount_cents` in responses); no floating-point math anywhere near money.
- **Double-return protection** — `ReturnIfActive` uses a conditional SQL update; a second return on the same transaction gets `409` and does not inflate stock.
- **Book snapshot on borrow** — title, author, and ISBN are snapshotted into the transaction row at borrow time, so history responses never call the Book Service per row.
- **Token type enforcement** — the JWT payload carries a `token_type` field; access token endpoints reject refresh tokens and vice versa.
- **Rate limiting** — Fiber rate limiter on auth and all transaction write endpoints (30 req/min).
- **golangci-lint clean** — `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused` — zero issues.

---

## Architecture

```
┌─────────────────────┐  ┌─────────────────┐  ┌──────────────────────┐
│   Identity Service  │  │   Book Service   │  │  Transaction Service │
│   (identity-api)    │  │   (book-api)     │  │  (transaction-api)   │
│                     │  │                  │  │                      │
│  Register / Login   │  │  Catalog / Search│  │  Borrow / Return     │
│  JWT Token OAuth2   │  │  Stock Mgmt      │  │  Fine Calculation    │
│  Profile            │  │  Availability    │  │  History / Audit     │
└────────┬────────────┘  └────────┬─────────┘  └───────────┬──────────┘
         │                        │                        │
         │  ┌───────────────────────────────────────────┐  │
         └──►      stateless JWT verification (HS256)    ◄──┘
            └───────────────────────────────────────────┘
                                │
┌───────────────────────┐       │
│    Book Worker        │       │
│  (RabbitMQ Consumer)  │◄──────┘ (async stock events)
│  Reconnect Loop + DLQ │
└───────────┬───────────┘
            │
    ┌───────▼───────┐     ┌────────────┐
    │   RabbitMQ    │     │ PostgreSQL │
    │  + DLQ        │     │ (3 DBs)    │
    └───────────────┘     └────────────┘
```

**Service-to-Service Communication:**
- Transaction Service → Book Service: synchronous HTTP via internal API (`X-Internal-Token`)
- Transaction Service → RabbitMQ: async stock event publishing (bonus path, disabled by default in Compose)
- Book Worker ← RabbitMQ: consumes stock events, applies idempotently to Book DB

---

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.25+ |
| HTTP Framework | Fiber v2 |
| Database | PostgreSQL 16 |
| Auth | JWT HS256, OAuth2 (`password` + `refresh_token` grants) |
| Message Broker | RabbitMQ 3 with publisher confirms + DLQ |
| Password Hashing | bcrypt |
| Containerization | Docker Compose |
| Linting | golangci-lint (`errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`) |
| CI | GitHub Actions (lint · test · build · Postgres integration) |

---

## Quick Start

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- RabbitMQ (optional, included in Docker Compose)

### Option 1: Docker Compose (Recommended)

```bash
# Clone and enter project
cd kita-be

# Copy environment file
cp .env.example .env
# Edit .env with your secrets (JWT_SECRET, INTERNAL_API_TOKEN)

# Start all services (identity, book, transaction, worker, postgres, rabbitmq)
docker-compose up -d --build

# Verify all services are healthy
curl http://localhost:3000/api/v1/ready   # Identity
curl http://localhost:3001/api/v1/ready   # Book
curl http://localhost:3002/api/v1/ready   # Transaction

# Seed sample books
go run ./scripts/seed.go
```

> **Note:** On a **fresh Postgres volume**, `docker/postgres/init.sql` automatically creates databases and runs all migrations. No manual migration step needed.

### Option 2: Local Development

```bash
# Start infrastructure only
docker-compose up -d postgres rabbitmq

# Create databases
docker-compose exec postgres psql -U postgres -c "CREATE DATABASE kita_identity;"
docker-compose exec postgres psql -U postgres -c "CREATE DATABASE kita_book;"
docker-compose exec postgres psql -U postgres -c "CREATE DATABASE kita_transaction;"

# Copy and configure environment
cp .env.example .env

# Run migrations
psql -h localhost -U postgres -d kita_identity -f migrations/identity/001_create_users.up.sql
psql -h localhost -U postgres -d kita_identity -f migrations/identity/002_create_refresh_tokens.up.sql
psql -h localhost -U postgres -d kita_book    -f migrations/book/001_create_books.up.sql
psql -h localhost -U postgres -d kita_book    -f migrations/book/002_create_book_stock_events.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/001_create_borrow_transactions.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/002_create_transaction_audits.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/003_create_idempotency_records.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/004_add_book_snapshot_to_borrow_transactions.up.sql

# Seed books
go run ./scripts/seed.go

# Start services (separate terminals, or use make)
make run-identity
make run-book
make run-transaction
make run-worker   # optional RabbitMQ consumer
```

### Makefile Commands

```bash
make test          # go test ./...
make test-verbose  # go test ./... -v
make build         # build all 4 binaries to bin/
make docker-up     # docker-compose up -d
make docker-down   # docker-compose down
make docker-logs   # docker-compose logs -f
make seed          # go run ./scripts/seed.go
make clean         # rm -rf bin/
```

---

## API Reference

Full OpenAPI specification: [`docs/openapi.yaml`](docs/openapi.yaml)
HTTP request samples: [`docs/api-requests.http`](docs/api-requests.http)

Base URLs (default ports):
- Identity Service: `http://localhost:3000`
- Book Service: `http://localhost:3001`
- Transaction Service: `http://localhost:3002`

### Identity Service

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | Public | Register new user, returns access token |
| POST | `/api/v1/auth/token` | Public | OAuth2: `password` or `refresh_token` grant |
| POST | `/api/v1/auth/logout` | Public | Revoke submitted refresh token (session-scoped) |
| GET | `/api/v1/users/me` | JWT | Logged-in user profile |
| GET | `/api/v1/health` | Public | Health check |
| GET | `/api/v1/ready` | Public | Readiness check (DB ping) |

### Book Service

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/books` | Public | List books (paginated, searchable, category filter) |
| GET | `/api/v1/books/:id` | Public | Book detail |
| GET | `/api/v1/books/:id/availability` | Public | Real-time available stock check |
| POST | `/api/v1/books` | Internal Token | Create book |
| PUT | `/api/v1/books/:id` | Internal Token | Update book |
| POST | `/api/v1/internal/books/:id/stock/decrease` | Internal Token | Reserve stock (borrow) |
| POST | `/api/v1/internal/books/:id/stock/increase` | Internal Token | Release stock (return) |
| GET | `/api/v1/health` | Public | Health check |
| GET | `/api/v1/ready` | Public | Readiness check (DB ping) |

### Transaction Service

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/transactions/borrow` | JWT | Borrow a book (idempotent) |
| POST | `/api/v1/transactions/:id/return` | JWT | Return a borrowed book |
| GET | `/api/v1/transactions/history` | JWT | Paginated transaction history |
| GET | `/api/v1/transactions/active` | JWT | Currently borrowed books |
| GET | `/api/v1/transactions/:id` | JWT | Single transaction detail |
| GET | `/api/v1/internal/transactions` | Internal Token | Admin: all transactions |
| GET | `/api/v1/internal/transactions/:id` | Internal Token | Admin: transaction detail |
| GET | `/api/v1/internal/transactions/:id/audits` | Internal Token | Admin: audit trail |
| GET | `/api/v1/health` | Public | Health check |
| GET | `/api/v1/ready` | Public | Readiness check (DB + Book Service ping) |

### Response Format

```json
// Success
{ "success": true, "data": { ... } }

// Success (paginated)
{ "success": true, "data": [...], "meta": { "page": 1, "per_page": 20, "total": 42, "total_pages": 3 } }

// Error
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "email is required" } }
```

### Business Rules

- Maximum **3 active borrowed books** per user
- Cannot borrow a book with **zero available stock**
- Loan period: **7 days** (configurable via `LOAN_DAYS`)
- Late fine: **500 cents/day** (configurable via `DAILY_FINE_AMOUNT`, exposed as `fine_amount_cents`)
- All Transaction Service endpoints require a valid JWT access token
- Borrow requests require an `idempotency_key` to prevent double-borrow on retry

---

## SOLID Principles

### Single Responsibility Principle (SRP)
Each layer has exactly one reason to change:
- **`delivery/http`** — parse HTTP requests, call usecase, return JSON. No business logic.
- **`usecase`** — orchestrate business rules. No HTTP, no SQL.
- **`repository/postgres`** — SQL only. No business logic.
- **`auth/jwt`** / **`auth/password`** — token and password concerns completely isolated from domain code.

### Open/Closed Principle (OCP)
New behavior without changing existing code:
- `FineCalculator` can be extended with tiered fine rules without touching `ReturnUsecase`.
- New stock event types can be added to the worker without modifying the consumer infrastructure.
- New middleware (logging, tracing) plugs into Fiber without touching any handler.

### Liskov Substitution Principle (LSP)
Any implementation satisfying an interface is substitutable:
- `handlerFakeTxnRepo` in tests replaces the real `postgres.TransactionRepository` transparently.
- `handlerFakeBookClient` replaces the real HTTP Book Service client — same interface, no test doubles needed.
- The RabbitMQ `StockEventPublisher` can be swapped for any other transport without touching `BorrowUsecase`.

### Interface Segregation Principle (ISP)
Small, focused interfaces — usecases only see what they need:
```go
// BorrowUsecase depends on these three, nothing more
type TransactionRepository interface { ... }  // 8 methods
type BookServiceClient      interface { ... }  // 3 methods
type StockEventPublisher    interface { ... }  // 2 methods
```

### Dependency Inversion Principle (DIP)
High-level policy depends on abstractions, not concretions:
- `BorrowUsecase` depends on `BookServiceClient` interface → wired to `*bookclient.Client` in `main.go`.
- `BorrowUsecase` depends on `StockEventPublisher` interface → wired to `*messaging.Publisher` (or nil) in `main.go`.
- All wiring is constructor injection in `cmd/*/main.go` — no globals, no `init()` magic.

---

## Testing

```bash
make test                          # all unit tests
go test -race ./...                # race detector
go test ./... -v                   # verbose
go test ./internal/transaction/usecase/... -v  # single package

# Postgres-backed integration tests (requires running Postgres)
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/kita_test?sslmode=disable" \
  go test ./internal/transaction/repository/postgres -run Postgres -count=1 -v
```

### Test Coverage by Layer

| Layer | What's Tested |
|---|---|
| **JWT Service** | Token generation, validation, expiry, wrong secret, token type rejection |
| **Password Service** | Hash, verify, wrong password, bcrypt salt uniqueness |
| **Identity Handlers** | Register validation, register success, OAuth password grant, profile unauthorized |
| **Identity Usecases** | Register, duplicate email, login, wrong password, refresh, token reuse, single-token logout, profile |
| **Book Handlers** | Create validation, create success, list pagination normalization, internal stock decrease |
| **Book Domain** | Stock decrease/increase, zero stock guard, insufficient stock |
| **Book Usecases** | List, search, pagination, create, duplicate ISBN, stock ops, availability |
| **Transaction Handlers** | Borrow unauthorized, missing idempotency key, borrow success + stock assertion, history pagination |
| **Transaction Usecases** | Borrow, max limit, insufficient stock, idempotency replay, return, late fine, wrong user, history, active loans |
| **Fine Calculator** | On-time, exact due date, 1 day late, 5 days late, partial day rounding |
| **Messaging** | Message structure, retry count logic, payload completeness, duplicate event idempotency |
| **PostgreSQL Repositories** | Real-Postgres idempotency concurrent insert, conflict detection (via `TEST_DATABASE_URL`) |
| **Integration** | Response format consistency, health/ready endpoint contracts |

### Code Quality

```bash
go vet ./...                        # static analysis
gofmt -l .                          # formatting (zero output = clean)
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.0 run
```

Linters enabled (`.golangci.yml`): `errcheck` · `govet` · `ineffassign` · `staticcheck` · `unused` — **0 issues**.

---

## CI Pipeline

Three parallel jobs on every push to `main`/`master` and every pull request:

```
┌──────────┐   ┌───────────────────┐   ┌──────────────────────────────┐
│  Lint    │   │  Test & Build     │   │  Postgres Integration        │
│          │   │                   │   │                              │
│ golangci │   │ gofmt check       │   │ Postgres 16 service          │
│ -lint    │   │ go test ./...     │   │ go test .../postgres         │
│ v2.12    │   │ go build ./...    │   │   -run Postgres              │
└──────────┘   └───────────────────┘   └──────────────────────────────┘
```

See [`.github/workflows/ci.yml`](.github/workflows/ci.yml) for the full configuration.

---

## Code Structure

```text
.github/workflows/
  ci.yml              # Lint · Test · Build · Postgres integration jobs
cmd/
  identity-api/       # Identity Service entrypoint + DI wiring
  book-api/           # Book Service entrypoint + DI wiring
  book-worker/        # RabbitMQ consumer entrypoint (reconnect loop)
  transaction-api/    # Transaction Service entrypoint + DI wiring
internal/
  platform/           # Shared infrastructure (no business logic)
    config/           # Env config with fail-fast validation
    database/         # PostgreSQL pgx connection pool
    httpserver/       # Fiber app factory (middlewares, error handler)
    logger/           # slog JSON logger
    middleware/       # Request-ID, recovery, logging, rate limiter
    rabbitmq/         # Connection, Publisher (confirms), Consumer (reconnect + DLQ)
    response/         # Consistent JSON envelope helpers
    apperror/         # Transport-independent application errors
    validation/       # Email, UUID validators
  auth/
    jwt/              # JWT sign + validate (token_type enforcement)
    password/         # bcrypt hash + verify
    middleware/       # JWTAuth, InternalAuth middlewares
  identity/
    domain/           # User, RefreshToken value objects + rules
    delivery/http/    # Handlers, DTOs, handler tests
    usecase/          # Register, Login, Refresh, Logout, Profile
    repository/postgres/
  book/
    domain/           # Book, BookStockEvent + stock rules
    delivery/http/    # Handlers, DTOs, handler tests
    usecase/          # List, Get, Create, Update, Stock
    repository/postgres/
    messaging/        # RabbitMQ stock event handler + tests
  transaction/
    domain/           # BorrowTransaction, Audit, Idempotency entities
    delivery/http/    # Handlers, DTOs, handler tests
    usecase/          # Borrow, Return, FineCalculator, History
    repository/postgres/  # Advisory-lock borrow, conditional return, idempotency
    client/book/      # Internal HTTP client for Book Service
    messaging/        # RabbitMQ stock event publisher
migrations/
  identity/           # 001 users, 002 refresh_tokens
  book/               # 001 books, 002 book_stock_events
  transaction/        # 001 borrow_transactions, 002 audits, 003 idempotency, 004 book_snapshot
docs/
  openapi.yaml        # Full OpenAPI 3.0 specification
  api-requests.http   # HTTP client request samples
scripts/
  seed.go             # Sample book seeder
tests/
  integration/        # Response format + health endpoint tests
.golangci.yml         # Linter configuration
Makefile              # Developer shortcuts
```

---

## Environment Variables

See `.env.example` for all variables. Key configuration:

| Variable | Default | Description |
|---|---|---|
| `SERVER_PORT` | `3000` | HTTP listen port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_NAME` | `kita` | Database name |
| `JWT_SECRET` | *(required)* | HMAC secret for JWT signing |
| `JWT_EXPIRY` | `15m` | Access token lifetime |
| `REFRESH_TOKEN_EXPIRY` | `168h` | Refresh token lifetime (7 days) |
| `INTERNAL_API_TOKEN` | *(required)* | Shared token for service-to-service calls |
| `BOOK_SERVICE_URL` | `http://localhost:3001` | Book Service URL (Transaction Service only) |
| `LOAN_DAYS` | `7` | Standard loan period |
| `DAILY_FINE_AMOUNT` | `500.00` | Fine per late day (parsed into integer cents) |
| `MAX_ACTIVE_BORROWS` | `3` | Max simultaneous borrowed books per user |
| `RABBITMQ_URL` | *(empty = disabled)* | RabbitMQ AMQP URL for async stock events |

---

## Demo Flow

Full end-to-end scenario:

```bash
# 1. Register
curl -X POST http://localhost:3000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Demo User","email":"demo@example.com","password":"password123"}'

# 2. Login → save access_token and refresh_token
curl -X POST http://localhost:3000/api/v1/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password&email=demo@example.com&password=password123"

# 3. List books
curl "http://localhost:3001/api/v1/books?page=1&per_page=5"

# 4. Check availability (copy a book_id)
curl http://localhost:3001/api/v1/books/<book_id>/availability

# 5. Borrow a book
curl -X POST http://localhost:3002/api/v1/transactions/borrow \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"book_id":"<book_id>","idempotency_key":"demo-borrow-1"}'

# 6. View active loans
curl http://localhost:3002/api/v1/transactions/active \
  -H "Authorization: Bearer <access_token>"

# 7. Return a book (copy transaction_id from step 5)
curl -X POST http://localhost:3002/api/v1/transactions/<transaction_id>/return \
  -H "Authorization: Bearer <access_token>"

# 8. View history (includes fine if returned late)
curl "http://localhost:3002/api/v1/transactions/history?page=1&per_page=10" \
  -H "Authorization: Bearer <access_token>"

# 9. Refresh access token
curl -X POST http://localhost:3000/api/v1/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token&refresh_token=<refresh_token>"

# 10. Logout (revokes only the submitted refresh token)
curl -X POST http://localhost:3000/api/v1/auth/logout \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token>"}'
```

---

## Flutter Integration Guide

This backend is consumed by the Flutter mobile app. Key integration points:

| Step | What to call | Notes |
|---|---|---|
| Register | `POST /api/v1/auth/register` | Returns `access_token` immediately |
| Login | `POST /api/v1/auth/token` (`grant_type=password`) | Returns `access_token` + `refresh_token` |
| Refresh | `POST /api/v1/auth/token` (`grant_type=refresh_token`) | Call when `401` is received |
| Protected calls | `Authorization: Bearer <access_token>` | Required on all Transaction endpoints |
| Borrow | `POST /api/v1/transactions/borrow` | Include unique `idempotency_key` per attempt |
| History | `GET /api/v1/transactions/history` | Supports `page` + `per_page` |

HTTP status codes the Flutter app must handle:

| Code | Meaning | Flutter action |
|---|---|---|
| `401` | Token expired / invalid | Refresh token, retry |
| `403` | Resource belongs to another user | Show error message |
| `409` | Business rule violation (max borrows, stock = 0, duplicate) | Show human-readable `error.message` |
| `404` | Resource not found | Show not-found state |
| `429` | Rate limit exceeded | Back off and retry |

---

## Known Limitations

- Services share a single PostgreSQL instance (separate databases, not separate servers)
- No cross-service database foreign keys (logical references only, by design for microservices)
- RabbitMQ async path is disabled by default in Docker Compose (`RABBITMQ_URL` unset for `transaction-api`)
- Compose auto-migration only runs on a **fresh Postgres volume**; existing volumes need manual migration
- Rate limiting is in-memory per service instance (no distributed rate limiter)
