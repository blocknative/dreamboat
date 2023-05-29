package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blocknative/dreamboat/api"
	"github.com/blocknative/dreamboat/api/inner"
	"github.com/blocknative/dreamboat/auction"
	"github.com/blocknative/dreamboat/beacon"
	bcli "github.com/blocknative/dreamboat/beacon/client"
	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	fileS "github.com/blocknative/dreamboat/cmd/dreamboat/config/source/file"
	"github.com/blocknative/dreamboat/datastore"
	daBadger "github.com/blocknative/dreamboat/datastore/evidence/badger"
	"github.com/blocknative/dreamboat/metrics"
	"github.com/blocknative/dreamboat/relay"
	"github.com/blocknative/dreamboat/sim/client/fallback"
	"github.com/blocknative/dreamboat/stream"
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/validators"
	"github.com/blocknative/dreamboat/verify"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"

	wh "github.com/blocknative/dreamboat/datastore/warehouse"

	trBadger "github.com/blocknative/dreamboat/datastore/transport/badger"
	trPostgres "github.com/blocknative/dreamboat/datastore/transport/postgres"

	daPostgres "github.com/blocknative/dreamboat/datastore/evidence/postgres"

	valBadger "github.com/blocknative/dreamboat/datastore/validator/badger"
	valPostgres "github.com/blocknative/dreamboat/datastore/validator/postgres"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/lthibault/log"
)

const (
	shutdownTimeout = 45 * time.Second
)

var (
	loglvl     string
	logfmt     string
	datadir    string
	configFile string
)

func init() {
	flag.StringVar(&loglvl, "loglvl", "info", "logging level: trace, debug, info, warn, error or fatal")
	flag.StringVar(&logfmt, "logfmt", "text", "format logs as text, json or none")
	flag.StringVar(&configFile, "config", "./config", "configuration file needed for relay to run")
	flag.StringVar(&datadir, "datadir", "/tmp/relay", "data directory where blocks and validators are stored in the default datastore implementation")
	flag.Parse()
}

