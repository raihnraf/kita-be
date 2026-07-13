CREATE TABLE IF NOT EXISTS book_stock_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id UUID NOT NULL,
    book_id UUID NOT NULL,
    transaction_id UUID,
    event_type VARCHAR(20) NOT NULL,
    quantity INT NOT NULL DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    error_message TEXT,
    processed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_book_stock_events_event_id ON book_stock_events(event_id);
CREATE UNIQUE INDEX idx_book_stock_events_transaction_type ON book_stock_events(transaction_id, event_type);
CREATE INDEX idx_book_stock_events_book_created ON book_stock_events(book_id, created_at);
CREATE INDEX idx_book_stock_events_transaction ON book_stock_events(transaction_id);
CREATE INDEX idx_book_stock_events_status ON book_stock_events(status);
