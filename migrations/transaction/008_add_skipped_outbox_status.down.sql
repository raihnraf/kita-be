UPDATE stock_event_outbox SET status = 'FAILED' WHERE status = 'SKIPPED';
ALTER TABLE stock_event_outbox DROP CONSTRAINT IF EXISTS chk_stock_event_outbox_status;
ALTER TABLE stock_event_outbox ADD CONSTRAINT stock_event_outbox_status_check CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED'));
