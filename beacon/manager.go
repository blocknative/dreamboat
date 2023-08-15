//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/beacon Datastore,ValidatorCache,State
package beacon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/beacon/client"
	"github.com/blocknative/dreamboat/datastore"
	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"github.com/r3labs/sse"
)

var (
	ErrUnkownFork = errors.New("beacon node fork is unknown")

	ErrBeaconNodeSyncing      = errors.New("beacon node is syncing")
	ErrWithdrawalsUnsupported = errors.New("withdrawals are not supported")
)

type Datastore interface {
	GetRegistration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
}

type ValidatorCache interface {
	Get(types.PublicKey) (structs.ValidatorCacheEntry, bool)
}

type State interface {
	/*
		Duties() structs.DutiesState
		SetDuties(structs.DutiesState)

		KnownValidators() structs.ValidatorsState
		KnownValidatorsUpdateTime() time.Time
		SetKnownValidators(structs.ValidatorsState)

		HeadSlot() uint64
		SetHeadSlotIfHigher(structs.Slot) (structs.Slot, bool)

		Withdrawals(uint64, types.Hash) structs.WithdrawalsState
		SetWithdrawals(structs.WithdrawalsState)

		SetRandao(structs.RandaoState)
		Randao(uint64, types.Hash) structs.RandaoState


		ParentBlockHash() types.Hash
		SetParentBlockHash(types.Hash)
	*/

	SetGenesis(structs.GenesisInfo)
	Fork() structs.ForkState
	SetFork(structs.ForkState)

	SetPayloadAttributesState(slot uint64)
}

type Config struct {
	AltairForkVersion    string
	BellatrixForkVersion string
	CapellaForkVersion   string

	RunPayloadAttributesSubscription bool
}

type BeaconAddresses struct {
}

type Manager struct {
	ba BeaconAddresses

	Log    log.Logger
	Config Config
}

func NewManager(l log.Logger, ba BeaconAddresses, cfg Config) *Manager {
	return &Manager{
		Log: l.With(log.F{
			"subService":                       "beacon-manager",
			"runPayloadAttributesSubscription": cfg.RunPayloadAttributesSubscription,
			"numberOfSlotsInState":             structs.NumberOfSlotsInState,
		}),
		Config: cfg,
	}
}

func (s *Manager) Sync(ctx context.Context, state State) error {
	logger := s.Log.WithField("method", "Init")

	headSlot, err := waitForSynced(ctx, address)
	if err != nil {
		return err
	}

	genesis, err := client.Genesis(ctx, address)
	if err != nil {
		return fmt.Errorf("fail to get genesis from beacon: %w", err)
	}
	state.SetGenesis(genesis)
	logger.
		WithField("genesis-time", time.Unix(int64(genesis.GenesisTime), 0)).
		Info("genesis retrieved")

	fork, err := s.initForkEpoch(ctx)
	if err != nil {
		return fmt.Errorf("failed to set fork state: %w", err)
	}
	state.SetFork(fork)

	//fork := state.Fork()
	if !fork.IsAltair(headSlot, headSlot/structs.NumberOfSlotsInState) &&
		!fork.IsBellatrix(headSlot, headSlot/structs.NumberOfSlotsInState) &&
		!fork.IsCapella(headSlot, headSlot/structs.NumberOfSlotsInState) {
		return ErrUnkownFork
	}
	return nil
}

func (m *Manager) initForkEpoch(ctx context.Context) (forkState structs.ForkState, err error) {
	forkSchedule, err := client.GetForkSchedule(ctx)
	if err != nil {
		return forkState, fmt.Errorf("failed to get fork: %w", err)
	}

	forkState = structs.ForkState{}
	for _, fork := range forkSchedule.Data {
		switch fork.CurrentVersion {
		case m.Config.AltairForkVersion:
			forkState.AltairEpoch = fork.Epoch
		case m.Config.BellatrixForkVersion:
			forkState.BellatrixEpoch = fork.Epoch
		case m.Config.CapellaForkVersion:
			forkState.CapellaEpoch = fork.Epoch
		}
	}

	return forkState, nil
}