// Main starts the relay
func main() {
	ctx, cancel := context.WithCancel(context.Background())

	osSig := make(chan os.Signal, 2)
	signal.Notify(osSig, syscall.SIGTERM)
	signal.Notify(osSig, syscall.SIGINT)
	go waitForSignal(cancel, osSig)

	reloadSig := make(chan os.Signal, 2)
	signal.Notify(reloadSig, syscall.SIGHUP)
	cFile := fileS.NewSource(configFile)

	logger := logger(loglvl, logfmt, false, false, os.Stdout)

	cfg := config.NewConfigManager(cFile)
	if err := cfg.Load(); err != nil {
		logger.WithError(err).Fatal("failed loading config file")
		return
	}

	go reloadConfigSignal(reloadSig, cfg)
	chainCfg := config.NewChainConfig()
	chainCfg.LoadNetwork(cfg.Relay.Network)
	if chainCfg.GenesisForkVersion == "" {
		if err := chainCfg.ReadNetworkConfig(datadir, cfg.Relay.Network); err != nil {
			logger.WithError(err).Fatal("failed read chain configuration")
			return
		}
	}

	timeDataStoreStart := time.Now()
	m := metrics.NewMetrics()

	// BadgerDB
	storage, err := trBadger.Open(datadir)
	if err != nil {
		logger.WithError(err).Fatal("failed to initialize datastore")
		return
	}

	if err = trBadger.InitDatastoreMetrics(m); err != nil {
		logger.WithError(err).Fatal("failed to initialize datastore metrics")
		return
	}

	logger.With(log.F{
		"service":     "datastore",
		"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
	}).Info("data store initialized")

	timeRelayStart := time.Now()

	beaconCli := bcli.NewMultiBeaconClient(logger)
	cfg.Beacon.SubscribeForUpdates(beaconCli)

	bcfg := &bcli.BeaconConfig{
		EventTimeout: cfg.Beacon.EventTimeout,
		EventRestart: cfg.Beacon.EventRestart,
		QueryTimeout: cfg.Beacon.QueryTimeout,
	}
	cfg.Beacon.SubscribeForUpdates(bcfg)
	if err := initBeaconClients(logger, beaconCli, cfg.Beacon.Addresses, m, bcfg); err != nil {
		logger.Fatalf("fail to initialize beacon: %w", err)
		return
	}
	/*

	   beaconConfig := bcli.BeaconConfig{
	   		BeaconEventTimeout: c.Duration("beacon-event-timeout"),
	   		BeaconEventRestart: c.Int("beacon-event-restart"),
	   		BeaconQueryTimeout: c.Duration("beacon-query-timeout"),
	   	}
	   	beaconCli, err := initBeaconClients(logger, c.StringSlice("beacon"), m, beaconConfig)
	   	if err != nil {
	   		return fmt.Errorf("fail to initialize beacon: %w", err)
	   	}

	   	beaconPubCli, err := initBeaconClients(logger, c.StringSlice("beacon-publish"), m, beaconConfig)
	   	if err != nil {
	   		return fmt.Errorf("fail to initialize publish beacon: %w", err)
	   	}

	*/

	// SIM Client
	simFallb := fallback.NewFallback()
	simFallb.AttachMetrics(m)
	/*

		if simHttpAddr := cfg.BlockSimulation.RPC.Address; simHttpAddr != "" {
			simRPCCli := gethrpc.NewClient(gethSimNamespace, simHttpAddr)
			if err := simRPCCli.Dial(ctx); err != nil {
				logger.WithError(err).Fatalf("fail to initialize rpc connection (%s): %w", simHttpAddr, err)
				return
			}
			simFallb.AddClient(simRPCCli)
		}

		if len(cfg.BlockSimulation.WS.Address) > 0 {
			simWSConn := gethws.NewReConn(logger)
			for _, s := range cfg.BlockSimulation.WS.Address {
				input := make(chan []byte, 1000)
				go simWSConn.KeepConnection(s, input)
			}
			simWSCli := gethws.NewClient(simWSConn, gethSimNamespace, cfg.BlockSimulation.WS.Retry, logger)
			simFallb.AddClient(simWSCli)
		}

		if simHttpAddr := cfg.BlockSimulation.HTTP.Address; simHttpAddr != "" {
			simHTTPCli := gethhttp.NewClient(simHttpAddr, gethSimNamespace, logger)
			simFallb.AddClient(simHTTPCli)
		}
	*/

	verificator := verify.NewVerificationManager(logger, cfg.Verify.QueueSize)
	verificator.RunVerify(cfg.Verify.QueueSize)

	// VALIDATOR MANAGEMENT
	var valDS ValidatorStore
	if cfg.Validators.DB.URL != "" {
		valPG, err := trPostgres.Open(cfg.Validators.DB.URL,
			cfg.Validators.DB.MaxOpenConns,
			cfg.Validators.DB.MaxIdleConns,
			cfg.Validators.DB.ConnMaxIdleTime)
		if err != nil {
			logger.WithError(err).Fatalf("failed to connect to the database")
			return
		}
		m.RegisterDB(valPG, "registrations")
		valDS = valPostgres.NewDatastore(valPG)
	} else { // by default use existsing storage
		valDS = valBadger.NewDatastore(storage, cfg.Validators.Badger.TTL)
	}

	validatorCache, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](cfg.Validators.RegistrationsCacheSize)
	if err != nil {
		logger.WithError(err).Fatalf("fail to initialize validator cache")
		return
	}

	// DATAAPI
	var daDS relay.DataAPIStore
	if cfg.DataAPI.DB.URL != "" {
		daPG, err := trPostgres.Open(cfg.DataAPI.DB.URL,
			cfg.DataAPI.DB.MaxOpenConns,
			cfg.DataAPI.DB.MaxIdleConns,
			cfg.DataAPI.DB.ConnMaxIdleTime)
		if err != nil {
			logger.WithError(err).Fatal("failed to connect to the database")
			return
		}
		m.RegisterDB(daPG, "dataapi")
		daDS = daPostgres.NewDatastore(daPG, 0)
		defer daPG.Close()
	} else { // by default use existsing storage
		daDS = daBadger.NewDatastore(storage, storage.DB, cfg.DataAPI.Badger.TTL)
	}

	// lazyload validators cache, it's optional and we don't care if it errors out
	go preloadValidators(ctx, logger, valDS, validatorCache)
	validatorStoreManager := validators.NewStoreManager(logger, validatorCache, valDS, int(math.Floor(cfg.Validators.Badger.TTL.Seconds()/2)), cfg.Validators.QueueSize)
	validatorStoreManager.AttachMetrics(m)
	if cfg.Validators.StoreWorkersNum > 0 {
		validatorStoreManager.RunStore(cfg.Validators.StoreWorkersNum)
	}

	domainBuilder, err := ComputeDomain(types.DomainTypeAppBuilder, chainCfg.GenesisForkVersion, types.Root{}.String())
	if err != nil {
		logger.WithError(err).Fatal("error computing genesis domain")
		return
	}

	state := &beacon.MultiSlotState{}

	validatorRelay := validators.NewRegister(logger, domainBuilder, state, verificator, validatorStoreManager)
	validatorRelay.AttachMetrics(m)
	b := beacon.NewManager(logger, beacon.Config{
		BellatrixForkVersion: chainCfg.BellatrixForkVersion,
		CapellaForkVersion:   chainCfg.CapellaForkVersion,
	})

	auctioneer := auction.NewAuctioneer()

	skBytes, err := hexutil.Decode(cfg.Relay.SecretKey)
	if err != nil {
		logger.WithError(err).Fatal("decoding secret key")
		return
	}
	sk, pk, err := blstools.SecretKeyFromBytes(skBytes)
	if err != nil {
		logger.WithError(err).Fatal("getting secret key")
		return
	}

	bellatrixBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, chainCfg.BellatrixForkVersion, chainCfg.GenesisValidatorsRoot)
	if err != nil {
		logger.WithError(err).Fatal("error computing bellatrix domain")
		return
	}

	capellaBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, chainCfg.CapellaForkVersion, chainCfg.GenesisValidatorsRoot)
	if err != nil {
		logger.WithError(err).Fatal("error computing capella domain")
		return
	}
	/*
		payloadCache, err := lru.New[structs.PayloadKey, structs.BlockBidAndTrace](cfg.Payload.CacheSize)
		if err != nil {
			logger.WithError(err).Fatal("failed to initialize payload cache")
			return
		}
	*/
	ds := datastore.NewDatastore(storage, storage.DB, cfg.Payload.Badger.TTL)

	var allowed map[[48]byte]struct{}
	if len(cfg.Relay.AllowedBuilders) > 0 {
		allowed = make(map[[48]byte]struct{})
		for _, k := range cfg.Relay.AllowedBuilders {
			var pk types.PublicKey
			if err := pk.UnmarshalText([]byte(k)); err != nil {
				logger.WithError(err).With(log.F{"key": k}).Error("ALLOWED BUILDER NOT ADDED - wrong public key")
				continue
			}
			allowed[pk] = struct{}{}
		}
	}

	rcfg := &relay.RelayConfig{
		BuilderSigningDomain: domainBuilder,
		ProposerSigningDomain: map[structs.ForkVersion]types.Domain{
			structs.ForkBellatrix: bellatrixBeaconProposer,
			structs.ForkCapella:   capellaBeaconProposer},
		PubKey:                pk,
		SecretKey:             sk,
		RegistrationCacheTTL:  cfg.Validators.RegistrationsCacheTTL,
		AllowedListedBuilders: allowed,
		PublishBlock:          cfg.Relay.PublishBlock,
		MaxBlockPublishDelay:  cfg.Relay.MaxBlockPublishDelay,
	}
	cfg.Relay.SubscribeForUpdates(rcfg)
	r := relay.NewRelay(logger, rcfg, beaconCli, validatorCache, valDS, verificator, state, ds, daDS, auctioneer, simFallb, relayWh, streamer)
	r.AttachMetrics(m)

	ee := &api.EnabledEndpoints{
		GetHeader:   true,
		GetPayload:  true,
		SubmitBlock: true,
	}
	iApi := inner.NewAPI(ee, ds)

	limitter := api.NewLimitter(cfg.Api.SubmissionLimitRate, cfg.Api.SubmissionLimitBurst, allowed)
	cfg.Api.SubscribeForUpdates(limitter)

	a := api.NewApi(logger, ee, r, validatorRelay, state, limitter)
	a.AttachMetrics(m)
	logger.With(log.F{
		"service":     "relay",
		"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
	}).Info("initialized")

	if err := b.Init(ctx, state, beaconCli, validatorStoreManager, validatorCache); err != nil {
		logger.Fatalf("failed to init beacon manager: %w", err)
	}

	go b.Run(ctx, state, beaconCli, validatorStoreManager, validatorCache)

	//if s.Config.RunPayloadAttributesSubscription {
	// go b.RunPayloadAttributesSubscription(ctx, state, beaconCli.PayloadAttributesSubscription(), beaconCli.HeadEventsSubscription())
	//}

	logger.Info("beacon manager ready")

	internalMux := http.NewServeMux()
	iApi.AttachToHandler(internalMux)
	metrics.AttachProfiler(internalMux)

	// run internal http server
	go func(m *metrics.Metrics, internalMux *http.ServeMux) (err error) {
		internalMux.Handle("/metrics", m.Handler())
		logger.Info("internal server listening")
		internalSrv := http.Server{
			Addr:    cfg.InternalHttp.Address,
			Handler: internalMux,
		}

		if err = internalSrv.ListenAndServe(); err == http.ErrServerClosed {
			err = nil
		}
		return err
	}(m, internalMux)

	mux := http.NewServeMux()
	a.AttachToHandler(mux)
	srv := &http.Server{
		Addr:           cfg.ExternalHttp.Address,
		ReadTimeout:    cfg.ExternalHttp.ReadTimeout,
		WriteTimeout:   cfg.ExternalHttp.WriteTimeout,
		IdleTimeout:    time.Second * 2,
		Handler:        mux,
		MaxHeaderBytes: 4096,
	}

	logger.Info("http server listening")
	if err := srv.ListenAndServe(); err == http.ErrServerClosed {
		err = nil
	}

	logger.Info("Shutdown initialized")
	err = srv.Shutdown(ctx)
	logger.Info("Shutdown returned ", err)

	ctx, closeC := context.WithTimeout(context.Background(), shutdownTimeout)
	defer closeC()
	finish := make(chan struct{})
	go closemanager(ctx, finish, validatorStoreManager, r)

	select {
	case <-finish:
	case <-ctx.Done():
		logger.Warn("Closing, manager deadline exceeded ")
	}

}

