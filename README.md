# Kita Library Backend

Document Role: Primary project overview  
Scope: Backend only  
Status: Primary current document

Backend untuk prototipe Sistem Informasi Perpustakaan sesuai `soal.md`.

Repository ini hanya berisi backend. Aplikasi mobile Flutter berada di repository terpisah dan menggunakan backend ini sebagai API server.

## Reviewer Quick Start

Jika ingin review backend ini dengan cepat, jalankan:

```bash
go test ./...
go test -race ./...
go build ./...
docker compose config
```

Dokumen primary current untuk reviewer backend:

- `BACKEND_SUBMISSION_CHECKLIST.md` untuk jalur review tercepat dan status verifikasi terakhir
- `BACKEND_DEMO_SCRIPT.md` untuk narasi demo backend 3-5 menit saat submission
- `SUBMISSION_NARRATIVE.md` untuk alasan desain arsitektur backend
- `README.md` untuk arsitektur, cara menjalankan, dan mapping ke `soal.md`
- `docs/openapi.yaml` untuk kontrak API
- `internal/` untuk implementasi Clean Architecture per service
- `audit.md` untuk ringkasan verifikasi backend saat ini
- `audit_final.md` untuk audit akhir arsitektur dan sisa polish yang non-blocking

Dokumen supporting current dan archived:

- `UPGRADE_REPORT.md` untuk ringkasan upgrade engineering yang membawa codebase ke state saat ini
- `VERIFICATION.md` untuk snapshot bukti verifikasi yang lebih panjang
- `audit_2.md` adalah arsip review pre-fix dan bukan representasi current state

Catatan penting reviewer: endpoint `borrow` dan `return` memang didesain async. Borrow mulai dari `PENDING`, return mulai dari `RETURN_PENDING`, lalu final state dilihat dari endpoint detail transaksi setelah result event diproses. Jika stock restore untuk return ditolak, status transaksi dapat kembali ke `ACTIVE`.

---

## Kesesuaian Dengan Soal

Bagian ini merangkum kebutuhan backend dari `soal.md` dan implementasinya di repository ini.

### Teknologi Wajib

| Kebutuhan | Status | Implementasi |
|---|---|---|
| Backend Golang + Fiber | ✅ | Service HTTP dibuat dengan Go dan Fiber v2 |
| Database PostgreSQL atau MySQL | ✅ | Menggunakan PostgreSQL 16 |
| Auth OAuth2 menggunakan JWT | ✅ | Endpoint token mendukung grant `password` dan `refresh_token`, access token berbasis JWT |
| SOLID Principle | ✅ | Pemisahan domain, usecase, repository, delivery, dan dependency via interface |
| Clean Architecture | ✅ | Business rule berada di layer usecase/domain, tidak bercampur dengan HTTP atau SQL |

### Service Backend

| Service | Tanggung Jawab | Status |
|---|---|---|
| Identity Service | Registrasi, login, token OAuth2/JWT, refresh token, logout, profil user | ✅ |
| Book Service | Katalog buku, detail buku, pencarian, manajemen stok, cek ketersediaan | ✅ |
| Transaction Service | Peminjaman, pengembalian, riwayat transaksi, transaksi aktif, perhitungan denda | ✅ |
| Book Worker | Consumer RabbitMQ untuk sinkronisasi stok secara async | ✅ Bonus |

### Aturan Bisnis Backend

| Aturan dari soal | Implementasi |
|---|---|
| Semua request ke Transaction Service harus menyertakan JWT valid | Endpoint transaksi menggunakan middleware JWT |
| User tidak boleh meminjam buku dengan stok 0 | Stok dicek saat proses borrow dan operasi pengurangan stok menolak stok kosong |
| Jika buku dipinjam, stok Book Service berkurang | Borrow dan return memakai alur DB-first + transactional outbox. Stok dimutasi oleh Book Worker melalui RabbitMQ, lalu status transaksi difinalkan dari result event yang idempoten |
| Maksimal 3 buku dipinjam sekaligus | Dibatasi oleh konfigurasi `MAX_ACTIVE_BORROWS` dan validasi di usecase |
| Pengembalian terlambat dikenakan denda | Denda dihitung berdasarkan `LOAN_DAYS` dan `DAILY_FINE_AMOUNT` |

### Nilai Plus

