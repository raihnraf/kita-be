CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS borrow_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_ref VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL,
    book_id UUID NOT NULL,
    borrowed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    due_at TIMESTAMP WITH TIME ZONE NOT NULL,
    returned_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    fine_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    late_days INT NOT NULL DEFAULT 0,
    stock_event_id UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_borrow_transactions_ref ON borrow_transactions(transaction_ref);
CREATE INDEX idx_borrow_transactions_user_status ON borrow_transactions(user_id, status);
CREATE INDEX idx_borrow_transactions_book_status ON borrow_transactions(book_id, status);
CREATE INDEX idx_borrow_transactions_due_at ON borrow_transactions(due_at);
CREATE INDEX idx_borrow_transactions_returned_at ON borrow_transactions(returned_at);
