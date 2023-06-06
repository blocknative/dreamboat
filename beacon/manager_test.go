package beacon_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blocknative/dreamboat/beacon"
	"github.com/blocknative/dreamboat/beacon/client"
	"github.com/blocknative/dreamboat/beacon/mocks"
	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
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

	bm := beacon.NewManager(log.New(), beacon.Config{})

	beaconMock.EXPECT().GetProposerDuties(gomock.Any()).Return(&client.RegisteredProposersResponse{Data: []client.RegisteredProposersResponseData{}}, nil).Times(2)
	beaconMock.EXPECT().KnownValidators(gomock.Any()).Return(client.AllValidatorsResponse{Data: []client.ValidatorResponseEntry{}}, nil).Times(1)
	beaconMock.EXPECT().SubscribeToPayloadAttributesEvents(gomock.Any()).Times(1).DoAndReturn(
		func(events chan client.PayloadAttributesEvent) {
			go func() {
				events <- client.PayloadAttributesEvent{Data: client.PayloadAttributesEventData{ProposalSlot: 1, ParentBlockHash: types.Hash{}.String()}}
			}()
		},
	)

	stateMock.EXPECT().SetParentBlockHash(types.Hash{}).Times(1)
	stateMock.EXPECT().ParentBlockHash().Return(types.Hash{}).Times(2)
	stateMock.EXPECT().SetHeadSlotIfHigher(gomock.Any()).Return(structs.Slot(1), true).Times(1)
	stateMock.EXPECT().HeadSlot().Return(structs.Slot(1)).Times(1)
	stateMock.EXPECT().KnownValidators().Return(structs.ValidatorsState{}).Times(1)
	stateMock.EXPECT().SetDuties(gomock.Any()).Times(1)
	stateMock.EXPECT().Duties().Return(structs.DutiesState{}).Times(1)
	stateMock.EXPECT().SetRandao(gomock.Any()).Times(1)
	stateMock.EXPECT().Randao(gomock.Any(), gomock.Any()).Return(structs.RandaoState{}).Times(1)
	stateMock.EXPECT().SetWithdrawals(gomock.Any()).Times(1)
	stateMock.EXPECT().Withdrawals(gomock.Any(), gomock.Any()).Return(structs.WithdrawalsState{}).Times(1)
	stateMock.EXPECT().SetKnownValidators(gomock.Any()).Times(1)
	stateMock.EXPECT().KnownValidatorsUpdateTime().Return(time.Now().Add(-time.Hour)).Times(2)
	stateMock.EXPECT().Fork().Return(structs.ForkState{}).AnyTimes()

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
