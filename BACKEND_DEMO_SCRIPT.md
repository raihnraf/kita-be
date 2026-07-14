# Backend Demo Script

Document Role: Demo walkthrough aid  
Scope: Backend only  
Status: Supporting current document

Dokumen ini adalah naskah singkat untuk demo backend-only selama sekitar 3 sampai 5 menit.

## 1. Opening

"Repo ini adalah backend untuk sistem informasi perpustakaan. Scope repo ini memang backend-only, jadi fokus saya di sini adalah arsitektur microservices, konsistensi stok buku, autentikasi JWT, dan kualitas engineering di sisi backend."

## 2. Show The Service Split

"Sesuai soal, backend saya pecah menjadi tiga service utama: Identity Service untuk register/login/token, Book Service untuk katalog dan stok, dan Transaction Service untuk borrow/return/history/fine. Masing-masing service punya database boundary sendiri dan business logic dipisah dengan Clean Architecture: domain, usecase, repository, dan delivery." 

Jika perlu tunjukkan:

- `cmd/identity-api`
- `cmd/book-api`
- `cmd/transaction-api`
- `cmd/book-worker`
- `internal/`

## 3. Explain The Main Design Choice

"Keputusan utama di backend ini adalah stok tetap dimiliki Book Service sebagai single source of truth. Jadi Transaction Service tidak mengubah stok langsung. Saat user borrow atau return, Transaction Service hanya menyimpan intent transaksi dan outbox event, lalu Book Worker memproses mutasi stok lewat RabbitMQ. Setelah itu result event dipakai untuk memfinalisasi status transaksi." 

"Dengan pendekatan ini, saya menghindari dual write dan mengurangi risiko inkonsistensi jika ada kegagalan di tengah request lintas service."

## 4. Explain Borrow Flow

"Untuk borrow, request tidak langsung dianggap final. API membuat transaksi dengan status `PENDING`, lalu menyimpan internal outbox untuk stock decrease. Saat dipublish ke RabbitMQ, command ini memakai nama event yang eksplisit yaitu `DecreaseStockRequested`. Setelah Book Worker sukses mengurangi stok dan mem-publish result event, Transaction Service mengubah status menjadi `ACTIVE`. Jika ditolak, status menjadi `CANCELLED`."

"Artinya borrow di API memang async, dan client perlu cek detail transaksi untuk melihat final state. Tradeoff ini saya pilih supaya boundary service tetap bersih dan flow retry lebih aman."

## 5. Explain Return Flow

"Untuk return, pola yang sama dipakai. Request awal membuat status `RETURN_PENDING` dan internal outbox untuk stock increase. Saat dipublish ke RabbitMQ, command ini memakai nama event `IncreaseStockRequested`. Setelah Book Worker memproses restore stok dan mengirim result event eksplisit, Transaction Service memfinalisasi status menjadi `RETURNED` atau `RETURNED_LATE`. Kalau restore stok ditolak, transaksi direvert ke `ACTIVE`."

"Jadi borrow dan return sekarang konsisten, sama-sama event-driven, dan tidak bergantung pada HTTP stock mutation sinkron sebagai jalur utama bisnis."

## 6. Highlight Reliability Features

"Bagian yang saya prioritaskan untuk best practice adalah reliability. Saya pakai transactional outbox supaya perubahan state lokal dan intent publish event tersimpan atomik. Lalu saya tambahkan idempotency untuk request client dan juga idempotent stock processing di Book Service supaya duplicate delivery tidak menggandakan stok." 

"Selain itu ada retry, DLQ, publisher confirm, reconnect loop, dan reconciliation worker. Reconciliation di sini bukan jalur bisnis utama, tetapi safety net untuk transaksi yang terlalu lama macet di `PENDING` atau `RETURN_PENDING`."

## 7. Show Verification Evidence

"Untuk kualitas backend, saya verifikasi repo ini dengan `go test ./...`, `go test -race ./...`, `go build ./...`, dan `docker compose config`. Semua pass. Jadi selain feature jalan, saya juga cek race condition, buildability, dan validitas Compose configuration." 

Jika perlu tunjukkan dokumen:

- `BACKEND_SUBMISSION_CHECKLIST.md`
- `SUBMISSION_NARRATIVE.md`
- `audit.md`

## 8. Close

"Kalau diringkas, fokus backend ini bukan cuma memenuhi fitur dasar soal, tapi juga memastikan stok tetap konsisten antar service, retry aman, dan flow borrow/return mudah dijelaskan saat review. Untuk scope backend take-home, saya anggap desain ini cukup pragmatis dan sudah dekat ke best practice." 

## Optional Q&A Prep

Jika reviewer bertanya kenapa tidak dibuat sinkron penuh:

"Saya sengaja pilih async saga karena Book Service adalah pemilik stok, dan saya ingin menghindari kasus transaction row sudah tercatat tetapi mutasi stok lintas service gagal di tengah jalan. Dengan outbox + idempotent consumer, failure handling jadi lebih aman dan lebih mudah direkonsiliasi." 

Jika reviewer bertanya kenapa response borrow/return tidak langsung final:

"Karena saya memilih correctness lintas service dan reliability publish/consume event sebagai prioritas. Dari sisi API ini memang eventual consistency, tetapi state transiennya eksplisit dan sudah terdokumentasi." 
