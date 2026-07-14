# Backend Submission Checklist

Document Role: Primary reviewer entry point  
Scope: Backend only  
Status: Primary current document

Scope dokumen ini adalah backend-only untuk repository `kita-be`.

## Current Status

Backend saat ini siap untuk review submission dari sisi backend. Fokus review yang disarankan adalah correctness, arsitektur, dan kejelasan flow async antar service.

## Verified Commands

Berikut command yang sudah diverifikasi lulus pada pass terakhir tanggal `2026-07-14`:

```bash
go test ./...
go test -race ./...
go build ./...
docker compose config
```

Verifikasi live backend yang opsional tetapi lebih kuat:

```bash
RUN_LIVE_BACKEND_INTEGRATION=1 go test -v ./tests/integration -run TestAsyncBorrowReturnLiveFlow
```

## Fast Reviewer Path

Urutan baca tercepat untuk reviewer backend:

1. `README.md`
2. `docs/openapi.yaml`
3. `SUBMISSION_NARRATIVE.md`
4. `audit.md`
5. `audit_final.md` jika ingin melihat jejak keputusan arsitektur dan audit akhir yang lebih panjang

Dokumen yang tidak perlu dijadikan entry point review:

- `audit_2.md` adalah snapshot review pre-fix
- `VERIFICATION.md` adalah bukti verifikasi detail, bukan ringkasan reviewer utama
- `UPGRADE_REPORT.md` adalah dokumen pendukung, bukan status sheet utama

## Document Taxonomy

- Primary current documents: `BACKEND_SUBMISSION_CHECKLIST.md`, `README.md`, `SUBMISSION_NARRATIVE.md`, `audit.md`
- Supporting current documents: `BACKEND_DEMO_SCRIPT.md`, `audit_final.md`, `UPGRADE_REPORT.md`, `VERIFICATION.md`
- Archived historical documents: `audit_2.md`

## Important Runtime Notes

- Borrow bersifat async: response awal mengembalikan transaksi dengan status `PENDING`.
- Return bersifat async: response awal mengembalikan transaksi dengan status `RETURN_PENDING`.
- Final state borrow ditentukan setelah result event diproses: `ACTIVE` atau `CANCELLED`.
- Final state return ditentukan setelah result event diproses: `RETURNED`, `RETURNED_LATE`, atau kembali ke `ACTIVE` jika stock restore ditolak.
- Client perlu poll `GET /api/v1/transactions/{id}` atau refresh history/active list untuk melihat final state.

## Architecture Review Guide

Area paling relevan untuk menilai keputusan engineering:

1. `internal/transaction/usecase/borrow.go`
2. `internal/transaction/usecase/return.go`
3. `internal/transaction/messaging/outbox_dispatcher.go`
4. `internal/transaction/messaging/result_handler.go`
5. `internal/transaction/messaging/reconciler.go`
6. `internal/book/messaging/`

Yang perlu dicermati reviewer:

- Book Service tetap menjadi pemilik stok tunggal.
- Transaction Service menyimpan intent transaksi dan outbox secara atomik.
- RabbitMQ dipakai untuk command/result flow, bukan hanya bonus tempelan.
- Duplicate delivery aman karena processing stok idempoten.
- Reconciliation berperan sebagai safety net untuk transaksi macet, bukan jalur bisnis utama.

## Remaining Optional Polish

Masih ada satu follow-up yang sifatnya confidence-building, bukan blocker submission:

1. Tambah integration test end-to-end dengan RabbitMQ nyata.