| Nilai plus dari soal | Status |
|---|---|
| RabbitMQ | ✅ Untuk event stok buku |
| Docker Compose | ✅ Menjalankan PostgreSQL, RabbitMQ, dan semua service backend |
| Unit Testing | ✅ Dapat dijalankan dengan `make test` atau `go test ./...` |

---

## Engineering Highlights

Selain fitur wajib dan nilai plus dari soal, backend ini juga menambahkan beberapa detail engineering agar sistem lebih aman saat retry, concurrency, dan integrasi antar service.

- **Idempotency dengan response replay**: request peminjaman dan pengembalian yang dikirim ulang dengan `idempotency_key` yang sama dapat mengembalikan response awal, sehingga retry dari client lebih aman dan tidak membuat transaksi atau mutasi stok ganda.
- **Advisory lock untuk limit borrow**: validasi batas pinjam aktif yang definitif dan pembuatan transaksi borrow dibungkus dengan PostgreSQL advisory lock per user agar request paralel tidak bisa melewati batas maksimal 3 buku.
- **Stock event idempotency**: constraint unik `(transaction_id, event_type)` pada `book_stock_events` memastikan stok tidak berubah dua kali untuk event transaksi yang sama, meskipun RabbitMQ mengirim duplicate message.
- **RabbitMQ publisher confirms**: publisher menunggu `ack` dari broker sebelum dianggap berhasil, sehingga kegagalan publish bisa terdeteksi dan dilog.
- **Transactional outbox untuk RabbitMQ**: Transaction Service menyimpan intent publish event stok di database yang sama dengan transaksi borrow/return, lalu dispatcher mem-publish dan retry sampai sukses.
- **RabbitMQ consumer reconnect loop**: consumer otomatis retry dan reconnect saat broker bermasalah; jalur reconnect broker memakai exponential backoff, sementara startup retry dibuat sederhana dengan delay tetap.
- **Dead Letter Queue**: message yang tetap gagal setelah retry tidak hilang diam-diam, tetapi masuk ke DLQ agar bisa diperiksa.
- **Async saga untuk borrow dan return**: request borrow membuat transaksi `PENDING` dan internal outbox operasi stock decrease; request return membuat transaksi `RETURN_PENDING` dan internal outbox operasi stock increase. Di boundary RabbitMQ, command/result memakai contract eksplisit seperti `DecreaseStockRequested`, `DecreaseStockSucceeded`, `IncreaseStockRequested`, dan `IncreaseStockRejected`. Book Worker memproses command stok secara idempoten lalu mem-publish result event, dan Transaction Service memfinalkan status transaksi dari result tersebut.
- **Deterministic command replay**: Book Service menyimpan hasil sukses atau gagal dari stock command per `(transaction_id, event_type)`. Jika message di-retry atau dipublish ulang oleh reconciliation worker, hasilnya tetap sama dan tidak tergantung kondisi stok terbaru di saat retry.
- **Reconciliation sebagai re-dispatch**: jika transaksi terlalu lama di `PENDING` atau `RETURN_PENDING`, reconciliation worker tidak langsung membatalkan bisnis. Worker akan menandai command outbox untuk dipublish ulang agar result event bisa diproduksi ulang secara aman.
- **Integer cents untuk uang**: denda dihitung, diekspos, dan disimpan langsung sebagai integer cents (`fine_amount_cents` dengan tipe `BIGINT` di PostgreSQL) untuk menghindari floating-point error dan overhead konversi database-ke-aplikasi.
- **Double-return protection**: proses return memakai idempotency replay, conditional update `WHERE status='ACTIVE'`, transactional outbox, dan stock event idempotency; request paralel tidak meninggalkan kenaikan stok ganda permanen.
- **Book snapshot saat borrow**: judul, penulis, dan ISBN disimpan ke transaksi saat peminjaman, sehingga history tetap stabil meskipun data buku berubah dan tidak perlu query Book Service per baris history.
- **Token type enforcement**: JWT membawa field `token_type`, sehingga access token dan refresh token tidak bisa saling tertukar penggunaannya.
- **Rate limiting**: endpoint auth dan write transaction dibatasi dengan Fiber rate limiter untuk mengurangi brute force dan request spam.
- **golangci-lint config tersedia**: konfigurasi lint mencakup `errcheck`, `govet`, `ineffassign`, `staticcheck`, dan `unused` untuk menjaga kualitas kode.

---

## Arsitektur Singkat

Backend dipisah menjadi beberapa service:

