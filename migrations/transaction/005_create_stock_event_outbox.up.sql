CREATE TABLE IF NOT EXISTS stock_event_outbox (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type VARCHAR(20) NOT NULL CHECK (event_type IN ('DECREASE', 'INCREASE')),
    transaction_id UUID NOT NULL,
    transaction_ref VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL,
    book_id UUID NOT NULL,
    quantity INT NOT NULL DEFAULT 1 CHECK (quantity > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED')),
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT,
    next_attempt_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    published_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_stock_event_outbox_transaction_type ON stock_event_outbox(transaction_id, event_type);
CREATE INDEX idx_stock_event_outbox_due ON stock_event_outbox(status, next_attempt_at, created_at);
CREATE INDEX idx_stock_event_outbox_transaction ON stock_event_outbox(transaction_id);