func waitForSynced(ctx context.Context, addresses []string) (uint64, error) {
	for {
		status, err := client.SyncStatus(ctx, "")
		if err != nil {
			if !errors.Is(err, ErrBeaconNodeSyncing) {
				return 0, err
			}
		} else if !status.IsSyncing {
			return status.HeadSlot, nil
		}

		//logger.Debug("beacon clients are syncing...")
		select {
		case <-ctx.Done():
			return status.HeadSlot, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *Manager) Run(ctx context.Context, state State, d Datastore, vCache ValidatorCache) error {
	logger := s.Log.WithField("method", "RunBeacon")

	defer logger.Debug("beacon loop stopped")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-c:
			t := time.Now()

			if ev.Data.ProposalSlot == 0 {
				logger.
					WithField("slot", ev.Data.ProposalSlot).
					Warn("error processing slot - zero slot")
				continue
			}

			err := s.processNewSlot(ctx, state, client, ev, d, vCache)

			//  headSlot := state.HeadSlot()
			//	validators := state.KnownValidators()
			//	duties := state.Duties()
			//	parentHash := state.ParentBlockHash()

			logger = logger.With(log.F{
				"epoch":                     structs.ToEpoch(headSlot),
				"slotHead":                  headSlot,
				"slotStartNextEpoch":        structs.Slot(structs.ToEpoch(headSlot)+1) * structs.SlotsPerEpoch,
				"fork":                      state.Fork().Version(headSlot, structs.ToEpoch(headSlot)),
				"numDuties":                 len(duties.ProposerDutiesResponse),
				"numKnownValidators":        len(validators.KnownValidators),
				"knownValidatorsUpdateTime": state.KnownValidatorsUpdateTime(),
				"randao":                    state.Randao(uint64(headSlot), parentHash).Randao,
				"withdrawalsRoot":           state.Withdrawals(uint64(headSlot), parentHash).Root.String(),
				"processingTimeMs":          time.Since(t).Milliseconds(),
			})
			if err != nil {
				logger.
					WithField("slot", ev.Data.ProposalSlot-1).
					WithError(err).
					Error("error processing slot")
				continue
			}

			logger.Debug("processed new slot")
		}
	}
}

func PayloadAttributesMultisubscribe(slotC chan PayloadAttributesEvent) {
	for _, instance := range b.Clients {
		go instance.SubscribeToPayloadAttributesEvents(slotC)
	}
}

func (s *Manager) processNewSlot(ctx context.Context, state State, event PayloadAttributesEvent, d Datastore, vCache ValidatorCache) error {
	logger := s.Log.WithField("method", "ProcessNewSlot")

	var (
		receivedParentBlockHash    types.Hash
		currParentBlockHash        = state.ParentBlockHash()
		receivedSlot               = event.Data.ProposalSlot - 1
		receivedEpoch              = structs.ToEpoch(receivedSlot)
		currHeadSlot, isNewHighest = state.SetHeadSlotIfHigher(receivedSlot)
	)

	if err := receivedParentBlockHash.UnmarshalText([]byte(event.Data.ParentBlockHash)); err != nil {
		return fmt.Errorf("failed to unmarshal parentBlockHash: %w", err)
	}

	if !isNewHighest && receivedSlot < currHeadSlot {
		logger.WithField("slotHead", currHeadSlot).Debug("received old payload attributes")
		return nil
	} else if !isNewHighest && receivedSlot == currHeadSlot && receivedParentBlockHash == currParentBlockHash {
		logger.WithField("slotHead", currHeadSlot).Debug("received duplicate payload attributes")
		return nil
	} else if !isNewHighest && receivedSlot == currHeadSlot && receivedParentBlockHash != currParentBlockHash && !(state.Fork().IsAltair(receivedSlot, receivedEpoch) || state.Fork().IsBellatrix(receivedSlot, receivedEpoch)) {
		logger.
			WithField("slotHead", currHeadSlot).
			WithField("currParentBlockHash", currParentBlockHash).
			WithField("receivedParentBlockHash", receivedParentBlockHash).
			Debug("received payload attributes with different parentBlockhash")
		return s.updateWithdrawalsAndRandao(ctx, logger, state, event, receivedParentBlockHash)
	}

	if currHeadSlot > 0 {
		for slot := currHeadSlot + 1; slot < receivedSlot; slot++ {
			logger.With(log.F{"slot": slot, "event": "missed_slot"}).Warn("missed slot")
		}
	}

	currHeadSlot = receivedSlot
	logger = logger.WithField("slotHead", currHeadSlot)

	if fork := state.Fork().Version(receivedSlot, receivedEpoch); fork == structs.ForkAltair || fork == structs.ForkBellatrix {
		return fmt.Errorf("unknown fork: %d", fork)
	}

	// update proposer duties and known validators in the background
	if (structs.DurationPerEpoch / 2) < time.Since(state.KnownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func(slot structs.Slot) {
			if err := s.updateKnownValidators(ctx, state, client, slot); err != nil {
				logger.WithError(err).Error("failed to update known validators")
				return
			}
		}(receivedSlot)
	}

	state.SetParentBlockHash(receivedParentBlockHash)

	// update proposer duties
	entries, err := s.getProposerDuties(ctx, client, structs.Slot(currHeadSlot))
	if err != nil {
		return err
	}
	s.storeProposerDuties(ctx, state, d, vCache, structs.Slot(currHeadSlot), entries)

	if state.Fork().IsAltair(receivedSlot, receivedEpoch) ||
		state.Fork().IsBellatrix(receivedSlot, receivedEpoch) {
		return nil
	}

	return s.updateWithdrawalsAndRandao(ctx, logger, state, event, receivedParentBlockHash)
}

