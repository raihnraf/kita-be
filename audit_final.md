# Final Audit: Status Akhir Rombak Arsitektur Backend

Document Role: Architecture decision-history summary  
Scope: Backend only  
Status: Supporting current document

> This document explains the end-state of the backend refactor and the path taken to get there. For the shortest current-state reviewer summary, prefer `audit.md`. For the fastest reviewer entry point, start at `BACKEND_SUBMISSION_CHECKLIST.md`.

Dokumen ini merangkum:

- apa yang tadinya direkomendasikan untuk dirombak
- apa yang sekarang sudah benar-benar diimplementasikan
- apa yang masih tersisa sebagai optional polish

Fokus audit tetap sama: backend harus lebih dekat ke best practice distributed systems, tetap pragmatis untuk take-home, dan tetap selaras dengan objective di `soal.md`.

## Tujuan

Target dari rombak ini bukan menambah fitur baru, tetapi:

- membuat alur borrow dan return lebih konsisten antar service
- mengurangi hybrid flow yang terlalu banyak pengecualian
- membuat RabbitMQ benar-benar menjadi mekanisme sinkronisasi async yang utama
- menjaga requirement `soal.md` tetap terpenuhi dengan arsitektur yang lebih rapi

## Ringkasan Eksekutif

Kondisi saat ini sudah masuk kategori kuat untuk submission backend dan jauh lebih dekat ke best practice daripada baseline sebelumnya.

Yang sekarang sudah benar:

- stack wajib terpenuhi: Go, Fiber, PostgreSQL, JWT
- service split terpenuhi: Identity, Book, Transaction
- RabbitMQ, Docker Compose, dan testing sudah ada
- borrow dan return sudah memakai model async yang seragam
- finalisasi status transaksi sudah event-driven
- reconciliation sudah menjadi safety net, bukan jalur utama bisnis

Arsitektur yang aktif sekarang:

- borrow membuat transaksi `PENDING` + internal outbox stock decrease
- return membuat transaksi `RETURN_PENDING` + internal outbox stock increase
- Book Worker memproses command stok secara idempoten
- Book Worker mem-publish result event ke Transaction Service
- Transaction Service memfinalkan state dari result event

Kesimpulan praktis:

- rombak besar yang tadinya direkomendasikan sudah dikerjakan di area inti
- backend sekarang lebih bersih, lebih konsisten, dan lebih mudah dijelaskan saat review
- yang tersisa sekarang mostly documentation polish dan verifikasi tambahan, bukan redesign utama lagi

## Acuan Soal

Poin `soal.md` yang paling relevan untuk rombak ini:

- microservices harus jelas
- Book Service bertanggung jawab atas stok
- Transaction Service bertanggung jawab atas borrow, return, dan fine
- stok harus tetap konsisten saat buku dipinjam atau dikembalikan
- RabbitMQ memberi nilai plus untuk komunikasi async antar service
- SOLID dan Clean Architecture wajib terlihat

Artinya, versi terbaik bukan distributed transaction yang kompleks, tetapi:

- setiap service tetap punya database sendiri
- setiap service hanya mengubah datanya sendiri
- komunikasi lintas service dilakukan dengan event/command async yang idempoten
- perubahan state lokal dan niat publish event disimpan atomik melalui transactional outbox

## Penilaian Kondisi Saat Ini

Yang sudah bagus:

- boundary antar service jelas
- usecase bergantung pada interface
- transactional outbox sudah nyata
- consumer idempotency untuk stock event sudah ada
- retry, DLQ, dan reconciliation sudah aktif
- borrow dan return sudah sama-sama event-driven
- finalisasi status transaksi sudah tidak bergantung pada HTTP stock mutation sinkron

Yang tadinya terasa tambalan, sekarang sudah dibersihkan:

- borrow tidak lagi memakai reservasi stok HTTP sinkron sebagai jalur utama
- return tidak lagi memakai stock restore HTTP sinkron sebagai jalur utama
- finalisasi transaksi sudah ditentukan oleh result event
- reconciliation sudah berubah dari kompensasi bisnis menjadi command re-dispatch

Kesimpulan:

- current architecture sudah cukup kuat untuk take-home
- best-practice end state untuk scope repo ini pada dasarnya sudah tercapai
- sisa pekerjaan sekarang bukan redesign inti, tetapi finishing dan validasi tambahan