```text
Identity Service       Book Service              Transaction Service
Register/Login         Katalog/Detail/Stok       Borrow/Return/History
OAuth2 + JWT           Cek ketersediaan          Hitung denda
      |                       |                         |
      |                       |                         |
      +-----------------------+-------------------------+
                              |
                         PostgreSQL
                              |
                         RabbitMQ
                              |
                         Book Worker
```

Komunikasi antar service:

| Dari | Ke | Mekanisme | Keterangan |
|---|---|---|---|
| Flutter App | Identity Service | HTTP | Register, login, refresh token, profil |
| Flutter App | Book Service | HTTP | Melihat katalog, detail buku, stok |
| Flutter App | Transaction Service | HTTP + JWT | Peminjaman, pengembalian, riwayat |
| Transaction Service | Book Service | HTTP internal | Mengambil snapshot buku saat borrow dan health/readiness check |
| Transaction Service | RabbitMQ | AMQP | Publish explicit stock command events seperti `DecreaseStockRequested` dan `IncreaseStockRequested` |
| Book Worker | RabbitMQ | AMQP consumer + publisher | Consume command stok, proses idempoten, lalu publish result event ke Transaction Service |

---

## Tech Stack

| Komponen | Teknologi |
|---|---|
| Bahasa | Go |
| HTTP Framework | Fiber v2 |
| Database | PostgreSQL 16 |
| Auth | OAuth2-style token endpoint, JWT HS256, bcrypt |
| Message Broker | RabbitMQ |
| Container | Docker Compose |
| Dokumentasi API | OpenAPI di `docs/openapi.yaml` |
| Testing | Go test |
| Linting | golangci-lint |

---

## Cara Menjalankan

### Prasyarat

- Go
- Docker dan Docker Compose
- PostgreSQL client, jika menjalankan migrasi manual secara lokal

### Opsi 1: Docker Compose

Cara ini paling mudah untuk menjalankan seluruh backend.

```bash
cp .env.example .env
docker-compose up -d --build
```

Cek service:

```bash
curl http://localhost:3000/api/v1/ready
curl http://localhost:3001/api/v1/ready
curl http://localhost:3002/api/v1/ready
```

Port default:

| Service | URL |
|---|---|
| Identity Service | `http://localhost:3000` |
| Book Service | `http://localhost:3001` |
| Transaction Service | `http://localhost:3002` |
| RabbitMQ Management | `http://localhost:15672` |

Seeder data buku:

```bash
go run ./scripts/seed.go
```

### Verifikasi Alur End-to-End Otomatis (Saga & Messaging)

Setelah kontainer berjalan dan database di-seed, Anda dapat menjalankan skrip otomatisasi pengujian integrasi asinkron untuk memverifikasi alur Saga:

```bash
./scripts/verify_flow.sh
```

Skrip ini akan melakukan registrasi user baru, melakukan login, memilih buku dengan stok tersedia, meminta peminjaman (memantau transisi status `PENDING` -> `ACTIVE` secara asinkron), memverifikasi pengurangan stok di Book Service, meminta pengembalian (memantau transisi `RETURN_PENDING` -> `RETURNED`), dan memverifikasi pemulihan stok buku.

Catatan: pada volume PostgreSQL baru, file `docker/postgres/init.sql` akan membuat database service dan menjalankan migrasi awal secara otomatis.

### Opsi 2: Local Development

Jalankan infrastruktur:

```bash
docker-compose up -d postgres rabbitmq
cp .env.example .env
```

Jika memakai volume PostgreSQL baru dari Docker Compose, `docker/postgres/init.sql` sudah membuat database dan menjalankan migrasi. Jika memakai database lokal atau volume lama yang belum berisi database service, buat database berikut:

```bash
docker-compose exec postgres psql -U postgres -c "CREATE DATABASE kita_identity;"
docker-compose exec postgres psql -U postgres -c "CREATE DATABASE kita_book;"
docker-compose exec postgres psql -U postgres -c "CREATE DATABASE kita_transaction;"
```

Jika migrasi belum pernah dijalankan, jalankan migrasi berikut:

