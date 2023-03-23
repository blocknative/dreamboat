ALTER TABLE payload_delivered DROP COLUMN IF EXISTS relay_id;
ALTER TABLE payload_delivered DROP CONSTRAINT IF EXISTS payload_delivered_relay_id_slot_proposer_pubkey_block_hash_key;
ALTER TABLE payload_delivered ADD UNIQUE(slot, proposer_pubkey, block_hash);

ALTER TABLE builder_block_submission DROP COLUMN IF EXISTS relay_id;
ALTER TABLE builder_block_submission DROP CONSTRAINT IF EXISTS builder_block_submission_relay_id_slot_proposer_pubkey_block_hash_key;
ALTER TABLE builder_block_submission ADD UNIQUE(slot, proposer_pubkey, block_hash);