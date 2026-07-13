CREATE TABLE IF NOT EXISTS idempotency_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scope VARCHAR(100) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash VARCHAR(255) NOT NULL,
    response_payload JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'PROCESSING',
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_idempotency_scope_key ON idempotency_records(scope, idempotency_key);
CREATE INDEX idx_idempotency_expires ON idempotency_records(expires_at);
CREATE INDEX idx_idempotency_status ON idempotency_records(status);
