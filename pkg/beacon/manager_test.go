package beacon_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/pkg/beacon"
	"github.com/blocknative/dreamboat/pkg/beacon/client"
	"github.com/blocknative/dreamboat/pkg/beacon/mocks"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestBeaconClientState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	databaseMock := mocks.NewMockDatastore(ctrl)
	beaconMock := mocks.NewMockBeaconClient(ctrl)
	stateMock := mocks.NewMockState(ctrl)

	bm := beacon.NewManager(log.New())

	beaconMock.EXPECT().GetProposerDuties(gomock.Any()).Return(&client.RegisteredProposersResponse{Data: []client.RegisteredProposersResponseData{}}, nil).Times(4)
	beaconMock.EXPECT().SyncStatus().Return(&client.SyncStatusPayloadData{}, nil).Times(1)

	beaconMock.EXPECT().SubscribeToHeadEvents(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(ctx context.Context, events chan client.HeadEvent) {
			go func() {
				events <- client.HeadEvent{Slot: 1}
			}()
		},
	)
	beaconMock.EXPECT().KnownValidators(gomock.Any()).Return(client.AllValidatorsResponse{Data: []client.ValidatorResponseEntry{}}, nil).Times(1)
	beaconMock.EXPECT().Genesis().Times(1).Return(structs.GenesisInfo{}, nil)
	vCache := mocks.NewMockValidatorCache(ctrl)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := bm.Run(ctx, stateMock, beaconMock, databaseMock, vCache)
		require.Error(t, err, context.Canceled)
	}()

	time.Sleep(time.Second) // give time for the beacon state manager to kick-off

	cancel()
}
