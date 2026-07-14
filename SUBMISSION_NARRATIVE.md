# Backend Architecture & Submission Narrative

Document Role: Architecture rationale  
Scope: Backend only  
Status: Primary current document

Dokumen ini ditujukan bagi reviewer untuk memberikan pemahaman cepat mengenai pilihan arsitektur, keputusan desain, dan pola penanganan konsistensi terdistribusi yang diterapkan pada sistem backend `kita-be`.

---

## 1. Pemisahan Layanan (3 Microservices)
Sesuai dengan ketentuan `soal.md`, sistem dipecah menjadi tiga service utama dengan batas domain (*boundary*) dan basis data terpisah (*database per service*):
*   **Identity Service**: Mengelola autentikasi (registrasi, login, JWT issuance, dan *refresh token rotation*).
*   **Book Service**: Memegang kendali penuh atas katalog buku dan ketersediaan stok buku.
*   **Transaction Service**: Mengelola transaksi peminjaman, pengembalian, riwayat, dan perhitungan denda.

**Prinsip Desain**:
*   Tidak ada *Foreign Key* fisik lintas database service. Hubungan antar entitas (misalnya `book_id` atau `user_id` di database transaksi) dikaitkan secara logis di tingkat aplikasi.
*   Setiap service hanya membaca dan menulis ke databasenya sendiri. Lintas komunikasi data antar service dilakukan secara asinkron atau menggunakan read-only API client terenkripsi.

---

## 2. Kepemilikan Stok oleh Book Service
Di dalam sistem terdistribusi, **Single Source of Truth (SSOT)** sangat penting untuk mencegah inkonsistensi data. 
*   **Book Service** adalah satu-satunya pemilik data stok. 
*   **Transaction Service** sama sekali tidak memiliki akses langsung untuk mengubah tabel buku. 
*   Ketika peminjaman atau pengembalian terjadi, Transaction Service mengirimkan perintah mutasi stok ke Book Service. Hal ini menjamin bahwa seluruh aturan validasi stok (misalnya stok tidak boleh kurang dari 0) hanya dikelola di satu tempat terpusat.

---

## 3. Desain Asinkron Saga (Async Borrow & Return)
Mengapa alur peminjaman (`borrow`) dan pengembalian (`return`) dibuat asinkron melalui RabbitMQ?

*   **Menghindari Sinkronisasi HTTP yang Ringkih**: Jika mutasi stok dilakukan secara sinkron via HTTP selama request transaksi, kegagalan jaringan atau matinya Book Service di tengah jalan akan menyebabkan kegagalan parsial (transaksi tercatat aktif tapi stok tidak berkurang, atau sebaliknya).
*   **Penyelesaian Masalah dengan Saga Pattern**:
    1.  **Intent Commit**: Transaction Service mencatat transaksi lokal dengan status transien (`PENDING` untuk borrow, `RETURN_PENDING` untuk return), mengembalikan respons HTTP `202 Accepted` kepada client, dan menyimpan pesan outbox mutasi stok secara atomik dalam satu transaksi basis data lokal.
    2.  **Asynchronous Processing**: Pesan outbox dikirim ke RabbitMQ secara asinkron sebagai command event yang eksplisit, misalnya `DecreaseStockRequested` atau `IncreaseStockRequested`, untuk diproses oleh **Book Worker**.
    3.  **Finalization**: Book Worker mengupdate stok dan mempublikasikan result event yang juga eksplisit, misalnya `DecreaseStockSucceeded`, `DecreaseStockRejected`, `IncreaseStockSucceeded`, atau `IncreaseStockRejected`. Event ini kemudian dikonsumsi oleh Transaction Service untuk memfinalisasi status akhir transaksi. Borrow berakhir di `ACTIVE` atau `CANCELLED`, sedangkan return berakhir di `RETURNED`, `RETURNED_LATE`, atau kembali ke `ACTIVE` jika stock restore ditolak.

---

## 4. Mekanisme Resiliensi & Konsistensi Akhir (*Eventual Consistency*)

Sistem mengandalkan empat pilar untuk menjamin keandalan dan konsistensi akhir data tanpa memblokir performa HTTP request:

### A. Transactional Outbox
Untuk mencegah *dual-write problem* (situasi di mana data tersimpan di DB tetapi gagal terpublikasi ke RabbitMQ), kami menggunakan pola **Transactional Outbox**. Pesan outbox ditulis ke tabel database yang sama dengan data transaksi. Sebuah dispatcher latar belakang secara berkala membaca pesan `PENDING` dari outbox dan mempublikasikannya ke RabbitMQ dengan jaminan *at-least-once delivery*.

### B. Idempotensi Konsumer (Deterministic Command Replay)
*   **Book Service** mencatat setiap hasil mutasi stok per transaksi menggunakan pasangan unik `(transaction_id, event_type)` pada tabel `book_stock_events`.
*   Jika RabbitMQ mengirimkan pesan duplikat (misalnya akibat kegagalan jaringan), Book Service akan mengenali transaksi tersebut telah diproses dan secara aman mengirimkan kembali (*replay*) hasil sukses/gagal yang sama tanpa mengubah nilai stok buku lagi.

### C. Retry Policy dan Dead Letter Queue (DLQ)
Pesan yang gagal diproses akibat masalah temporer akan dicoba kembali (*retry*). Jika pesan terus menerus gagal melampaui batas maksimal *retry*, pesan tersebut dipindahkan ke **Dead Letter Queue (DLQ)** agar tidak memblokir antrean utama (*non-blocking processing*) dan dapat diaudit secara manual.

### D. Reconciliation Worker (Safety Net)
Reconciliation Worker bertindak sebagai pengaman jika terjadi keterlambatan atau kegagalan pengiriman *result event* dari Book Service. 
*   Worker memindai transaksi yang menggantung di status `PENDING` atau `RETURN_PENDING` melampaui batas waktu tertentu (*threshold*).
*   Daripada melakukan *rollback* paksa secara manual yang bisa merusak konsistensi stok, worker menandai kembali command outbox terkait untuk dikirim ulang (*re-queue*). Berkat sifat idempotensi Book Service, pengiriman ulang ini sepenuhnya aman dan menjamin penyelesaian akhir state transaksi.
