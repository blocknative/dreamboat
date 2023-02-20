package badger_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/blocknative/dreamboat/pkg/structs"
	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/stretchr/testify/require"
)

func TestPutGetHeaderDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := badger.NewDatastore("/tmp/EvidenceBadger", &badger.DefaultOptions)
	require.NoError(t, err)

	hc := datastore.NewHeaderController(200, time.Hour)
	d, err := datastore.NewDatastore(store, store.DB, hc, 100)
	require.NoError(t, err)

	header := randomHeaderAndTrace()
	slotInt := rand.Int()
	slot := structs.Slot(slotInt)

	header.Trace.Slot = uint64(slotInt)

	// put
	jsHeader, _ := json.Marshal(header)
	err = d.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		HeaderAndTrace: header,
		Marshaled:      jsHeader,
	}, time.Minute)
	require.NoError(t, err)

	// get
	_, err = d.GetDelivered(ctx, structs.PayloadQuery{Slot: slot})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block hash
	_, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockHash: header.Trace.BlockHash})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// get by block number
	_, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockNum: header.Header.BlockNumber})
	require.ErrorIs(t, err, ds.ErrNotFound)

	_, err = d.GetDelivered(ctx, structs.PayloadQuery{PubKey: header.Trace.ProposerPubkey})
	require.ErrorIs(t, err, ds.ErrNotFound)

	// set as delivered and retrieve again
	err = d.PutDelivered(ctx, slot, structs.DeliveredTrace{Trace: *header.Trace, BlockNumber: header.Header.BlockNumber}, time.Minute)
	require.NoError(t, err)

	// get
	gotHeader, err := d.GetDelivered(ctx, structs.PayloadQuery{Slot: slot})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	// get by block hash
	gotHeader, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockHash: header.Trace.BlockHash})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	// get by block number
	gotHeader, err = d.GetDelivered(ctx, structs.PayloadQuery{BlockNum: header.Header.BlockNumber})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)

	gotHeader, err = d.GetDelivered(ctx, structs.PayloadQuery{PubKey: header.Trace.ProposerPubkey})
	require.NoError(t, err)
	require.EqualValues(t, header.Trace.Value, gotHeader.BidTrace.Value)
}
