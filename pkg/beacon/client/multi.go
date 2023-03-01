//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg/beacon/client BeaconNode
package client

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	uberatomic "go.uber.org/atomic"
)

var (
	ErrBeaconNodeSyncing      = errors.New("beacon node is syncing")
	ErrWithdrawalsUnsupported = errors.New("withdrawals are not supported")
)

type BeaconNode interface {
	SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent)
	GetProposerDuties(structs.Epoch) (*RegisteredProposersResponse, error)
	SyncStatus() (*SyncStatusPayloadData, error)
	KnownValidators(structs.Slot) (AllValidatorsResponse, error)
	Genesis() (structs.GenesisInfo, error)
	PublishBlock(block *types.SignedBeaconBlock) error
	Endpoint() string
	GetWithdrawals(structs.Slot) (*GetWithdrawalsResponse, error)
}

type MultiBeaconClient struct {
	Log     log.Logger
	Clients []BeaconNode

	bestBeaconIndex uberatomic.Int64
}

func NewMultiBeaconClient(l log.Logger, clients []BeaconNode) *MultiBeaconClient {
	if l == nil {
		l = log.New()
	}
	return &MultiBeaconClient{Log: l.WithField("service", "multi-beacon client"), Clients: clients}
}

func (b *MultiBeaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent) {
	for _, client := range b.Clients {
		go client.SubscribeToHeadEvents(ctx, slotC)
	}
}

func (b *MultiBeaconClient) GetProposerDuties(epoch structs.Epoch) (*RegisteredProposersResponse, error) {
	// return the first successful beacon node response
	clients := b.clientsByLastResponse()

	for i, client := range clients {
		log := b.Log.WithField("endpoint", client.Endpoint())

		duties, err := client.GetProposerDuties(epoch)
		if err != nil {
			log.WithError(err).Error("failed to get proposer duties")
			continue
		}

		b.bestBeaconIndex.Store(int64(i))

		// Received successful response. Set this index as last successful beacon node
		return duties, nil
	}

	return nil, ErrNodesUnavailable
}

func (b *MultiBeaconClient) SyncStatus() (*SyncStatusPayloadData, error) {
	var bestSyncStatus *SyncStatusPayloadData
	var foundSyncedNode bool

	// Check each beacon-node sync status
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, instance := range b.Clients {
		wg.Add(1)
		go func(client BeaconNode) {
			defer wg.Done()
			log := b.Log.WithField("endpoint", client.Endpoint())

			syncStatus, err := client.SyncStatus()
			if err != nil {
				log.WithError(err).Error("failed to get sync status")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			if foundSyncedNode {
				return
			}

			if bestSyncStatus == nil {
				bestSyncStatus = syncStatus
			}

			if !syncStatus.IsSyncing {
				bestSyncStatus = syncStatus
				foundSyncedNode = true
			}
		}(instance)
	}

	// Wait for all requests to complete...
	wg.Wait()

	if !foundSyncedNode {
		return nil, ErrBeaconNodeSyncing
	}

	if bestSyncStatus == nil {
		return nil, ErrNodesUnavailable
	}

	return bestSyncStatus, nil
}

func (b *MultiBeaconClient) KnownValidators(headSlot structs.Slot) (AllValidatorsResponse, error) {
	// return the first successful beacon node response
	clients := b.clientsByLastResponse()

	for i, client := range clients {
		log := b.Log.WithField("endpoint", client.Endpoint())

		validators, err := client.KnownValidators(headSlot)
		if err != nil {
			log.WithError(err).Error("failed to fetch validators")
			continue
		}

		b.bestBeaconIndex.Store(int64(i))

		// Received successful response. Set this index as last successful beacon node
		return validators, nil
	}

	return AllValidatorsResponse{}, ErrNodesUnavailable
}

func (b *MultiBeaconClient) Genesis() (genesisInfo structs.GenesisInfo, err error) {
	clients := b.clientsByLastResponse()
	for _, client := range clients {
		if genesisInfo, err = client.Genesis(); err != nil {
			b.Log.WithError(err).
				WithField("endpoint", client.Endpoint()).
				Warn("failed to get genesis info")
			continue
		}

		return genesisInfo, nil
	}

	return genesisInfo, err
}

func (b *MultiBeaconClient) GetWithdrawals(slot structs.Slot) (withdrawalsResp *GetWithdrawalsResponse, err error) {
	for _, client := range b.clientsByLastResponse() {
		if withdrawalsResp, err = client.GetWithdrawals(slot); err != nil {
			if strings.Contains(err.Error(), "Withdrawals not enabled before capella") {
				break
			}
			b.Log.WithError(err).
				WithField("endpoint", client.Endpoint()).
				Warn("failed to get withdrawals")
			continue
		}
		return withdrawalsResp, nil
	}

	if strings.Contains(err.Error(), "Withdrawals not enabled before capella") {
		return nil, ErrWithdrawalsUnsupported
	}

	return nil, err
}

func (b *MultiBeaconClient) Endpoint() string {
	if clients := b.clientsByLastResponse(); len(clients) > 0 {
		return clients[0].Endpoint()
	}
	return ""
}

// beaconInstancesByLastResponse returns a list of beacon clients that has the client
// with the last successful response as the first element of the slice
func (b *MultiBeaconClient) clientsByLastResponse() []BeaconNode {
	index := b.bestBeaconIndex.Load()
	if index == 0 {
		return b.Clients
	}

	instances := make([]BeaconNode, len(b.Clients))
	copy(instances, b.Clients)
	instances[0], instances[index] = instances[index], instances[0]

	return instances
}

func (b *MultiBeaconClient) PublishBlock(block *types.SignedBeaconBlock) (err error) {
	for _, client := range b.clientsByLastResponse() {
		if err = client.PublishBlock(block); err != nil {
			b.Log.WithError(err).
				WithField("endpoint", client.Endpoint()).
				Warn("failed to publish block to beacon")
			continue
		}

		return nil
	}

	return err
}