func (m *Manager) updateWithdrawalsAndRandao(ctx context.Context, logger log.Logger, state State, event PayloadAttributesEvent, parentHash types.Hash) error {
	slot := event.Data.ProposalSlot - 1

	hW := structs.HashWithdrawals{Withdrawals: event.Data.PayloadAttributes.Withdrawals}
	root, err := hW.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("failed to compute withdrawals root: %w", err)
	}
	randao := event.Data.PayloadAttributes.PrevRandao

	state.SetWithdrawals(structs.WithdrawalsState{Slot: structs.Slot(slot), Root: root, ParentHash: parentHash})
	state.SetRandao(structs.RandaoState{Slot: uint64(slot), Randao: randao, ParentHash: parentHash})

	return nil
}

func (s *Manager) getProposerDuties(ctx context.Context, epoch uint64) (entries []bcli.RegisteredProposersResponseData, err error) {

	// Query current epoch
	current, err := client.GetProposerDuties(ctx, epoch)
	if err != nil {
		return nil, fmt.Errorf("current epoch: get proposer duties: %w", err)
	}

	entries = current.Data

	// Query next epoch
	next, err := client.GetProposerDuties(ctx, epoch+1)
	if err != nil {
		return nil, fmt.Errorf("next epoch: get proposer duties: %w", err)
	}

	return append(entries, next.Data...), nil
}

