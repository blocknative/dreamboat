package dspostgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/blocknative/dreamboat/structs"
)

func (s *Datastore) PutBuilderBlockSubmission(ctx context.Context, bid structs.BidTraceWithTimestamp, isMostProfitable bool) (err error) {
	_, err = s.DB.ExecContext(ctx, `INSERT INTO builder_block_submission
	( relay_id, slot, parent_hash, block_hash, builder_pubkey, proposer_pubkey, proposer_fee_recipient,
	  gas_used, gas_limit, value, epoch, num_tx, block_number, was_most_profitable, block_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
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
		was_most_profitable = EXCLUDED.was_most_profitable,
		block_time = EXCLUDED.block_time,
		inserted_at = NOW()`,
		s.RelayID, bid.Slot, bid.ParentHash.String(), bid.BlockHash.String(), bid.BuilderPubkey.String(), bid.ProposerPubkey.String(),
		bid.ProposerFeeRecipient.String(), bid.GasUsed, bid.GasLimit, bid.Value.String(), uint64(bid.Slot)/uint64(SlotsPerEpoch),
		bid.NumTx, bid.BlockNumber, isMostProfitable, time.UnixMilli(int64(bid.TimestampMs)))
	return err
}

func (s *Datastore) GetBuilderBlockSubmissions(ctx context.Context, headSlot uint64, payload structs.SubmissionTraceQuery) (bts []structs.BidTraceWithTimestamp, err error) {
	var i = 1
	parts := []string{"relay_id = $" + strconv.Itoa(i)}
	data := []interface{}{s.RelayID}
	i++

	if payload.Slot > 0 {
		parts = append(parts, "slot = $"+strconv.Itoa(i))
		data = append(data, payload.Slot)
		i++
	}

	if payload.BlockHash != Emptybytes32 {
		parts = append(parts, "block_hash = $"+strconv.Itoa(i))
		data = append(data, payload.BlockHash.String())
		i++
	}

	if payload.BlockNum > 0 {
		parts = append(parts, "block_number = $"+strconv.Itoa(i))
		data = append(data, payload.BlockNum)
		i++
	}

	qBuilder := strings.Builder{}
	qBuilder.WriteString(`SELECT block_time, slot, builder_pubkey, proposer_pubkey, proposer_fee_recipient, parent_hash, block_hash, value, gas_used, gas_limit, block_number, num_tx FROM builder_block_submission `)

	if len(parts) > 0 {
		qBuilder.WriteString(" WHERE ")
		for i, par := range parts {
			if i != 0 {
				qBuilder.WriteString(" AND ")
			}
			qBuilder.WriteString(par)
		}
	}

	qBuilder.WriteString(` ORDER BY slot DESC, block_time DESC, block_hash DESC LIMIT $` + strconv.Itoa(i))
	data = append(data, payload.Limit)
	rows, err := s.DB.QueryContext(ctx, qBuilder.String(), data...)
	switch {
	case err == sql.ErrNoRows:
		return []structs.BidTraceWithTimestamp{}, nil
	case err != nil:
		return nil, fmt.Errorf("query error: %w", err)
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
	for rows.Next() {
		bt := structs.BidTraceWithTimestamp{}
		t := time.Time{}
		err = rows.Scan(&t, &bt.Slot, &builderpubkey, &proposerPubkey, &proposerFeeRecipient, &parentHash, &blockHash, &value,
			&bt.GasUsed, &bt.GasLimit, &bt.BlockNumber, &bt.NumTx)
		if err != nil {
			return nil, err
		}
		bt.BuilderPubkey.UnmarshalText(builderpubkey)
		bt.ProposerPubkey.UnmarshalText(proposerPubkey)
		bt.ProposerFeeRecipient.UnmarshalText(proposerFeeRecipient)
		bt.ParentHash.UnmarshalText(parentHash)
		bt.BlockHash.UnmarshalText(blockHash)
		bt.Value.UnmarshalText(value)

		bt.Timestamp = uint64(t.Unix())
		bt.TimestampMs = uint64(t.UnixMilli())
		bts = append(bts, bt)
	}
	return bts, err
}

type GetBuilderSubmissionsFilters struct {
	Slot          uint64
	Limit         uint64
	BlockHash     string
	BlockNumber   uint64
	BuilderPubkey string
}
