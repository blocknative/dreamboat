CREATE TABLE IF NOT EXISTS validator_registrations (
	pubkey VARCHAR(98) NOT NULL,
    fee_recipient VARCHAR(42) NOT NULL,
    gas_limit bigint NOT NULL,
    reg_time timestamp,
    signature VARCHAR NOT NULL,
	UNIQUE(pubkey)
);
