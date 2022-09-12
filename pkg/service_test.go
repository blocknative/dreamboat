package relay_test

import (
	"context"
	"sync"
	"testing"
	"time"

	mock_relay "github.com/blocknative/dreamboat/internal/mock/pkg"
	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/golang/mock/gomock"
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
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return nil, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		relayMock.EXPECT().
			GetHeader(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		service.GetHeader(nil, nil)
	})

	t.Run("RegisterValidator", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return nil, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		relayMock.EXPECT().
			RegisterValidator(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		service.RegisterValidator(ctx, nil)

	})

	t.Run("GetHeader", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return nil, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		relayMock.EXPECT().
			GetHeader(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		service.GetHeader(ctx, nil)
	})

	t.Run("GetPayload", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return nil, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		relayMock.EXPECT().
			GetPayload(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		service.GetPayload(ctx, nil)
	})

	t.Run("SubmitBlock", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return nil, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		relayMock.EXPECT().
			SubmitBlock(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		service.SubmitBlock(ctx, nil)
	})

	t.Run("GetValidators", func(t *testing.T) {
		t.Parallel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return nil, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		relayMock.EXPECT().
			GetValidators(gomock.Any()).
			Times(1)

		service.GetValidators()
	})
}

func TestBeaconClientState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	t.Run("connect", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		beaconMock := mock_relay.NewMockBeaconClient(ctrl)
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return beaconMock, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		beaconMock.EXPECT().SyncStatus().Return(&relay.SyncStatusPayloadData{}, nil).Times(1)
		beaconMock.EXPECT().UpdateProposerDuties(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
		events := make(chan relay.HeadEvent, 1)
		events <- relay.HeadEvent{Slot: 1}

		beaconMock.EXPECT().SubscribeToHeadEvents(gomock.Any()).Return(events).Times(1)
		beaconMock.EXPECT().ProcessNewSlot(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

		var wg sync.WaitGroup
		defer wg.Wait()

		wg.Add(1)
		go func() {
			defer wg.Done()

			err := service.Run(ctx)
			require.Error(t, err, context.Canceled)
		}()
		<-service.Ready()

		relayMock.EXPECT().
			GetHeader(nil, nil, beaconStateMatcher{beaconMock}).
			Times(1)

		service.GetHeader(nil, nil)
		time.Sleep(time.Second) // give time for the beacon state manager to kick-off

		cancel()
		close(events)
	})

	t.Run("reconnect", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		relayMock := mock_relay.NewMockRelay(ctrl)
		beaconMock := mock_relay.NewMockBeaconClient(ctrl)
		service := relay.DefaultService{
			Relay: relayMock,
			NewBeaconClient: func() (relay.BeaconClient, error) {
				return beaconMock, nil
			},
			Datastore: relay.DefaultDatastore{newMockDatastore()},
		}

		// for checking restarts, we set beacon functions to 'MinTimes(2)'
		beaconMock.EXPECT().SyncStatus().Return(&relay.SyncStatusPayloadData{}, nil).MinTimes(2)
		beaconMock.EXPECT().UpdateProposerDuties(gomock.Any(), gomock.Any(), gomock.Any()).MinTimes(2)
		events := make(chan relay.HeadEvent, 1)
		events <- relay.HeadEvent{Slot: 1}
		close(events)

		beaconMock.EXPECT().ProcessNewSlot(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
		beaconMock.EXPECT().SubscribeToHeadEvents(gomock.Any()).Return(events).MinTimes(2)

		var wg sync.WaitGroup
		defer wg.Wait()

		wg.Add(1)
		go func() {
			defer wg.Done()

			err := service.Run(ctx)
			require.Error(t, err, context.Canceled)
		}()
		<-service.Ready()

		time.Sleep(time.Second) // give time for the beacon state manager to kick-off

		cancel()
	})

}

type beaconStateMatcher struct {
	beacon relay.BeaconClient
}

func (m beaconStateMatcher) Matches(x interface{}) bool {
	state, ok := x.(relay.State)
	if !ok {
		return false
	}
	return state.Beacon() == m.beacon
}

func (m beaconStateMatcher) String() string {
	return "beacon client matcher"
}
