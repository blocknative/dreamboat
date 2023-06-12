package dspostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/blocknative/dreamboat/structs"
)

func (s *Datastore) PutDelivered(ctx context.Context, slot structs.Slot, payload structs.DeliveredTrace, ttl time.Duration) (err error) {
	_, err = s.DB.ExecContext(ctx, `INSERT INTO payload_delivered
	( relay_id, slot, epoch, builder_pubkey, proposer_pubkey, proposer_fee_recipient, parent_hash, block_hash, num_tx, block_number, gas_used, gas_limit, value )
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (relay_id, slot, proposer_pubkey, block_hash)
		DO UPDATE SET
		parent_hash = EXCLUDED.parent_hash,
		builder_pubkey = EXCLUDED.builder_pubkey,
		proposer_fee_recipient = EXCLUDED.proposer_fee_recipient,
		gas_used  = EXCLUDED.gas_used,
		gas_limit = EXCLUDED.gas_limit,
		value = EXCLUDED.value,
		epoch = EXCLUDED.epoch,
		num_tx = EXCLUDED.num_tx,
		block_number = EXCLUDED.block_number,
		inserted_at = NOW()`,
		s.RelayID, uint64(slot), uint64(slot)/uint64(SlotsPerEpoch), payload.Trace.BidTraceExtended.BuilderPubkey.String(), payload.Trace.BidTraceExtended.ProposerPubkey.String(),
		payload.Trace.BidTraceExtended.ProposerFeeRecipient.String(), payload.Trace.BidTraceExtended.ParentHash.String(), payload.Trace.BidTraceExtended.BlockHash.String(),
		payload.Trace.BidTraceExtended.NumTx, payload.BlockNumber, payload.Trace.BidTraceExtended.GasUsed, payload.Trace.BidTraceExtended.GasLimit,
		payload.Trace.BidTraceExtended.Value.String())
	return err
}

func (s *Datastore) GetDeliveredPayloads(ctx context.Context, w io.Writer, headSlot uint64, queryArgs structs.PayloadTraceQuery) error {
	var i = 1
	parts := []string{"relay_id = $" + strconv.Itoa(i)}
	data := []interface{}{s.RelayID}
	i++

	if queryArgs.Slot > 0 {
		parts = append(parts, "slot = $"+strconv.Itoa(i))
		data = append(data, queryArgs.Slot)
		i++
	} else if queryArgs.Cursor > 0 {
		parts = append(parts, "slot <= $"+strconv.Itoa(i))
		data = append(data, queryArgs.Cursor)
		i++
	}

	if queryArgs.BlockHash != Emptybytes32 {
		parts = append(parts, "block_hash = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BlockHash.String())
		i++
	}

	if queryArgs.BlockNum > 0 {
		parts = append(parts, "block_number = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BlockNum)
		i++
	}

	if queryArgs.ProposerPubkey != Emptybytes48 {
		parts = append(parts, "proposer_pubkey = $"+strconv.Itoa(i))
		data = append(data, queryArgs.ProposerPubkey.String())
		i++
	}

	if queryArgs.BuilderPubkey != Emptybytes48 {
		parts = append(parts, "builder_pubkey = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BuilderPubkey.String())
		i++
	}

	qBuilder := strings.Builder{}
	qBuilder.WriteString(`SELECT slot, builder_pubkey, proposer_pubkey, proposer_fee_recipient, parent_hash, block_hash, block_number, num_tx, value, gas_used, gas_limit FROM payload_delivered `)

	if len(parts) > 0 {
		qBuilder.WriteString(" WHERE ")
		for i, par := range parts {
			if i != 0 {
				qBuilder.WriteString(" AND ")
			}
			qBuilder.WriteString(par)
		}
	}

	if queryArgs.OrderByValue > 0 {
		qBuilder.WriteString(` ORDER BY value ASC `)
	} else if queryArgs.OrderByValue < 0 {
		qBuilder.WriteString(` ORDER BY value DESC `)
	} else {
		qBuilder.WriteString(` ORDER BY slot DESC, inserted_at DESC `)
	}

	if queryArgs.Limit > 0 {
		qBuilder.WriteString(` LIMIT $` + strconv.Itoa(i))
		data = append(data, queryArgs.Limit)
	}

	rows, err := s.DB.QueryContext(ctx, qBuilder.String(), data...)
	switch {
	case err == sql.ErrNoRows:
		return json.NewEncoder(w).Encode([]structs.BidTraceExtended{})
	case err != nil:
		return fmt.Errorf("query error: %w", err)
	default:
	}

	defer rows.Close()

	var (
		builderpubkey        []byte
		proposerPubkey       []byte
		proposerFeeRecipient []byte
		parentHash           []byte
		blockHash            []byte
		value                []byte
	)

	encoder := json.NewEncoder(w)

	fmt.Fprint(w, "[") // Write the opening bracket manually
	idx := 0
	for rows.Next() {
		bt := structs.BidTraceExtended{}
		err = rows.Scan(&bt.Slot, &builderpubkey, &proposerPubkey, &proposerFeeRecipient, &parentHash, &blockHash, &bt.BlockNumber, &bt.NumTx, &value, &bt.GasUsed, &bt.GasLimit)
		if err != nil {
			fmt.Fprint(w, "]")
			return err
		}

		bt.BuilderPubkey.UnmarshalText(builderpubkey)
		bt.ProposerPubkey.UnmarshalText(proposerPubkey)
		bt.ProposerFeeRecipient.UnmarshalText(proposerFeeRecipient)
		bt.ParentHash.UnmarshalText(parentHash)
		bt.BlockHash.UnmarshalText(blockHash)
		bt.Value.UnmarshalText(value)

		if idx > 0{
			fmt.Fprint(w, ", ")
		}
		idx++
		
		if err := encoder.Encode(bt); err != nil {
			fmt.Fprint(w, "]")
			return err
		}
	}

	fmt.Fprint(w, "]") // Write the closing bracket manually
	return nil
}

/*
func (s *Datastore) CheckSlotDelivered(ctx context.Context, slot uint64) (bool, error) {
	var sl uint64
	err := s.DB.QueryRowContext(ctx, "SELECT slot FROM payload_delivered WHERE slot = $1 LIMIT 1", slot).Scan(&sl)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return sl == slot, err
}
*/

type GetDeliveredPayloadsFilters struct {
	Slot           uint64
	Cursor         uint64
	Limit          uint64
	BlockHash      string
	BlockNumber    uint64
	ProposerPubkey string
	BuilderPubkey  string
	OrderByValue   int8
}
