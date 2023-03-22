ALTER TABLE payload_delivered ADD COLUMN IF NOT EXISTS num_tx integer NOT NULL DEFAULT 0;
