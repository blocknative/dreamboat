//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/beacon/client BeaconClient
package client

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/blocknative/dreamboat/structs"
	"github.com/lthibault/log"
)

var (
	ErrBeaconNodeSyncing      = errors.New("beacon node is syncing")
	ErrWithdrawalsUnsupported = errors.New("withdrawals are not supported")
	ErrFailedToPublish        = errors.New("failed to publish")
)

type BeaconClient interface {
	GetProposerDuties(structs.Epoch) (*RegisteredProposersResponse, error)
	SyncStatus() (*SyncStatusPayloadData, error)
	KnownValidators(structs.Slot) (AllValidatorsResponse, error)
	Genesis() (structs.GenesisInfo, error)
	GetForkSchedule() (*GetForkScheduleResponse, error)
	PublishBlock(context.Context, structs.SignedBeaconBlock) error
	Randao(structs.Slot) (string, error)
	Endpoint() string
	GetWithdrawals(structs.Slot) (*GetWithdrawalsResponse, error)
}

type MultiBeaconClient struct {
	Log log.Logger

	slotC chan HeadEvent
	payC  chan PayloadAttributesEvent

	clients     map[int]BeaconClient
	clientsLock sync.RWMutex
}

func NewMultiBeaconClient(l log.Logger) *MultiBeaconClient {
	return &MultiBeaconClient{
		slotC: make(chan HeadEvent, 10),
		payC:  make(chan PayloadAttributesEvent, 10),
		Log:   l.WithField("service", "multi-beacon client"),
	}
	// TODO
	//client.AttachMetrics(m)
}

func (b *MultiBeaconClient) HeadEventsSubscription() chan HeadEvent {
	return b.slotC
}

func (b *MultiBeaconClient) PayloadAttributesSubscription() chan PayloadAttributesEvent {
	return b.payC
}

func (b *MultiBeaconClient) Add(bc BeaconClient) {
	b.clientsLock.Lock()
	defer b.clientsLock.Unlock()

	// it's map for a reason
	b.clients[len(b.clients)] = bc
}

func (b *MultiBeaconClient) Remove(toRemove url.URL) error {
	b.clientsLock.Lock()
	defer b.clientsLock.Unlock()

	for k, bc := range b.clients {
		if bc.Endpoint() == toRemove.String() {
			delete(b.clients, k)
			return nil
		}
	}
	return nil
}

func (b *MultiBeaconClient) GetProposerDuties(epoch structs.Epoch) (*RegisteredProposersResponse, error) {
	b.clientsLock.RLock()
	defer b.clientsLock.RUnlock()

	for _, client := range b.clients {
		duties, err := client.GetProposerDuties(epoch)
		if err != nil {
			b.Log.WithField("endpoint", client.Endpoint()).
				WithError(err).
				Error("failed to get proposer duties")
			continue
		}

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
	for _, instance := range b.clients {
		wg.Add(1)
		go func(client BeaconClient) {
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
	b.clientsLock.RLock()
	defer b.clientsLock.RUnlock()

	// return the first successful beacon node response
	for _, client := range b.clients {
		log := b.Log.WithField("endpoint", client.Endpoint())

		validators, err := client.KnownValidators(headSlot)
		if err != nil {
			log.WithError(err).Error("failed to fetch validators")
			continue
		}

		// Received successful response. Set this index as last successful beacon node
		return validators, nil
	}

	return AllValidatorsResponse{}, ErrNodesUnavailable
}

func (b *MultiBeaconClient) Genesis() (genesisInfo structs.GenesisInfo, err error) {
	b.clientsLock.RLock()
	defer b.clientsLock.RUnlock()

	for _, client := range b.clients {
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
	b.clientsLock.RLock()
	defer b.clientsLock.RUnlock()

	for _, client := range b.clients {
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

func (b *MultiBeaconClient) Randao(slot structs.Slot) (randao string, err error) {
	b.clientsLock.RLock()
	defer b.clientsLock.RUnlock()

	for _, client := range b.clients {
		if randao, err = client.Randao(slot); err != nil {
			b.Log.WithError(err).WithField("slot", slot).WithField("endpoint", client.Endpoint()).Warn("failed to get randao")
			continue
		}
		return
	}
	return
}

func (b *MultiBeaconClient) GetForkSchedule() (spec *GetForkScheduleResponse, err error) {
	b.clientsLock.RLock()
	defer b.clientsLock.RUnlock()

	for _, client := range b.clients { // random
		if spec, err = client.GetForkSchedule(); err != nil {
			b.Log.WithError(err).
				WithField("endpoint", client.Endpoint()).
				Warn("failed to get fork")
			continue
		}
		return spec, nil
	}
	return spec, err
}

/*
// beaconInstancesByLastResponse returns a list of beacon clients that has the client
// with the last successful response as the first element of the slice
func (b *MultiBeaconClient) clientsByLastResponse() []BeaconClient {
	index := b.bestBeaconIndex.Load()
	if index == 0 {
		return b.Clients
	}

	instances := make([]BeaconClient, len(b.Clients))
	copy(instances, b.Clients)
	instances[0], instances[index] = instances[index], instances[0]

	return instances
}*/

func (b *MultiBeaconClient) PublishBlock(ctx context.Context, block structs.SignedBeaconBlock) (err error) {
	resp := make(chan error, 20)
	var i int

	b.clientsLock.RLock()
	for _, client := range b.clients {
		i++
		c := client
		go publishAsync(ctx, c, b.Log, block, resp)
	}
	b.clientsLock.RUnlock()

	var (
		defError error
		r        int
	)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-resp:
			r++
			switch e {
			case nil:
				return nil
			default:
				defError = e
				if r == i {
					return defError
				}
			}
		}
	}
}

func publishAsync(ctx context.Context, client BeaconClient, l log.Logger, block structs.SignedBeaconBlock, resp chan<- error) {
	err := client.PublishBlock(ctx, block)
	if err != nil {
		l.WithError(err).
			WithField("endpoint", client.Endpoint()).
			Warn("failed to publish block to beacon")
	}
	resp <- err
}