```bash
psql -h localhost -U postgres -d kita_identity -f migrations/identity/001_create_users.up.sql
psql -h localhost -U postgres -d kita_identity -f migrations/identity/002_create_refresh_tokens.up.sql
psql -h localhost -U postgres -d kita_book -f migrations/book/001_create_books.up.sql
psql -h localhost -U postgres -d kita_book -f migrations/book/002_create_book_stock_events.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/001_create_borrow_transactions.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/002_create_transaction_audits.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/003_create_idempotency_records.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/004_add_book_snapshot_to_borrow_transactions.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/005_create_stock_event_outbox.up.sql
psql -h localhost -U postgres -d kita_transaction -f migrations/transaction/006_add_stock_event_compensation_metadata.up.sql
```

Jalankan service di terminal terpisah dengan konfigurasi database dan port masing-masing:

```bash
SERVER_PORT=3000 DB_NAME=kita_identity make run-identity
SERVER_PORT=3001 DB_NAME=kita_book RABBITMQ_URL= make run-book
SERVER_PORT=3002 DB_NAME=kita_transaction BOOK_SERVICE_URL=http://localhost:3001 RABBITMQ_URL=amqp://guest:guest@localhost:5672/ make run-transaction
DB_NAME=kita_book RABBITMQ_URL=amqp://guest:guest@localhost:5672/ make run-worker
```

Catatan: `book-api` tetap memakai `RABBITMQ_URL` kosong karena mutasi stok utama masih memiliki fast path via HTTP internal. `transaction-api` juga menyimpan stock intent ke outbox RabbitMQ agar event yang gagal publish atau butuh retry tetap bisa diproses ulang secara durable, dan duplicate event tetap aman karena `book_stock_events` memiliki constraint unik `(transaction_id, event_type)`.

Perintah Makefile yang tersedia:

| Perintah | Fungsi |
|---|---|
| `make test` | Menjalankan semua test |
| `make test-verbose` | Menjalankan test dengan output verbose |
| `make build` | Build semua binary ke folder `bin/` |
| `make seed` | Menjalankan seeder buku |
| `make docker-up` | Menjalankan Docker Compose |
| `make docker-down` | Menghentikan Docker Compose |
| `make docker-logs` | Melihat log Docker Compose |
| `make clean` | Menghapus folder `bin/` |

---

## Konfigurasi Environment

Contoh konfigurasi tersedia di `.env.example`.

| Variable | Keterangan |
|---|---|
| `SERVER_PORT` | Port HTTP service |
| `DB_HOST` | Host PostgreSQL |
| `DB_PORT` | Port PostgreSQL |
| `DB_USER` | User PostgreSQL |
| `DB_PASSWORD` | Password PostgreSQL |
| `DB_NAME` | Nama database service |
| `DB_SSLMODE` | Mode SSL PostgreSQL |
| `JWT_SECRET` | Secret untuk signing JWT |
| `JWT_EXPIRY` | Masa aktif access token |
| `REFRESH_TOKEN_EXPIRY` | Masa aktif refresh token |
| `INTERNAL_API_TOKEN` | Token internal antar service |
| `BOOK_SERVICE_URL` | URL Book Service untuk Transaction Service |
| `LOAN_DAYS` | Lama peminjaman sebelum denda |
| `DAILY_FINE_AMOUNT` | Nominal denda per hari keterlambatan |
| `MAX_ACTIVE_BORROWS` | Maksimal buku aktif yang boleh dipinjam user |
| `RABBITMQ_URL` | URL RabbitMQ; kosong berarti jalur async tidak aktif |

---

## Dokumentasi API

Dokumentasi lengkap tersedia di:

- OpenAPI: `docs/openapi.yaml`
- Contoh request HTTP: `docs/api-requests.http`

### Identity Service

| Method | Endpoint | Auth | Fungsi |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | Public | Registrasi user baru |
| `POST` | `/api/v1/auth/token` | Public | Login dengan `grant_type=password` atau refresh token |
| `POST` | `/api/v1/auth/logout` | Public | Logout dengan revoke refresh token |
| `GET` | `/api/v1/users/me` | JWT | Melihat profil user login |
| `GET` | `/api/v1/health` | Public | Health check |
| `GET` | `/api/v1/ready` | Public | Readiness check |

### Book Service

| Method | Endpoint | Auth | Fungsi |
|---|---|---|---|
| `GET` | `/api/v1/books` | Public | List buku dengan pagination, search, dan filter kategori |
| `GET` | `/api/v1/books/:id` | Public | Detail buku |
| `GET` | `/api/v1/books/:id/availability` | Public | Cek stok tersedia |
| `POST` | `/api/v1/books` | Internal Token | Tambah buku |
| `PUT` | `/api/v1/books/:id` | Internal Token | Update buku |
| `POST` | `/api/v1/internal/books/:id/stock/decrease` | Internal Token | Kurangi stok saat peminjaman |
| `POST` | `/api/v1/internal/books/:id/stock/increase` | Internal Token | Tambah stok saat pengembalian |
| `GET` | `/api/v1/health` | Public | Health check |
| `GET` | `/api/v1/ready` | Public | Readiness check |

