CREATE TABLE IF NOT EXISTS builder_block_submission (
	slot        bigint NOT NULL,
	parent_hash varchar(66) NOT NULL,
	block_hash  varchar(66) NOT NULL,

	builder_pubkey         varchar(98) NOT NULL,
	proposer_pubkey        varchar(98) NOT NULL,
	proposer_fee_recipient varchar(42) NOT NULL,

	gas_used   bigint NOT NULL,
	gas_limit  bigint NOT NULL,

	value  NUMERIC(48, 0),

	-- helpers
	epoch        bigint NOT NULL,
	block_number bigint NOT NULL,
	was_most_profitable boolean NOT NULL,
	block_time timestamp ,

	inserted_at timestamp NOT NULL default current_timestamp,

	UNIQUE (slot, proposer_pubkey, block_hash)
);

CREATE INDEX IF NOT EXISTS builder_block_submission_slot_idx ON builder_block_submission("slot");
CREATE INDEX IF NOT EXISTS builder_block_submission_slts_idx ON builder_block_submission("slot", "block_time");
CREATE INDEX IF NOT EXISTS builder_block_submission_ts_idx ON builder_block_submission("block_time");
CREATE INDEX IF NOT EXISTS builder_block_submission_blockhash_idx ON builder_block_submission("block_hash");
CREATE INDEX IF NOT EXISTS builder_block_submission_blocknumber_idx ON builder_block_submission("block_number");
CREATE INDEX IF NOT EXISTS builder_block_submission_builderpubkey_idx ON builder_block_submission("builder_pubkey");


CREATE TABLE IF NOT EXISTS payload_delivered (
	builder_pubkey         varchar(98) NOT NULL,
	proposer_pubkey        varchar(98) NOT NULL,
	proposer_fee_recipient varchar(42) NOT NULL,

	epoch bigint NOT NULL,
	slot  bigint NOT NULL,


	parent_hash  varchar(66) NOT NULL,
	block_hash   varchar(66) NOT NULL,
	block_number bigint NOT NULL,

	gas_used  bigint NOT NULL,
	gas_limit bigint NOT NULL,

	value   NUMERIC(48, 0),

	inserted_at timestamp NOT NULL default current_timestamp,

	UNIQUE (slot, proposer_pubkey, block_hash)
);

CREATE INDEX IF NOT EXISTS payload_delivered_slot_idx ON payload_delivered("slot");
CREATE INDEX IF NOT EXISTS payload_delivered_slbh_idx ON payload_delivered("slot","inserted_at");
CREATE INDEX IF NOT EXISTS payload_delivered_blockhash_idx ON payload_delivered("block_hash");
CREATE INDEX IF NOT EXISTS payload_delivered_blocknumber_idx ON payload_delivered("block_number");
CREATE INDEX IF NOT EXISTS payload_delivered_proposerpubkey_idx ON payload_delivered("proposer_pubkey");
CREATE INDEX IF NOT EXISTS payload_delivered_builderpubkey_idx ON payload_delivered("builder_pubkey");
CREATE INDEX IF NOT EXISTS payload_delivered_value_idx ON payload_delivered("value");
 