ALTER TABLE stock_event_outbox
    ADD COLUMN IF NOT EXISTS compensation_for_event_type VARCHAR(20),
    ADD COLUMN IF NOT EXISTS compensation_reason TEXT;

ALTER TABLE stock_event_outbox
    DROP CONSTRAINT IF EXISTS stock_event_outbox_compensation_for_event_type_check;

ALTER TABLE stock_event_outbox
    ADD CONSTRAINT stock_event_outbox_compensation_for_event_type_check
    CHECK (compensation_for_event_type IS NULL OR compensation_for_event_type IN ('DECREASE', 'INCREASE'));
