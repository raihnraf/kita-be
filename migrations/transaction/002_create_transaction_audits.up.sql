CREATE TABLE IF NOT EXISTS transaction_audits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id UUID NOT NULL,
    from_status VARCHAR(20),
    to_status VARCHAR(20) NOT NULL,
    reason VARCHAR(255) NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_transaction_audits_transaction FOREIGN KEY (transaction_id) REFERENCES borrow_transactions(id) ON DELETE CASCADE
);

CREATE INDEX idx_transaction_audits_txn_created ON transaction_audits(transaction_id, created_at);
CREATE INDEX idx_transaction_audits_to_status ON transaction_audits(to_status);