## Yang Sudah Diimplementasikan

Bagian ini mencatat hasil akhir rombak yang sudah benar-benar ada di codebase.

### 1. Borrow Dan Return Sudah Diseragamkan

Sudah dilakukan:

- borrow request menyimpan `PENDING` + internal outbox stock decrease
- return request menyimpan `RETURN_PENDING` + internal outbox stock increase
- tidak ada lagi HTTP stock mutation sinkron sebagai jalur utama bisnis

### 2. Finalisasi Status Sudah Event-Driven

Sudah dilakukan:

- Book Worker publish result event setelah proses stok selesai
- Transaction Service consume result event
- borrow difinalkan menjadi `ACTIVE` atau `CANCELLED`
- return difinalkan menjadi `RETURNED`, `RETURNED_LATE`, atau direvert ke `ACTIVE`

### 3. RabbitMQ Topology Sudah Lebih Rapi

Sudah dilakukan:

- queue command dipisah dari queue result
- routing key command dan result sudah dibedakan
- publisher confirm, retry, dan DLQ tetap dipertahankan

### 4. Reconciliation Sudah Menjadi Safety Net

Sudah dilakukan:

- transaksi macet di `PENDING` atau `RETURN_PENDING` tidak langsung dikompensasi manual
- reconciliation worker me-requeue stock command agar result event diproduksi ulang secara aman

### 5. Deterministic Replay Sudah Ada

Sudah dilakukan:

- Book Service menyimpan hasil stock command per `(transaction_id, event_type)`
- sukses tetap sukses saat retry
- reject tetap reject saat retry
- retry tidak bergantung pada kondisi stok baru di waktu berbeda

### 6. Rule Soal Tentang Stok 0 Tetap Dijaga

Sudah dilakukan:

- borrow tetap melakukan precheck read-only terhadap data buku
- jika `can_borrow=false`, request ditolak lebih awal
- ini menjaga perilaku tetap sesuai `soal.md` tanpa kembali ke mutasi sinkron lintas service

## Target Arsitektur Yang Disarankan

Model yang disarankan adalah `intent-first async saga`.

Prinsipnya:

- Transaction Service menyimpan state transaksi lokal terlebih dahulu
- Transaction Service menyimpan outbox event dalam transaksi DB yang sama
- dispatcher mem-publish event ke RabbitMQ
- Book Service memproses event stok secara idempoten
- Book Service mem-publish hasil proses
- Transaction Service mengonsumsi result event dan memfinalkan status transaksi

Dengan model ini:

- tidak ada external side effect sebelum niat bisnis tersimpan durable
- tidak perlu kompensasi sinkron via HTTP yang rapuh
- alur borrow dan return punya pola yang sama
- reasoning lebih mudah saat code review atau demo

## Flow Ideal Yang Disarankan

### Borrow

Flow target:

1. API menerima request borrow.
2. Transaction Service validasi JWT, idempotency, dan max active borrow.
3. Transaction Service menyimpan row borrow dengan status `PENDING`.
4. Transaction Service menyimpan outbox command `DECREASE_STOCK_REQUESTED`.
5. Dispatcher publish command ke RabbitMQ.
6. Book Worker consume command.
7. Book Service mengurangi stok jika tersedia, secara idempoten.
8. Book Service publish result event.
9. Transaction Service consume result event.
10. Jika sukses, status menjadi `ACTIVE`.
11. Jika gagal, status menjadi `CANCELLED`.

### Return

Flow target:

1. API menerima request return.
2. Transaction Service validasi JWT, idempotency, ownership, dan state transaksi.
3. Transaction Service menyimpan status `RETURN_PENDING` atau status transien yang setara.
4. Transaction Service menyimpan outbox command `INCREASE_STOCK_REQUESTED`.
5. Dispatcher publish command ke RabbitMQ.
6. Book Worker consume command.
7. Book Service menaikkan stok secara idempoten.
8. Book Service publish result event.
9. Transaction Service consume result event.
10. Jika sukses, status menjadi `RETURNED` atau `RETURNED_LATE`.
11. Jika gagal, status error ditandai dan bisa direkonsiliasi.

