package dspostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
)

var SlotsPerEpoch = 32
var ErrNoRows = errors.New("no rows")

type Datastore struct {
	RelayID uint64
	DB      *sql.DB
}

func NewDatastore(db *sql.DB, relayID uint64) *Datastore {
	return &Datastore{
		DB:      db,
		RelayID: relayID}
}

// func (s *Datastore) PutBuilderBlockSubmission(ctx context.Context, bid structs.BidTraceWithTimestamp, isMostProfitable bool) (err error) {
func (s *Datastore) PutBuilderBlockSubmission(ctx context.Context, bid structs.HeaderData, isMostProfitable bool) (err error) {
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
		s.RelayID, bid.Slot, bid.HeaderAndTrace.Header.ParentHash, bid.HeaderAndTrace.Header.BlockHash, bid.HeaderAndTrace.Trace.BuilderPubkey,
		bid.HeaderAndTrace.Trace.ProposerPubkey, bid.HeaderAndTrace.Trace.ProposerFeeRecipient, bid.HeaderAndTrace.Header.GasUsed,
		bid.HeaderAndTrace.Header.GasLimit, bid.HeaderAndTrace.Trace.Value, uint64(bid.Slot)/uint64(SlotsPerEpoch), bid.HeaderAndTrace.Trace.NumTx,
		bid.HeaderAndTrace.Header.BlockNumber, isMostProfitable, time.Unix(int64(bid.HeaderAndTrace.Header.Timestamp), 0))
	return err
}

func (s *Datastore) GetBuilderBlockSubmissions(ctx context.Context, headSlot uint64, payload structs.SubmissionTraceQuery) (bts []structs.BidTraceWithTimestamp, err error) {

	var i = 2
	parts := []string{"relay_id = $" + strconv.Itoa(i)}
	data := []interface{}{s.RelayID}

	if payload.Slot > 0 {
		parts = append(parts, "slot = $"+strconv.Itoa(i))
		data = append(data, payload.Slot)
		i++
	}

	if payload.BlockHash.String() != "" {
		parts = append(parts, "block_hash = $"+strconv.Itoa(i))
		data = append(data, payload.BlockHash.String())
		i++
	}

	if payload.BlockNum > 0 {
		parts = append(parts, "block_number = $"+strconv.Itoa(i))
		data = append(data, payload.BlockNum)
		i++
	}
	/*
		if payload. != "" {
			parts = append(parts, "builder_pubkey = $"+strconv.Itoa(i))
			data = append(data, payload.BuilderPubkey)
			i++
		}
	*/
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
		return nil, ErrNoRows
	case err != nil:
		return nil, fmt.Errorf("query error: %w", err)
	default:
	}

	defer rows.Close()

	for rows.Next() {
		bt := structs.BidTraceWithTimestamp{}
		t := time.Time{}
		err = rows.Scan(&t, &bt.Slot, &bt.BuilderPubkey, &bt.ProposerPubkey,
			&bt.ProposerFeeRecipient, &bt.ParentHash, &bt.BlockHash, &bt.Value, &bt.GasUsed, &bt.GasLimit, &bt.BlockNumber, &bt.NumTx)
		if err != nil {
			return nil, err
		}
		bt.Timestamp = uint64(t.Unix())
		bts = append(bts, bt)
	}
	return bts, err
}

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
		s.RelayID, uint64(slot), uint64(slot)/uint64(SlotsPerEpoch), payload.Trace.BidTraceExtended.BuilderPubkey, payload.Trace.BidTraceExtended.ProposerPubkey,
		payload.Trace.BidTraceExtended.ProposerFeeRecipient, payload.Trace.BidTraceExtended.ParentHash, payload.Trace.BidTraceExtended.BlockHash,
		payload.Trace.BidTraceExtended.NumTx, payload.BlockNumber, payload.Trace.BidTraceExtended.GasUsed, payload.Trace.BidTraceExtended.GasLimit,
		payload.Trace.BidTraceExtended.Value)
	return err
}

func (s *Datastore) GetDeliveredPayloads(ctx context.Context, headSlot uint64, queryArgs structs.PayloadTraceQuery) (bts []structs.BidTraceExtended, err error) {
	//GetDeliveredPayloads(ctx context.Context, relayID int, queryArgs structs.GetDeliveredPayloadsFilters) (bts []structs.BidTraceExtended, err error) {
	var i = 2
	parts := []string{"relay_id = $" + strconv.Itoa(i)}
	data := []interface{}{s.RelayID}

	if queryArgs.Slot > 0 {
		parts = append(parts, "slot = $"+strconv.Itoa(i))
		data = append(data, queryArgs.Slot)
		i++
	} else if queryArgs.Cursor > 0 {
		parts = append(parts, "slot <= $"+strconv.Itoa(i))
		data = append(data, queryArgs.Cursor)
		i++
	}

	if queryArgs.BlockHash.String() != "" {
		parts = append(parts, "block_hash = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BlockHash)
		i++
	}

	if queryArgs.BlockNum > 0 {
		parts = append(parts, "block_number = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BlockNum)
		i++
	}

	if queryArgs.Pubkey.String() != "" {
		parts = append(parts, "builder_pubkey = $"+strconv.Itoa(i))
		data = append(data, queryArgs.Pubkey.String())
		i++
	}

	// TODO(l): BUG? Unsupported in relay?
	/*
		if queryArgs.ProposerPubkey != "" {
			parts = append(parts, "proposer_pubkey = $"+strconv.Itoa(i))
			data = append(data, queryArgs.ProposerPubkey)
			i++
		}
	*/
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

	// if filters.OrderByValue > 0 {
	// 	qBuilder.WriteString(` ORDER BY value ASC `)
	// } else if filters.OrderByValue < 0 {
	// 	qBuilder.WriteString(` ORDER BY value DESC `)
	// } else {
	qBuilder.WriteString(` ORDER BY slot DESC, inserted_at DESC `)

	if queryArgs.Limit > 0 {
		qBuilder.WriteString(` LIMIT $` + strconv.Itoa(i))
		data = append(data, queryArgs.Limit)
	}

	rows, err := s.DB.QueryContext(ctx, qBuilder.String(), data...)
	switch {
	case err == sql.ErrNoRows:
		return nil, ErrNoRows
	case err != nil:
		return nil, fmt.Errorf("query error: %w", err)
	default:
	}

	defer rows.Close()

	for rows.Next() {
		bt := structs.BidTraceExtended{}
		err = rows.Scan(&bt.Slot, &bt.BuilderPubkey, &bt.ProposerPubkey, &bt.ProposerFeeRecipient, &bt.ParentHash, &bt.BlockHash, &bt.BlockNumber, &bt.NumTx, &bt.Value, &bt.GasUsed, &bt.GasLimit)
		if err != nil {
			return nil, err
		}
		bts = append(bts, bt)
	}
	return bts, err
}

func (s *Datastore) CheckSlotDelivered(ctx context.Context, slot uint64) (bool, error) {
	var sl uint64
	err := s.DB.QueryRowContext(ctx, "SELECT slot FROM payload_delivered WHERE slot = ? LIMIT 1", slot).Scan(&sl)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return sl == slot, err
}

type GetBuilderSubmissionsFilters struct {
	Slot          uint64
	Limit         uint64
	BlockHash     string
	BlockNumber   uint64
	BuilderPubkey string
}

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
