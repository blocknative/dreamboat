package postgres

import (
	"context"
	"database/sql"

	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/flashbots/go-boost-utils/types"
)

type Datastore struct {
	DB *sql.DB
}

func NewDatastore(db *sql.DB) *Datastore {
	return &Datastore{DB: db}
}

func (s *Datastore) GetRegistration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT signature, fee_recipient, gas_limit, reg_time FROM validator_registrations WHERE pubkey = $1 LIMIT 1;`, pk)
	reg := types.SignedValidatorRegistration{Message: &types.RegisterValidatorRequestMessage{}}

	err := row.Scan(&reg.Signature, &reg.Message.FeeRecipient, &reg.Message.GasLimit, &reg.Message.Timestamp)
	if err == sql.ErrNoRows {
		return reg, datastore.ErrNotFound
	}
	return reg, err
}

func (s *Datastore) PutNewerRegistration(ctx context.Context, pk types.PublicKey, reg types.SignedValidatorRegistration) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO validator_registrations(pubkey, signature, fee_recipient, gas_limit, reg_time, updated_at)
										VALUES ($1, $2, $3, $4, $5, NOW())
										ON CONFLICT (pubkey)
										DO UPDATE SET
										signature = EXCLUDED.signature, fee_recipient = EXCLUDED.fee_recipient,
										gas_limit = EXCLUDED.gas_limit, reg_time = EXCLUDED.reg_time, updated_at = NOW() 
									WHERE validator_registrations.reg_time < EXCLUDED.reg_time`,
		reg.Message.Pubkey, reg.Signature, reg.Message.FeeRecipient, reg.Message.GasLimit, reg.Message.Timestamp)
	return err
}
