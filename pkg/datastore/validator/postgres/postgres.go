package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
)

type Datastore struct {
	DB *sql.DB
}

func NewDatastore(db *sql.DB) *Datastore {
	return &Datastore{DB: db}
}

func (s *Datastore) GetRegistration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT signature, fee_recipient, gas_limit, reg_time FROM validator_registrations WHERE pubkey = $1 LIMIT 1;`, pk.String())
	reg := types.SignedValidatorRegistration{
		Message: &types.RegisterValidatorRequestMessage{
			Pubkey: pk,
		},
	}
	var (
		signature    string
		feeRecipient string
		t            time.Time
	)

	if err := row.Scan(&signature, &feeRecipient, &reg.Message.GasLimit, &t); err == sql.ErrNoRows {
		return reg, datastore.ErrNotFound
	}

	reg.Message.Timestamp = uint64(t.Unix())

	decsig, err := hexutil.Decode(signature)
	if err != nil {
		return reg, err
	}

	if err = reg.Signature.FromSlice(decsig); err != nil {
		return reg, err
	}

	decRecip, err := hexutil.Decode(feeRecipient)
	if err != nil {
		return reg, err
	}

	if err = reg.Message.FeeRecipient.FromSlice(decRecip); err != nil {
		return reg, err
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
		reg.Message.Pubkey.String(), reg.Signature.String(), reg.Message.FeeRecipient.String(), reg.Message.GasLimit, time.Unix(int64(reg.Message.Timestamp), 0))
	return err
}
