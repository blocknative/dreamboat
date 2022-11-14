package relay_test

import (
	"context"
	"sync"
	"testing"
	"time"

	relay "github.com/blocknative/dreamboat/pkg"
	mock_relay "github.com/blocknative/dreamboat/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

func TestServiceRouting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("Status", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		databaseMock := mock_relay.NewMockDatastore(ctrl)
		as := &relay.AtomicState{}
		service := relay.NewService(log.New(), relay.Config{}, databaseMock, relayMock, as)
		service.NewBeaconClient = func() (relay.BeaconClient, error) {
			return nil, nil
		}

		relayMock.EXPECT().
			GetHeader(gomock.Any(), gomock.Any()).
			Times(1)

		service.GetHeader(context.Background(), nil)
	})

	t.Run("RegisterValidator", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		databaseMock := mock_relay.NewMockDatastore(ctrl)

		as := &relay.AtomicState{}
		service := relay.NewService(log.New(), relay.Config{}, databaseMock, relayMock, as)
		service.NewBeaconClient = func() (relay.BeaconClient, error) {
			return nil, nil
		}

		relayMock.EXPECT().
			RegisterValidator(gomock.Any(), gomock.Any()).
			Times(1)

		service.RegisterValidator(ctx, nil)

	})

	t.Run("GetHeader", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		databaseMock := mock_relay.NewMockDatastore(ctrl)
		as := &relay.AtomicState{}
		service := relay.NewService(log.New(), relay.Config{}, databaseMock, relayMock, as)
		service.NewBeaconClient = func() (relay.BeaconClient, error) {
			return nil, nil
		}
		service.Relay = relayMock

		relayMock.EXPECT().
			GetHeader(gomock.Any(), gomock.Any()).
			Times(1)

		service.GetHeader(ctx, nil)
	})

	t.Run("GetPayload", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		databaseMock := mock_relay.NewMockDatastore(ctrl)

		as := &relay.AtomicState{}
		service := relay.NewService(log.New(), relay.Config{}, databaseMock, relayMock, as)
		service.NewBeaconClient = func() (relay.BeaconClient, error) {
			return nil, nil
		}
		service.Relay = relayMock

		relayMock.EXPECT().
			GetPayload(gomock.Any(), gomock.Any()).
			Times(1)

		service.GetPayload(ctx, nil)
	})

	t.Run("SubmitBlock", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		databaseMock := mock_relay.NewMockDatastore(ctrl)

		as := &relay.AtomicState{}
		service := relay.NewService(log.New(), relay.Config{}, databaseMock, relayMock, as)
		service.NewBeaconClient = func() (relay.BeaconClient, error) {
			return nil, nil
		}
		service.Relay = relayMock

		relayMock.EXPECT().
			SubmitBlock(gomock.Any(), gomock.Any()).
			Times(1)

		service.SubmitBlock(ctx, nil)
	})

	t.Run("GetValidators", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		databaseMock := mock_relay.NewMockDatastore(ctrl)

		as := &relay.AtomicState{}
		service := relay.NewService(log.New(), relay.Config{}, databaseMock, relayMock, as)
		service.NewBeaconClient = func() (relay.BeaconClient, error) {
			return nil, nil
		}
		service.Relay = relayMock
		relayMock.EXPECT().
			GetValidators().
			Times(1)

		service.GetValidators()
	})
}

func TestBeaconClientState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relayMock := mock_relay.NewMockRelay(ctrl)
	databaseMock := mock_relay.NewMockDatastore(ctrl)
	beaconMock := mock_relay.NewMockBeaconClient(ctrl)

	as := &relay.AtomicState{}
	service := relay.NewService(log.New(), relay.Config{}, databaseMock, relayMock, as)
	service.NewBeaconClient = func() (relay.BeaconClient, error) {
		return beaconMock, nil
	}
	service.Relay = relayMock

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
	beaconMock.EXPECT().Genesis().Times(1).Return(relay.GenesisInfo{}, nil)

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := service.RunBeacon(ctx)
		require.Error(t, err, context.Canceled)
	}()
	<-service.Ready()

	relayMock.EXPECT().
		GetHeader(gomock.Any(), gomock.Any()).
		Times(1)

	service.GetHeader(context.Background(), nil)
	time.Sleep(time.Second) // give time for the beacon state manager to kick-off

	cancel()
}