type ValidatorStore interface {
	GetRegistration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
	PutNewerRegistration(ctx context.Context, pk types.PublicKey, registration types.SignedValidatorRegistration) error
	PopulateAllRegistrations(ctx context.Context, out chan structs.ValidatorCacheEntry) error
}

func waitForSignal(cancel context.CancelFunc, osSig chan os.Signal) {
	for range osSig {
		cancel()
		return
	}
}

func reloadConfigSignal(osSig chan os.Signal, cfg *config.ConfigManager) {
	for range osSig {
		cfg.Reload()
	}
}

func preloadValidators(ctx context.Context, l log.Logger, vs ValidatorStore, vc *lru.Cache[types.PublicKey, structs.ValidatorCacheEntry]) {
	ch := make(chan structs.ValidatorCacheEntry, 100)
	go asyncPopulateAllRegistrations(ctx, l, vs, ch)
	for v := range ch {
		v := v
		vc.ContainsOrAdd(v.Entry.Message.Pubkey, v)
	}
	l.With(log.F{"count": vc.Len()}).Info("Loaded cache validators")
}

func asyncPopulateAllRegistrations(ctx context.Context, l log.Logger, vs ValidatorStore, ch chan structs.ValidatorCacheEntry) {
	defer close(ch)
	if err := vs.PopulateAllRegistrations(ctx, ch); err != nil {
		l.WithError(err).Warn("Cache population error")
	}
}

