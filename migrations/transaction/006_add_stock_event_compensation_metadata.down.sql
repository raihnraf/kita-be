ALTER TABLE stock_event_outbox
    DROP CONSTRAINT IF EXISTS stock_event_outbox_compensation_for_event_type_check;

ALTER TABLE stock_event_outbox
    DROP COLUMN IF EXISTS compensation_reason,
    DROP COLUMN IF EXISTS compensation_for_event_type;
