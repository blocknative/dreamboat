package relay_test

import (
	"context"
	"sync"
	"testing"
	"time"

	relay "github.com/blocknative/dreamboat/pkg"
	mock_relay "github.com/blocknative/dreamboat/pkg/mocks"
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

	databaseMock := mock_relay.NewMockDatastore(ctrl)
	beaconMock := mock_relay.NewMockBeaconClient(ctrl)

	as := &relay.AtomicState{}
	service := relay.NewService(log.New(), relay.Config{}, databaseMock, as)
	service.NewBeaconClient = func() (relay.BeaconClient, error) {
		return beaconMock, nil
	}

	beaconMock.EXPECT().GetProposerDuties(gomock.Any()).Return(&relay.RegisteredProposersResponse{[]relay.RegisteredProposersResponseData{}}, nil).Times(4)
	beaconMock.EXPECT().SyncStatus().Return(&relay.SyncStatusPayloadData{}, nil).Times(1)

	beaconMock.EXPECT().SubscribeToHeadEvents(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(ctx context.Context, events chan relay.HeadEvent) {
			go func() {
				events <- relay.HeadEvent{Slot: 1}
			}()
		},
	)
	beaconMock.EXPECT().KnownValidators(gomock.Any()).Return(relay.AllValidatorsResponse{Data: []relay.ValidatorResponseEntry{}}, nil).Times(1)
	beaconMock.EXPECT().Genesis().Times(1).Return(structs.GenesisInfo{}, nil)
	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := service.RunBeacon(ctx)
		require.Error(t, err, context.Canceled)
	}()
	<-service.Ready()

	time.Sleep(time.Second) // give time for the beacon state manager to kick-off

	cancel()
}
