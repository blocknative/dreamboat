func (s *DatabaseService) GetDeliveredPayloads(ctx context.Context, queryArgs structs.GetDeliveredPayloadsFilters) (bts []structs.BidTrace, err error) {
	var i = 1
	parts := []string{}
	data := []interface{}{}

	if queryArgs.Slot > 0 {
		parts = append(parts, "slot = $"+strconv.Itoa(i))
		data = append(data, queryArgs.Slot)
		i++
	} else if queryArgs.Cursor > 0 {
		parts = append(parts, "slot <= $"+strconv.Itoa(i))
		data = append(data, queryArgs.Cursor)
		i++
	}

	if queryArgs.BlockHash != "" {
		parts = append(parts, "block_hash = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BlockHash)
		i++
	}

	if queryArgs.BlockNumber > 0 {
		parts = append(parts, "block_number = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BlockNumber)
		i++
	}

	if queryArgs.BuilderPubkey != "" {
		parts = append(parts, "builder_pubkey = $"+strconv.Itoa(i))
		data = append(data, queryArgs.BuilderPubkey)
		i++
	}

	if queryArgs.ProposerPubkey != "" {
		parts = append(parts, "proposer_pubkey = $"+strconv.Itoa(i))
		data = append(data, queryArgs.ProposerPubkey)
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

	rows, err := s.Db.QueryContext(ctx, qBuilder.String(), data...)
	switch {
	case err == sql.ErrNoRows:
		return nil, structs.ErrNoRows
	case err != nil:
		return nil, fmt.Errorf("query error: %w", err)
	default:
	}

	defer rows.Close()

	for rows.Next() {
		bt := structs.BidTrace{}
		err = rows.Scan(&bt.Slot, &bt.BuilderPubkey, &bt.ProposerPubkey, &bt.ProposerFeeRecipient, &bt.ParentHash, &bt.BlockHash, &bt.BlockNumber, &bt.NumTx, &bt.Value, &bt.GasUsed, &bt.GasLimit)
		if err != nil {
			return nil, err
		}
		bts = append(bts, bt)
	}
	return bts, err
}