ALTER TABLE borrow_transactions
    DROP CONSTRAINT IF EXISTS chk_borrow_transactions_status;

ALTER TABLE borrow_transactions
    ADD CONSTRAINT chk_borrow_transactions_status
    CHECK (status IN ('ACTIVE', 'RETURNED', 'RETURNED_LATE'));