### Transaction Service

| Method | Endpoint | Auth | Fungsi |
|---|---|---|---|
| `POST` | `/api/v1/transactions/borrow` | JWT | Meminjam buku |
| `POST` | `/api/v1/transactions/:id/return` | JWT | Mengembalikan buku; body wajib menyertakan `idempotency_key` |
| `GET` | `/api/v1/transactions/history` | JWT | Melihat riwayat transaksi |
| `GET` | `/api/v1/transactions/active` | JWT | Melihat buku yang sedang dipinjam |
| `GET` | `/api/v1/transactions/:id` | JWT | Detail transaksi |
| `GET` | `/api/v1/internal/transactions` | Internal Token | List semua transaksi untuk internal/admin |
| `GET` | `/api/v1/internal/transactions/:id` | Internal Token | Detail transaksi internal/admin |
| `GET` | `/api/v1/internal/transactions/:id/audits` | Internal Token | Audit trail transaksi |
| `GET` | `/api/v1/health` | Public | Health check |
| `GET` | `/api/v1/ready` | Public | Readiness check |

Format response umum:

```json
{
  "success": true,
  "data": {}
}
```

Format error umum:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "email is required"
  }
}
```

---

## Demo Flow Backend

Contoh alur end-to-end menggunakan `curl`.

### 1. Register

```bash
curl -X POST http://localhost:3000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Demo User","email":"demo@example.com","password":"password123"}'
```

### 2. Login

```bash
curl -X POST http://localhost:3000/api/v1/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password&email=demo@example.com&password=password123"
```

Simpan `access_token` dan `refresh_token` dari response.

### 3. Lihat Katalog Buku

```bash
curl "http://localhost:3001/api/v1/books?page=1&per_page=5"
```

### 4. Cek Stok Buku

```bash
curl http://localhost:3001/api/v1/books/<book_id>/availability
```

### 5. Pinjam Buku

```bash
curl -X POST http://localhost:3002/api/v1/transactions/borrow \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"book_id":"<book_id>","idempotency_key":"demo-borrow-1"}'
```

### 6. Lihat Buku Yang Sedang Dipinjam

```bash
curl http://localhost:3002/api/v1/transactions/active \
  -H "Authorization: Bearer <access_token>"
```

### 7. Kembalikan Buku

```bash
curl -X POST http://localhost:3002/api/v1/transactions/<transaction_id>/return \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"idempotency_key":"demo-return-1"}'
```

> **Catatan async:** response borrow akan berstatus `PENDING` dan response return akan berstatus `RETURN_PENDING`. Final state (`ACTIVE`, `CANCELLED`, `RETURNED`, `RETURNED_LATE`, atau kembali ke `ACTIVE` jika pengembalian ditolak) ditentukan setelah Book Worker memproses command stok dan Transaction Service menerima result event. Untuk menunggu final state, poll endpoint detail transaksi:
>
> ```bash
> curl http://localhost:3002/api/v1/transactions/<transaction_id> \
>   -H "Authorization: Bearer <access_token>"
> ```

### 8. Lihat Riwayat

```bash
curl "http://localhost:3002/api/v1/transactions/history?page=1&per_page=10" \
  -H "Authorization: Bearer <access_token>"
