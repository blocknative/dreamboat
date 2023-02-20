ALTER TABLE builder_block_submission ADD COLUMN IF NOT EXISTS num_tx integer NOT NULL DEFAULT 0;
