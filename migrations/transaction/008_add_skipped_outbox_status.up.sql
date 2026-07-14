ALTER TABLE stock_event_outbox DROP CONSTRAINT IF EXISTS stock_event_outbox_status_check;
ALTER TABLE stock_event_outbox ADD CONSTRAINT chk_stock_event_outbox_status CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED', 'SKIPPED'));