func (s *Manager) storeProposerDuties(ctx context.Context, state State, d Datastore, vCache ValidatorCache, headSlot structs.Slot, entries []bcli.RegisteredProposersResponseData) {
	epoch := headSlot.Epoch()
	logger := s.Log.With(log.F{
		"method":    "UpdateProposerDuties",
		"slot":      headSlot,
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

	newState := structs.DutiesState{
		CurrentSlot:            headSlot,
		ProposerDutiesResponse: make(structs.BuilderGetValidatorsResponseEntrySlice, 0, len(entries)),
	}

	var err error
	for _, e := range entries {
		reg, ok := vCache.Get(e.PubKey.PublicKey)
		if !ok {
			reg.Entry, err = d.GetRegistration(ctx, e.PubKey.PublicKey)
			if err != nil {
				if !errors.Is(err, datastore.ErrNotFound) {
					logger.Warn(fmt.Errorf("fail retrieve validator %s: %w", e.PubKey.PublicKey.String(), err))
				}
				continue
			}
		}
		newState.ProposerDutiesResponse = append(newState.ProposerDutiesResponse, types.BuilderGetValidatorsResponseEntry{
			Slot:  e.Slot,
			Entry: &reg.Entry,
		})
	}
	state.SetDuties(newState)
}

func (s *Manager) updateKnownValidators(ctx context.Context, state State, current uint64) error {
	newState := structs.ValidatorsState{}
	validators, err := client.KnownValidators(ctx, "", current)
	if err != nil {
		return err
	}

	knownValidators := make(map[types.PubkeyHex]struct{})
	knownValidatorsByIndex := make(map[uint64]types.PubkeyHex)
	for _, vs := range validators.Data {
		knownValidators[types.NewPubkeyHex(vs.Validator.Pubkey)] = struct{}{}
		knownValidatorsByIndex[vs.Index] = types.NewPubkeyHex(vs.Validator.Pubkey)
	}

	newState.KnownValidators = knownValidators
	newState.KnownValidatorsByIndex = knownValidatorsByIndex

	state.SetKnownValidators(newState)

	return nil
}

type PayloadAttributesEvent struct {
	Version string                     `json:"version"`
	Data    PayloadAttributesEventData `json:"data"`
}

type PayloadAttributesEventData struct {
	ProposerIndex     uint64            `json:"proposer_index,string"`
	ProposalSlot      uint64            `json:"proposal_slot,string"`
	ParentBlockNumber uint64            `json:"parent_block_number,string"`
	ParentBlockRoot   string            `json:"parent_block_root"`
	ParentBlockHash   string            `json:"parent_block_hash"`
	PayloadAttributes PayloadAttributes `json:"payload_attributes"`
}

type PayloadAttributes struct {
	Timestamp             uint64                `json:"timestamp,string"`
	PrevRandao            string                `json:"prev_randao"`
	SuggestedFeeRecipient string                `json:"suggested_fee_recipient"`
	Withdrawals           []*structs.Withdrawal `json:"withdrawals"`
}

/*
	ctx, cancel := context.WithCancel(ctx) // for stopping subscription after init is done
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-c:
			headSlot := structs.Slot(event.Data.ProposalSlot - 1)
			state.SetHeadSlotIfHigher(headSlot)

			// update proposer duties and known validators
			if err := s.updateKnownValidators(ctx, state, client, headSlot); err != nil {
				logger.WithError(err).Error("failed to update known validators")
				continue
			}

			entries, err := s.getProposerDuties(ctx, client, headSlot)
			if err != nil {
				return err
			}
			s.storeProposerDuties(ctx, state, d, vCache, headSlot, entries)

			var receivedParentBlockHash types.Hash
			if err := receivedParentBlockHash.UnmarshalText([]byte(event.Data.ParentBlockHash)); err != nil {
				return fmt.Errorf("failed to unmarshal parentBlockHash: %w", err)
			}

			if err := s.updateWithdrawalsAndRandao(ctx, logger, state, event, receivedParentBlockHash); err != nil {
				return err
			}
			return nil
		}
	}
*/

/*
func (b *MultiBeaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent) {
	for _, client := range b.Clients {
		go client.SubscribeToHeadEvents(ctx, slotC)
	}
}

func (b *MultiBeaconClient) GetProposerDuties(epoch structs.Epoch) (*RegisteredProposersResponse, error) {
	// return the first successful beacon node response
	//clients := b.clientsByLastResponse()

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
	// clients := b.clientsByLastResponse()

	for i, client := range clients {
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

func (b *MultiBeaconClient) Randao(slot structs.Slot) (randao string, err error) {
	for _, client := range b.clientsByLastResponse() {
		if randao, err = client.Randao(slot); err != nil {
			b.Log.WithError(err).WithField("slot", slot).WithField("endpoint", client.Endpoint()).Warn("failed to get randao")
			continue
		}

		return
	}

	return
}

func (b *MultiBeaconClient) GetForkSchedule() (spec *GetForkScheduleResponse, err error) {
	for _, client := range b.clientsByLastResponse() {
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



func (b *beaconClient) runNewHeadSubscriptionLoop(ctx context.Context, logger log.Logger, timer *time.Timer, slotC chan<- HeadEvent) {
	logger.Debug("subscription loop started")
	defer logger.Debug("subscription loop exited")

	for {
		client := sse.NewClient(fmt.Sprintf("%s/eth/v1/events?topics=head", b.beaconEndpoint.String()))
		err := client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
			var head HeadEvent
			if err := json.Unmarshal(msg.Data, &head); err != nil {
				logger.WithError(err).Warn("event subscription failed")
				return
			}

			timer.Reset(b.c.BeaconEventTimeout)

			select {
			case slotC <- head:
			case <-time.After(structs.DurationPerSlot / 2): // relief pressure
				logger.WithField("timeout", structs.DurationPerSlot/2).Warn("timeout waiting to consume head event")
				return
			case <-ctx.Done():
				logger.WithError(ctx.Err()).Warn("context cancelled waiting to consume head event after manual querying")
				return
			}
		})

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}

		logger.WithError(err).Warn("beacon subscription failed, restarting...")
	}
}

func (b *beaconClient) manuallyFetchLatestHeader(ctx context.Context, logger log.Logger, slotC chan HeadEvent) {
	// fetch head slot manully
	header, err := Headers(ctx, "")
	if err != nil {
		logger.WithError(err).Warn("failed querying latest header manually")
		return
	}
	if len(header.Data) != 1 {
		logger.Warnf("failed to query latest header manually, unexpected amount of header: expected 1 - got %d", len(header.Data))
		return
	}

	// send slot to beacon manager to consume async
	event := HeadEvent{Slot: header.Data[0].Header.Message.Slot}
	logger.With(event).Debug("manually fetched latest beacon header")

	select {
	case slotC <- event:
		return
	case <-time.After(structs.DurationPerSlot / 2):
		logger.WithField("timeout", structs.DurationPerSlot/2).Warn("timeout waiting to consume head event after manual querying")
		return
	case <-ctx.Done():
		logger.WithError(ctx.Err()).Warn("context cancelled waiting to consume head event after manual querying")
		return
	}
}
func (b *beaconClient) SubscribeToHeadEvents(ctx context.Context, slotC chan HeadEvent) {
	logger := b.log.WithField("method", "SubscribeToHeadEvents")
	defer logger.Debug("head events subscription stopped")

	for {
		loopCtx, cancelLoop := context.WithCancel(ctx)
		timer := time.NewTimer(b.c.BeaconEventTimeout)
		go b.runNewHeadSubscriptionLoop(loopCtx, logger, timer, slotC)

		lastTimeout := time.Time{} // zeroed value time.Time
		timeoutCounter := 0
	EventSelect:
		for {
			select {
			case <-timer.C:
				logger.Debug("timed out head events subscription, manually querying latest header")

				go b.manuallyFetchLatestHeader(ctx, logger, slotC)

				// to prevent disconnection due to multiple timeouts occurring very close in time, the subscription loop is canceled and restarted
				if time.Since(lastTimeout) <= b.c.BeaconEventTimeout*2 {
					timeoutCounter++
					if timeoutCounter >= b.c.BeaconEventRestart {
						logger.WithField("timeoutCounter", timeoutCounter).Warn("restarting subcription")
						cancelLoop()
						break EventSelect
					}
				} else {
					timeoutCounter = 0
				}

				lastTimeout = time.Now()
				timer.Reset(b.c.BeaconEventTimeout)
			case <-ctx.Done():
				cancelLoop()
				return
			}
		}
	}
}

*/

func SubscribeToPayloadAttributesEvents(lo log.Logger, endpoint string, outC chan PayloadAttributesEvent) {
	eventsURL := fmt.Sprintf("%s/eth/v1/events?topics=payload_attributes", endpoint)
	l := lo.WithField("url", eventsURL)

	client := sse.NewClient(eventsURL)
	for {
		err := client.SubscribeRaw(func(msg *sse.Event) {
			var data PayloadAttributesEvent
			err := json.Unmarshal(msg.Data, &data)
			if err != nil {
				l.WithError(err).Error("could not unmarshal payload_attributes event")
			} else {
				outC <- data
			}
		})
		if err != nil {
			l.WithError(err).Error("failed to subscribe to payload_attributes events")
			time.Sleep(1 * time.Second)
		}
		l.Warn("beaconclient SubscribeRaw ended, reconnecting")
	}
}