```

### 9. Refresh Access Token

```bash
curl -X POST http://localhost:3000/api/v1/auth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token&refresh_token=<refresh_token>"
```

### 10. Logout

```bash
curl -X POST http://localhost:3000/api/v1/auth/logout \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token>"}'
```

---

## Penerapan SOLID

### Single Responsibility Principle

Setiap layer punya tanggung jawab yang jelas:

- `delivery/http` menangani request/response HTTP.
- `usecase` menjalankan alur bisnis seperti register, login, borrow, return, dan history.
- `domain` menyimpan entity dan aturan domain.
- `repository/postgres` hanya menangani akses data PostgreSQL.
- `auth/jwt` dan `auth/password` memisahkan logic token dan password.

### Open/Closed Principle

Kode dibuat mudah diperluas tanpa mengubah banyak logic lama:

- Perhitungan denda dipisahkan dalam `FineCalculator`.
- Publisher event stok menggunakan interface, sehingga transport dapat diganti.
- Middleware Fiber dapat ditambah tanpa mengubah usecase.

### Liskov Substitution Principle

Usecase bergantung pada interface, sehingga implementasi asli dapat diganti oleh fake/mock pada test tanpa mengubah behavior yang diharapkan.

### Interface Segregation Principle

Interface dibuat sesuai kebutuhan usecase. Contohnya Borrow Usecase hanya mengenal interface repository transaksi, client Book Service, dan publisher event stok yang memang dibutuhkan.

### Dependency Inversion Principle

Layer bisnis tidak bergantung langsung pada detail PostgreSQL, RabbitMQ, atau HTTP client. Implementasi detail dihubungkan melalui constructor injection di `cmd/*/main.go`.

---

## Struktur Folder

```text
cmd/
  identity-api/        entrypoint Identity Service
  book-api/            entrypoint Book Service
  transaction-api/     entrypoint Transaction Service
  book-worker/         entrypoint RabbitMQ consumer
internal/
  auth/                JWT, password hashing, auth middleware
  identity/            domain, handler, usecase, repository Identity Service
  book/                domain, handler, usecase, repository Book Service
  transaction/         domain, handler, usecase, repository Transaction Service
  platform/            config, database, logger, response, middleware, RabbitMQ
migrations/
  identity/            migrasi database Identity Service
  book/                migrasi database Book Service
  transaction/         migrasi database Transaction Service
docs/
  openapi.yaml         spesifikasi OpenAPI
  api-requests.http    contoh request HTTP
scripts/
  seed.go              seeder data buku
```

---

## Testing Dan Kualitas Kode

Menjalankan semua test:

```bash
make test
```

Atau langsung:

```bash
go test ./...
```

Menjalankan test verbose:

```bash
make test-verbose
```

Static analysis dan formatting:

```bash
go vet ./...
gofmt -l .
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.0 run
```

Area yang ditest mencakup:

- JWT dan validasi token.
- Password hashing dan verification.
- Handler Identity, Book, dan Transaction.
- Usecase register, login, borrow, return, history, active loan, dan denda.
- Domain rule stok buku.
- Idempotency transaksi.
- Messaging RabbitMQ.
- Repository PostgreSQL tertentu dengan integration test.

---

## Catatan Integrasi Flutter

Backend ini disiapkan untuk digunakan oleh aplikasi Flutter.

Endpoint utama untuk mobile:

| Fitur mobile dari soal | Endpoint backend |
|---|---|
| Login | `POST /api/v1/auth/token` |
| Register | `POST /api/v1/auth/register` |
| Katalog buku | `GET /api/v1/books` |
| Detail dan stok buku | `GET /api/v1/books/:id`, `GET /api/v1/books/:id/availability` |
| Peminjaman | `POST /api/v1/transactions/borrow` |
| Pengembalian | `POST /api/v1/transactions/:id/return` dengan body `idempotency_key` |
| History transaksi | `GET /api/v1/transactions/history` |

Status code penting untuk ditangani aplikasi mobile:

| Status | Arti |
|---|---|
| `401` | Token tidak valid atau expired |
| `403` | User tidak punya akses ke resource |
| `404` | Resource tidak ditemukan |
| `409` | Konflik aturan bisnis, misalnya stok habis atau limit pinjam tercapai |
| `429` | Rate limit tercapai |

---

## Batasan

- Repository ini hanya backend, sehingga penilaian UI/UX Flutter berada di repository frontend.
- Service menggunakan satu instance PostgreSQL dengan database terpisah per service.
- Tidak ada foreign key lintas service karena referensi antar service dibuat secara logical.
- Jalur async RabbitMQ tersedia sebagai bonus dan menjadi jalur utama sinkronisasi stok pada Docker Compose default. API borrow mengembalikan transaksi `PENDING`, API return mengembalikan transaksi `RETURN_PENDING`, dan final state transaksi diselesaikan setelah result event dari Book Worker diterima. Event async aman dari mutasi ganda melalui idempotency `(transaction_id, event_type)` dan dapat dipublish ulang oleh reconciliation worker bila result event terlambat.
- Auto-migration Docker berjalan saat volume PostgreSQL masih baru; volume lama perlu migrasi manual.
