package inits

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	bcli "github.com/blocknative/dreamboat/beacon/client"
	"github.com/blocknative/dreamboat/cmd/dreamboat/config"

	wh "github.com/blocknative/dreamboat/datastore/warehouse"
	"github.com/blocknative/dreamboat/metrics"
	"github.com/blocknative/dreamboat/relay"
	"github.com/blocknative/dreamboat/stream"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/google/uuid"
	"github.com/lthibault/log"
)

func Streamer(ctx *context.Context, l log.Logger, cfg config.DistributedConfig, transport stream.Pubsub, st stream.State) (relay.Streamer, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	timeStreamStart := time.Now()

	id := cfg.InstanceID
	if id == "" {
		id = uuid.NewString()
	}

	streamConfig := stream.StreamConfig{
		Logger:          l,
		ID:              id,
		PubsubTopic:     c.String("relay-distribution-stream-topic"),
		StreamQueueSize: c.Int("relay-distribution-stream-queue"),
	}

	streamer := stream.NewClient(transport, st, streamConfig)

	if err := streamer.RunSubscriberParallel(ctx, c.Uint("relay-distribution-stream-workers")); err != nil {
		return nil, fmt.Errorf("fail to start stream subscriber: %w", err)
	}

	l.With(log.F{
		"relay-service": "stream-subscriber",
		"startTimeMs":   time.Since(timeStreamStart).Milliseconds(),
	}).Info("initialized")

	return streamer, nil
}

func Warehouse(ctx context.Context, l log.Logger, conf *config.WarehouseConfig) (relayWh *wh.Warehouse, err error) {

	warehouse := wh.NewWarehouse(l, conf.Buffer)

	if err := os.MkdirAll(conf.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create datadir: %w", err)
	}

	if err := warehouse.RunParallel(ctx, conf.Directory, conf.WorkerNumber); err != nil {
		return nil, fmt.Errorf("failed to run data exporter: %w", err)
	}

	l.With(log.F{
		"subService": "warehouse",
		"datadir":    conf.Directory,
		"workers":    conf.WorkerNumber,
	}).Info("initialized")

	return warehouse, nil
}

func BeaconClients(l log.Logger, mbc *bcli.MultiBeaconClient, endpoints []string, m *metrics.Metrics, c *bcli.BeaconConfig) error {
	for _, endpoint := range endpoints {
		u, err := url.Parse(endpoint)
		if err != nil {
			return err
		}

		bc := bcli.NewBeaconClient(l, u, c)
		mbc.Add(bc)
		go bc.SubscribeToHeadEvents(mbc.HeadEventsSubscription())
		// TODO
		//if enabled
		//	go bc.SubscribeToPayloadAttributesEvents(mbc.PayloadAttributesSubscription())
	}
	return nil
}

// ComputeDomain computes the signing domain
func ComputeDomain(domainType types.DomainType, forkVersionHex string, genesisValidatorsRootHex string) (domain types.Domain, err error) {
	genesisValidatorsRoot := types.Root(common.HexToHash(genesisValidatorsRootHex))
	forkVersionBytes, err := hexutil.Decode(forkVersionHex)
	if err != nil || len(forkVersionBytes) > 4 {
		err = errors.New("invalid fork version passed")
		return domain, err
	}
	var forkVersion [4]byte
	copy(forkVersion[:], forkVersionBytes[:4])
	return types.ComputeDomain(domainType, forkVersion, genesisValidatorsRoot), nil
}