func initBeaconClients(l log.Logger, mbc *bcli.MultiBeaconClient, endpoints []string, m *metrics.Metrics, c *bcli.BeaconConfig) error {
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

func closemanager(ctx context.Context, finish chan struct{}, regMgr *validators.StoreManager, r *relay.Relay, relayWh *wh.Warehouse) {
	regMgr.Close(ctx)
	r.Close(ctx)
	relayWh.Close(ctx)
	finish <- struct{}{}
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

func initStreamer(c *context.Context, redisClient *redis.Client, l log.Logger, m *metrics.Metrics, st stream.State) (relay.Streamer, error) {
	timeStreamStart := time.Now()

	pubsub := &redisStream.Pubsub{Redis: redisClient, Logger: l}

	id := c.String("relay-distribution-id")
	if id == "" {
		id = uuid.NewString()
	}

	streamConfig := stream.StreamConfig{
		Logger:          l,
		ID:              id,
		PubsubTopic:     c.String("relay-distribution-stream-topic"),
		StreamQueueSize: c.Int("relay-distribution-stream-queue"),
	}

	redisStreamer := stream.NewClient(pubsub, st, streamConfig)
	redisStreamer.AttachMetrics(m)

	if err := redisStreamer.RunSubscriberParallel(c, c.Uint("relay-distribution-stream-workers")); err != nil {
		return nil, fmt.Errorf("fail to start stream subscriber: %w", err)
	}

	l.With(log.F{
		"relay-service": "stream-subscriber",
		"startTimeMs":   time.Since(timeStreamStart).Milliseconds(),
	}).Info("initialized")

	return redisStreamer, nil
}

func InitWarehouse(c *context.Context, l log.Logger, m *metrics.Metrics, st stream.State) {
	var relayWh *wh.Warehouse

	warehouse := wh.NewWarehouse(logger, c.Int("warehouse-buffer"))

	datadir := c.String("warehouse-dir")
	if err := os.MkdirAll(datadir, 0755); err != nil {
		return fmt.Errorf("failed to create datadir: %w", err)
	}

	if err := warehouse.RunParallel(c.Context, datadir, c.Int("warehouse-workers")); err != nil {
		return fmt.Errorf("failed to run data exporter: %w", err)
	}

	warehouse.AttachMetrics(m)

	l.With(log.F{
		"subService": "warehouse",
		"datadir":    c.String("warehouse-dir"),
		"workers":    c.Int("warehouse-workers"),
	}).Info("initialized")

	relayWh = warehouse

}
