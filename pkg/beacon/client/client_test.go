package client_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/pkg/beacon/client"
	"github.com/blocknative/dreamboat/pkg/beacon/client/mocks"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
)

var (
	nullLog = log.New(log.WithWriter(io.Discard))
)

func TestMultiSubscribeToHeadEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)

	connected := mocks.NewMockBeaconNode(ctrl)
	disconnected := mocks.NewMockBeaconNode(ctrl)

	bc := &client.MultiBeaconClient{Log: nullLog, Clients: []client.BeaconNode{connected, disconnected}}

	events := make(chan client.HeadEvent)

	connected.EXPECT().SubscribeToHeadEvents(ctx, events).Times(1)
	disconnected.EXPECT().SubscribeToHeadEvents(ctx, events).Times(1)

	bc.SubscribeToHeadEvents(ctx, events)

	time.Sleep(time.Second) // wait for goroutines to spawn
}

func TestMultiGetProposerDuties(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	t.Run("connected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mocks.NewMockBeaconNode(ctrl)
		disconnected := mocks.NewMockBeaconNode(ctrl)

		clients := []client.BeaconNode{connected, disconnected}

		bc := &client.MultiBeaconClient{Log: nullLog, Clients: clients}

		duties := client.RegisteredProposersResponse{}

		epoch := structs.Epoch(rand.Uint64())

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			GetProposerDuties(epoch).
			Return(&duties, nil).
			Times(1)

		gotDuties, err := bc.GetProposerDuties(epoch)
		require.NoError(t, err)

		require.Equal(t, &duties, gotDuties)
	})

	t.Run("disconnected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mocks.NewMockBeaconNode(ctrl)
		disconnected := mocks.NewMockBeaconNode(ctrl)

		clients := []client.BeaconNode{disconnected, connected}

		bc := &client.MultiBeaconClient{Log: nullLog, Clients: clients}

		duties := client.RegisteredProposersResponse{}

		epoch := structs.Epoch(rand.Uint64())

		disconnected.EXPECT().Endpoint().Times(1)
		disconnected.EXPECT().
			GetProposerDuties(epoch).
			Return(nil, client.ErrHTTPErrorResponse).
			Times(1)

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			GetProposerDuties(epoch).
			Return(&duties, nil).
			Times(1)

		gotDuties, err := bc.GetProposerDuties(epoch)
		require.NoError(t, err)

		require.Equal(t, &duties, gotDuties)
	})
}

func TestMultiSyncStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	t.Run("all beacons connected", func(t *testing.T) {
		t.Parallel()

		connected := mocks.NewMockBeaconNode(ctrl)
		connected2 := mocks.NewMockBeaconNode(ctrl)

		clients := []client.BeaconNode{connected, connected2}

		bc := &client.MultiBeaconClient{Log: nullLog, Clients: clients}

		status := client.SyncStatusPayloadData{}

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			SyncStatus().
			Return(&status, nil).
			Times(1)

		connected2.EXPECT().Endpoint().Times(1)
		connected2.EXPECT().
			SyncStatus().
			Return(&status, nil).
			Times(1)

		gotStatus, err := bc.SyncStatus()
		require.NoError(t, err)

		require.Equal(t, &status, gotStatus)
	})

	t.Run("with disconnected beacon", func(t *testing.T) {
		t.Parallel()

		connected := mocks.NewMockBeaconNode(ctrl)
		disconnected := mocks.NewMockBeaconNode(ctrl)

		clients := []client.BeaconNode{disconnected, connected}

		bc := &client.MultiBeaconClient{Log: nullLog, Clients: clients}

		status := client.SyncStatusPayloadData{}

		disconnected.EXPECT().Endpoint().Times(1)
		disconnected.EXPECT().
			SyncStatus().
			Return(nil, client.ErrHTTPErrorResponse).
			Times(1)

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			SyncStatus().
			Return(&status, nil).
			Times(1)

		gotStatus, err := bc.SyncStatus()
		require.NoError(t, err)

		require.Equal(t, &status, gotStatus)
	})
}

func TestMultiKnownValidators(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	t.Run("connected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mocks.NewMockBeaconNode(ctrl)
		disconnected := mocks.NewMockBeaconNode(ctrl)

		clients := []client.BeaconNode{connected, disconnected}

		bc := &client.MultiBeaconClient{Log: nullLog, Clients: clients}

		validators := client.AllValidatorsResponse{}

		slot := structs.Slot(rand.Uint64())

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			KnownValidators(slot).
			Return(validators, nil).
			Times(1)

		gotValidators, err := bc.KnownValidators(slot)
		require.NoError(t, err)

		require.Equal(t, validators, gotValidators)
	})

	t.Run("disconnected beacon first", func(t *testing.T) {
		t.Parallel()

		connected := mocks.NewMockBeaconNode(ctrl)
		disconnected := mocks.NewMockBeaconNode(ctrl)

		clients := []client.BeaconNode{disconnected, connected}

		bc := &client.MultiBeaconClient{Log: nullLog, Clients: clients}

		validators := client.AllValidatorsResponse{Data: []client.ValidatorResponseEntry{}}
		disconnectedValidators := client.AllValidatorsResponse{Data: nil}

		slot := structs.Slot(rand.Uint64())

		disconnected.EXPECT().Endpoint().Times(1)
		disconnected.EXPECT().
			KnownValidators(slot).
			Return(disconnectedValidators, client.ErrHTTPErrorResponse).
			Times(1)

		connected.EXPECT().Endpoint().Times(1)
		connected.EXPECT().
			KnownValidators(slot).
			Return(validators, nil).
			Times(1)

		gotValidators, err := bc.KnownValidators(slot)
		require.NoError(t, err)

		require.EqualValues(t, validators, gotValidators)
	})
}
