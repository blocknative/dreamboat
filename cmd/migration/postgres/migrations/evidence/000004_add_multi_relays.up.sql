ALTER TABLE payload_delivered ADD COLUMN IF NOT EXISTS relay_id smallint NOT NULL DEFAULT 0;
ALTER TABLE payload_delivered DROP CONSTRAINT IF EXISTS payload_delivered_slot_proposer_pubkey_block_hash_key;
ALTER TABLE payload_delivered ADD UNIQUE(relay_id, slot, proposer_pubkey, block_hash);

ALTER TABLE builder_block_submission ADD COLUMN IF NOT EXISTS relay_id smallint NOT NULL DEFAULT 0;
ALTER TABLE builder_block_submission DROP CONSTRAINT IF EXISTS  builder_block_submission_slot_proposer_pubkey_block_hash_key;
ALTER TABLE builder_block_submission ADD UNIQUE(relay_id, slot, proposer_pubkey, block_hash);