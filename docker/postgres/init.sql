SELECT 'CREATE DATABASE kita_identity'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'kita_identity')\gexec

SELECT 'CREATE DATABASE kita_book'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'kita_book')\gexec

SELECT 'CREATE DATABASE kita_transaction'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'kita_transaction')\gexec

\connect kita_identity
\i /migrations/identity/001_create_users.up.sql
\i /migrations/identity/002_create_refresh_tokens.up.sql

\connect kita_book
\i /migrations/book/001_create_books.up.sql
\i /migrations/book/002_create_book_stock_events.up.sql

\connect kita_transaction
\i /migrations/transaction/001_create_borrow_transactions.up.sql
\i /migrations/transaction/002_create_transaction_audits.up.sql
\i /migrations/transaction/003_create_idempotency_records.up.sql
\i /migrations/transaction/004_add_book_snapshot_to_borrow_transactions.up.sql
\i /migrations/transaction/005_create_stock_event_outbox.up.sql
