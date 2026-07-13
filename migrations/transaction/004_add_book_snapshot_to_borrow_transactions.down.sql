ALTER TABLE borrow_transactions
    DROP COLUMN IF EXISTS book_author,
    DROP COLUMN IF EXISTS book_title,
    DROP COLUMN IF EXISTS book_isbn;