## Status Transaksi Yang Disarankan

Status minimal yang lebih jelas:

- `PENDING`
- `ACTIVE`
- `CANCELLED`
- `RETURN_PENDING`
- `RETURNED`
- `RETURNED_LATE`

Jika ingin lebih eksplisit lagi:

- `BORROW_PENDING`
- `ACTIVE`
- `BORROW_CANCELLED`
- `RETURN_PENDING`
- `RETURNED`
- `RETURNED_LATE`

Rekomendasi pragmatis:

- gunakan `PENDING` untuk borrow
- tambahkan `RETURN_PENDING` untuk return
- jangan mencampur status transien dan status final tanpa nama yang jelas

## Event Contract Yang Disarankan

Daripada event generik `DECREASE` dan `INCREASE` saja, gunakan command/result contract yang eksplisit.

Event yang disarankan:

- `DecreaseStockRequested`
- `DecreaseStockSucceeded`
- `DecreaseStockRejected`
- `IncreaseStockRequested`
- `IncreaseStockSucceeded`
- `IncreaseStockRejected`

Payload minimal:

- `event_id`
- `transaction_id`
- `transaction_ref`
- `book_id`
- `user_id`
- `quantity`
- `occurred_at`
- `cause` atau `reason` untuk event gagal

Manfaatnya:

- command dan result terpisah jelas
- consumer tidak perlu menebak status akhir dari satu event mentah
- lebih mudah diaudit dan dites

## Perubahan Yang Dibutuhkan

Sebagian besar poin di bagian ini sekarang sudah selesai. Bagian ini dipertahankan sebagai catatan audit jejak keputusan.

### 1. Transaction Service

Status implementasi:

- selesai untuk borrow async saga
- selesai untuk return async saga
- selesai untuk consumer result event
- selesai untuk status `RETURN_PENDING`
- selesai untuk audit transaksi pada state async

Yang tadinya perlu diubah:

- borrow usecase jangan lagi menganggap HTTP fast path sebagai jalur utama
- return usecase diseragamkan ke pola event-driven
- tambahkan consumer result event untuk finalisasi transaksi
- tambahkan status `RETURN_PENDING` bila return ikut dijadikan async penuh
- audit transaksi mengikuti perubahan state async

Yang bagusnya dihapus atau diperkecil perannya:

- dependency pada mutasi stok sinkron untuk menentukan sukses final request
- logika hybrid yang terlalu bercabang antara fast path dan fallback

### 2. Book Service

Status implementasi:

- idempotent stock processing tetap dipertahankan
- result event publisher sudah ditambahkan
- reject result sekarang punya reason yang bisa dibawa pulang ke Transaction Service

Yang perlu dipertahankan:

- stock event idempotency
- unique constraint per `transaction_id` dan `event_type`
- conditional stock update

Yang perlu ditambah:

- publish result event setelah stok berhasil atau gagal diproses
- reason code yang jelas untuk reject, misalnya stok habis

### 3. RabbitMQ Layer

Status implementasi:

- command queue dan result queue sudah dipisah
- routing key command/result sudah dibedakan
- topology publisher dan consumer sudah diseragamkan

Yang perlu dipertahankan:

- publisher confirm
- retry
- DLQ
- idempotent consumer

Yang perlu dirapikan:

- pisahkan command queue dan result queue jika perlu
- perjelas routing key untuk borrow dan return result
- dokumentasikan contract event secara formal

### 4. Reconciliation

Status implementasi:

- reconciliation sudah diposisikan sebagai safety net
- action utama reconciliation sekarang adalah re-dispatch command

Reconciliation tetap penting, tetapi posisinya berubah.

Best practice-nya:

- reconciliation hanya menangani state macet
- reconciliation bukan jalur bisnis normal
- threshold dan action-nya eksplisit

Contoh:

- `PENDING` terlalu lama tanpa result event -> retry command
- `RETURN_PENDING` terlalu lama tanpa result event -> retry command

## Perubahan API Yang Disarankan

Ada dua opsi yang sama-sama valid.

### Opsi A: Pure Async API

- borrow return `202 Accepted`
- return return `202 Accepted`
- response berisi `transaction_id` dan `status`
- mobile polling ke detail/history untuk lihat final status

Kelebihan:

- arsitektur paling bersih
- paling konsisten dengan message-driven design

Kekurangan:

- UX mobile sedikit lebih kompleks

### Opsi B: Async Core Dengan Sync-Like UX

- backend tetap commit intent dulu
- request handler boleh menunggu singkat untuk result event
- jika result cepat datang, response bisa langsung final
- jika tidak, response kembali `PENDING`

Kelebihan:

- core arsitektur tetap benar
- UX lebih nyaman

Kekurangan:

- implementasi lebih rumit daripada pure async

Rekomendasi untuk repo ini:

- jika target utama adalah best practice murni, pilih Opsi A
- jika target utama adalah demo take-home yang tetap terasa responsif, pilih Opsi B

## Urutan Implementasi Yang Disarankan

Supaya tidak rewrite brutal, urutan terbaik:

1. Definisikan status final dan status transien yang baru.
2. Definisikan contract event command/result.
3. Ubah borrow menjadi pure async saga terlebih dahulu.
4. Tambahkan consumer result di Transaction Service.
5. Tambahkan publisher result di Book Service.
6. Setelah borrow stabil, seragamkan return ke pola yang sama.
7. Kecilkan peran HTTP internal stock mutation, atau hapus jika sudah tidak dibutuhkan.
8. Rapikan README, OpenAPI, dan request examples.

Kenapa borrow dulu:

- itu area yang paling sensitif terhadap correctness antar service
- itu juga area yang paling sering jadi bahan tanya reviewer senior

## Pengujian Yang Wajib Jika Arsitektur Dirubah

Kalau rombak ini dilakukan, test juga harus naik level.

Yang wajib ada:

- duplicate message delivery
- out-of-order delivery
- retry setelah broker atau consumer restart
- stok habis saat borrow
- borrow paralel lebih dari limit
- return paralel pada transaksi yang sama
- pending transaction yang macet lalu direkonsiliasi
- idempotency API tetap benar pada request ulang

Yang ideal dijalankan:

- `go test ./...`
- `go test -race ./...`
- integration test dengan Postgres dan RabbitMQ nyata

Status saat ini:

- `go test ./...` sudah lulus pada codebase hasil rombak
- `go test -race ./...` masih direkomendasikan sebagai verifikasi tambahan
- integration test dengan broker nyata masih menjadi optional follow-up yang bagus

## Yang Masih Tersisa

Yang tersisa sekarang bukan masalah arsitektur inti, tetapi penyempurnaan:

- ✅ update `docs/openapi.yaml` agar status `PENDING` dan `RETURN_PENDING` terdokumentasi jelas
- ✅ update contoh request/response agar reviewer tidak mengira borrow/return selalu final langsung
- ✅ jalankan `go test -race ./...` (data race di `reconciler_test.go` sudah diperbaiki)
- jika ingin lebih kuat lagi, tambah integration test end-to-end dengan RabbitMQ nyata

Semua poin di atas sifatnya finishing dan confidence-building, bukan bug correctness besar

## Rekomendasi Final

Setelah rombak ini selesai, rekomendasi terbaik bukan lagi redesign besar, tetapi:

- rapikan dokumentasi API
- tambah verifikasi non-happy-path di level integration
- siapkan narasi demo yang menjelaskan flow async dengan singkat dan percaya diri

Versi yang paling cocok dengan `soal.md` adalah versi yang:

- tetap microservice
- tetap clean architecture
- stok konsisten tanpa mengandalkan rollback sinkron yang rapuh
- benar-benar memakai RabbitMQ sebagai mekanisme sinkronisasi bonus yang meaningful

## Keputusan Praktis

Kalau harus memilih sekarang:

- untuk submission cepat: current code sudah cukup kuat
- untuk submission matang: current code sudah layak dibawa
- untuk submission paling rapi: tambahkan polish pada docs dan verifikasi tambahan

Kesimpulan akhir:

- rombak besar yang tadinya layak sekarang sudah dikerjakan di area inti
- best practice yang dipakai sekarang adalah `transaction intent first -> outbox -> RabbitMQ -> idempotent stock processing -> result event -> finalize transaction`
- untuk scope backend di `soal.md`, arsitektur ini sudah berada pada bentuk yang bersih, mudah dijelaskan, dan kuat secara engineering
