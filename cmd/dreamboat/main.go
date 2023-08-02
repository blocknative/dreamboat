package main

import (
	"context"
	"errors"
	"flag"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"

	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"

	"github.com/blocknative/dreamboat/api"
	"github.com/blocknative/dreamboat/api/inner"
	"github.com/blocknative/dreamboat/auction"
	"github.com/blocknative/dreamboat/beacon"
	bcli "github.com/blocknative/dreamboat/beacon/client"
	"github.com/blocknative/dreamboat/blstools"
	wh "github.com/blocknative/dreamboat/datastore/warehouse"
	"github.com/blocknative/dreamboat/metrics"
	"github.com/blocknative/dreamboat/sim"
	"github.com/blocknative/dreamboat/sim/client/fallback"
	"github.com/blocknative/dreamboat/stream"
	badger "github.com/ipfs/go-ds-badger2"

	fileS "github.com/blocknative/dreamboat/cmd/dreamboat/config/source/file"
	redis_stream "github.com/blocknative/dreamboat/stream/transport/redis"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/datastore"
	dsRedis "github.com/blocknative/dreamboat/datastore/transport/redis"
	relay "github.com/blocknative/dreamboat/relay"
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/validators"
	"github.com/blocknative/dreamboat/verify"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"

	trPostgres "github.com/blocknative/dreamboat/datastore/transport/postgres"

	trBadger "github.com/blocknative/dreamboat/datastore/transport/badger"

	daBadger "github.com/blocknative/dreamboat/datastore/evidence/badger"
	daPostgres "github.com/blocknative/dreamboat/datastore/evidence/postgres"

	valBadger "github.com/blocknative/dreamboat/datastore/validator/badger"
	valPostgres "github.com/blocknative/dreamboat/datastore/validator/postgres"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/lthibault/log"
)

const (
	gethSimNamespace = "flashbots"
	shutdownTimeout  = 15 * time.Second
)

var (
	loglvl     string
	logfmt     string
	datadir    string
	configFile string

	flagDistribution bool
	flagWarehouse    bool
)

func init() {
	flag.StringVar(&loglvl, "loglvl", "info", "logging level: trace, debug, info, warn, error or fatal")
	flag.StringVar(&logfmt, "logfmt", "text", "format logs as text, json or none")
	flag.StringVar(&configFile, "config", "", "configuration file needed for relay to run")
	flag.StringVar(&datadir, "datadir", "/tmp/relay", "data directory where blocks and validators are stored in the default datastore implementation")

	flag.BoolVar(&flagDistribution, "relay-distribution", false, "run relay as a distributed system with multiple replicas")
	flag.BoolVar(&flagWarehouse, "warehouse", true, "Enable warehouse storage of data")
	flag.Parse()
}

// Main starts the relay
func main() {
	ctx, cancel := context.WithCancel(context.Background())

	termSig := make(chan os.Signal, 2)
	signal.Notify(termSig, syscall.SIGTERM)
	signal.Notify(termSig, syscall.SIGINT)
	go waitForSignal(cancel, termSig)

	reloadSig := make(chan os.Signal, 2)
	signal.Notify(reloadSig, syscall.SIGHUP)

	logger := logger(loglvl, logfmt, false, false, os.Stdout)

	cfg := config.NewConfigManager(fileS.NewSource(configFile))
	if configFile != "" {
		if err := cfg.Load(); err != nil {
			logger.WithError(err).Fatal("failed loading config file")
			return
		}
		go reloadConfigSignal(logger, reloadSig, cfg)
	}

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

	var (
		storage  datastore.TTLStorage
		badgerDs *badger.Datastore
		streamer *stream.Client
		err      error
	)

	badgerDs, err = trBadger.Open(datadir)
	if err != nil {
		logger.WithError(err).Error("failed to initialize datastore")
		return
	}
	if err = trBadger.InitDatastoreMetrics(m); err != nil {
		logger.WithError(err).Error("failed to initialize datastore metrics")
		return
	}

	state := &beacon.MultiSlotState{}

	if flagDistribution {
		readClient := redis.NewClient(&redis.Options{
			Addr: cfg.Payload.Redis.Read.Address,
		})
		defer readClient.Close()
		m.RegisterRedis("redis", "readreplica", readClient)

		writeClient := redis.NewClient(&redis.Options{
			Addr: cfg.Payload.Redis.Write.Address,
		})
		defer writeClient.Close()
		m.RegisterRedis("redis", "master", writeClient)
		storage = &dsRedis.RedisDatastore{Read: readClient, Write: writeClient}

		redisClient := redis.NewClient(&redis.Options{
			Addr: cfg.Distributed.Redis.Address,
		})
		defer redisClient.Close()
		m.RegisterRedis("redis", "stream", redisClient)

		streamer = newStreamClient(cfg.Distributed, redisClient, logger, m, state)
		defer streamer.Close()

	} else {
		storage = badgerDs
	}

	logger.With(log.F{
		"subService":  "datastore",
		"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
	}).Info("data store initialized")

	timeRelayStart := time.Now()
	ds := datastore.NewDatastore(storage, badgerDs.DB)
	if err != nil {
		logger.WithError(err).Error("failed to create datastore")
		return
	}

	beaconConfig := bcli.BeaconConfig{
		BeaconEventTimeout: cfg.Beacon.EventTimeout,
		BeaconEventRestart: cfg.Beacon.EventRestart,
		BeaconQueryTimeout: cfg.Beacon.QueryTimeout,
	}

	beaconCli, err := initBeaconClients(logger, cfg.Beacon.Addresses, nil, m, beaconConfig)
	if err != nil {
		logger.WithError(err).Error("fail to initialize beacon")
		return
	}

	beaconPubCli, err := initBeaconClients(logger, cfg.Beacon.PublishAddresses, cfg.Beacon.PublishAddressesV2, m, beaconConfig)
	if err != nil {
		logger.WithError(err).Error("fail to initialize publish beacon")
		return
	}

	simFallb := fallback.NewFallback()
	simFallb.AttachMetrics(m)

	simManager := sim.NewManager(logger, simFallb)
	simManager.AddRPCClient(ctx, cfg.BlockSimulation.RPC.Address)
	simManager.AddHTTPClient(ctx, cfg.BlockSimulation.HTTP.Address)
	for _, addr := range cfg.BlockSimulation.WS.Address {
		simManager.AddWsClients(ctx, addr, cfg.BlockSimulation.WS.Retry)
	}

	verificator := verify.NewVerificationManager(logger, cfg.Verify.QueueSize)
	verificator.RunVerify(uint(cfg.Verify.WorkersNum))

	// VALIDATOR MANAGEMENT
	var valDS ValidatorStore
	if cfg.Validators.DB.URL != "" {
		valPG, err := trPostgres.Open(cfg.Validators.DB.URL, cfg.Validators.DB.MaxOpenConns, cfg.Validators.DB.MaxIdleConns, cfg.Validators.DB.ConnMaxIdleTime)
		if err != nil {
			logger.WithError(err).Error("failed to connect to validator database")
			return
		}
		m.RegisterDB(valPG, "registrations")
		valDS = valPostgres.NewDatastore(valPG)
	} else { // by default use existsing storage
		valDS = valBadger.NewDatastore(storage, cfg.Validators.Badger.TTL)
	}

	validatorCache, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](cfg.Validators.RegistrationsCacheSize)
	if err != nil {
		logger.WithError(err).Error("fail to initialize validator cache")
		return
	}

	// DATAAPI
	var daDS relay.DataAPIStore
	if cfg.DataAPI.DB.URL != "" {
		valPG, err := trPostgres.Open(cfg.DataAPI.DB.URL, cfg.DataAPI.DB.MaxOpenConns, cfg.DataAPI.DB.MaxIdleConns, cfg.DataAPI.DB.ConnMaxIdleTime)
		if err != nil {
			logger.WithError(err).Error("failed to connect to dataapi database")
		}
		m.RegisterDB(valPG, "dataapi")
		daDSPost := daPostgres.NewDatastore(logger, valPG, 0)
		daDSPost.AttachMetrics(m)
		daDS = daDSPost

		defer valPG.Close()
	} else { // by default use badger
		daDS = daBadger.NewDatastore(storage, badgerDs.DB, cfg.DataAPI.Badger.TTL)
	}

	// lazyload validators cache, it's optional and we don't care if it errors out
	go preloadValidators(ctx, logger, valDS, cfg.Validators.RegistrationsWriteCacheTTL, validatorCache)

	validatorStoreManager := validators.NewStoreManager(logger, validatorCache, valDS, cfg.Validators.RegistrationsWriteCacheTTL, cfg.Validators.QueueSize)
	validatorStoreManager.AttachMetrics(m)
	if cfg.Validators.StoreWorkersNum > 0 {
		validatorStoreManager.RunStore(uint(cfg.Validators.StoreWorkersNum))
	}

	domainBuilder, err := ComputeDomain(types.DomainTypeAppBuilder, chainCfg.GenesisForkVersion, types.Root{}.String())
	if err != nil {
		logger.WithError(err).Error("fail to compute builder domain")
		return
	}

	validatorRelay := validators.NewRegister(logger, domainBuilder, state, verificator, validatorStoreManager)
	validatorRelay.AttachMetrics(m)
	b := beacon.NewManager(logger, beacon.Config{
		BellatrixForkVersion:             chainCfg.BellatrixForkVersion,
		CapellaForkVersion:               chainCfg.CapellaForkVersion,
		RunPayloadAttributesSubscription: cfg.Beacon.PayloadAttributesSubscription,
	})

	skBytes, err := hexutil.Decode(cfg.Relay.SecretKey)
	if err != nil {
		logger.WithError(err).Error("fail to decode secretKey")
		return
	}
	sk, pk, err := blstools.SecretKeyFromBytes(skBytes)
	if err != nil {
		logger.WithError(err).Error("fail to decode secretKey (public key)")
		return
	}

	bellatrixBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, chainCfg.BellatrixForkVersion, chainCfg.GenesisValidatorsRoot)
	if err != nil {
		logger.WithError(err).Error("fail to compute proposer domain (bellatrix)")
		return
	}

	capellaBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, chainCfg.CapellaForkVersion, chainCfg.GenesisValidatorsRoot)
	if err != nil {
		logger.WithError(err).Error("fail to compute proposer domain (capella)")
		return
	}

	var relayWh *wh.Warehouse
	if flagWarehouse {
		warehouse := wh.NewWarehouse(logger, cfg.Warehouse.Buffer)
		if err := os.MkdirAll(cfg.Warehouse.Directory, 0755); err != nil {
			logger.WithError(err).Error("failed to create datadir")
			return
		}

		if err := warehouse.RunParallel(ctx, cfg.Warehouse.Directory, cfg.Warehouse.WorkerNumber); err != nil {
			logger.WithError(err).Error("failed to run data exporter")
			return
		}

		warehouse.AttachMetrics(m)

		logger.With(log.F{
			"subService": "warehouse",
			"datadir":    cfg.Warehouse.Directory,
			"workers":    cfg.Warehouse.WorkerNumber,
		}).Info("initialized")

		relayWh = warehouse
	}

	payloadCache, err := structs.NewMultiSlotPayloadCache(cfg.Payload.CacheSize)
	if err != nil {
		logger.WithError(err).Error("fail to initialize stream cache")
		return
	}

	rCfg := &relay.RelayConfig{
		L:                          logger,
		BuilderSigningDomain:       domainBuilder,
		GetPayloadResponseDelay:    cfg.Relay.GetPayloadResponseDelay,
		GetPayloadRequestTimeLimit: cfg.Relay.GetPayloadRequestTimeLimit,
		ProposerSigningDomain: map[structs.ForkVersion]types.Domain{
			structs.ForkBellatrix: bellatrixBeaconProposer,
			structs.ForkCapella:   capellaBeaconProposer},
		PubKey:               pk,
		SecretKey:            sk,
		RegistrationCacheTTL: cfg.Validators.RegistrationsReadCacheTTL,
		PayloadDataTTL:       cfg.Payload.TTL,
		PublishBlock:         cfg.Relay.PublishBlock,
		Distributed:          flagDistribution,
		StreamServedBids:     cfg.Distributed.StreamServedBids,
	}
	rCfg.ParseInitialConfig(cfg.Relay.AllowedBuilders)
	cfg.Relay.SubscribeForUpdates(rCfg)

	auctioneer := auction.NewAuctioneer()
	r := relay.NewRelay(logger, rCfg, beaconPubCli, validatorCache, valDS, verificator,
		state, payloadCache, ds, daDS, auctioneer, simFallb, relayWh, streamer)
	r.AttachMetrics(m)

	if flagDistribution {
		r.RunSubscribersParallel(ctx, uint(cfg.Distributed.WorkerNumber))
	}

	ee := &api.EnabledEndpoints{
		GetHeader:   true,
		GetPayload:  true,
		SubmitBlock: true,
	}
	iApi := inner.NewAPI(ee, ds, cfg)

	limitterCache, _ := lru.New[[48]byte, *rate.Limiter](cfg.Api.LimitterCacheSize)
	apiLimitter := api.NewLimitter(logger, cfg.Api.SubmissionLimitRate, cfg.Api.SubmissionLimitBurst, limitterCache)
	apiLimitter.ParseInitialConfig(cfg.Api.AllowedBuilders)
	cfg.Api.SubscribeForUpdates(apiLimitter)

	a := api.NewApi(logger, ee, r, validatorRelay, state, apiLimitter, cfg.Api.DataLimit, cfg.Api.ErrorsOnDisable)
	a.AttachMetrics(m)
	cfg.Api.SubscribeForUpdates(a)
	logger.With(log.F{
		"service":     "relay",
		"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
	}).Info("initialized")

	if err := b.Init(ctx, state, beaconCli, validatorStoreManager, validatorCache); err != nil {
		logger.WithError(err).Error("failed to init beacon manager")
		return
	}

	go b.Run(ctx, state, beaconCli, validatorStoreManager, validatorCache)

	logger.Info("beacon manager ready")

	internalMux := http.NewServeMux()
	iApi.AttachToHandler(internalMux)

	// run internal http server
	go func(m *metrics.Metrics, internalMux *http.ServeMux) (err error) {
		metrics.AttachProfiler(internalMux)
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
		IdleTimeout:    cfg.ExternalHttp.IdleTimeout,
		Handler:        mux,
		MaxHeaderBytes: 4096,
	}
	// run the http server
	go func(srv *http.Server) (err error) {
		logger.Info("http server listening")
		if err = srv.ListenAndServe(); err == http.ErrServerClosed {
			err = nil
		}
		logger.Info("http server finished")
		return err
	}(srv)

	<-ctx.Done()

	ctx, closeC := context.WithTimeout(context.Background(), shutdownTimeout/2)
	defer closeC()
	logger.Info("Shutdown initialized")
	err = srv.Shutdown(ctx)
	logger.Info("Shutdown returned ", err)

	ctx, closeC = context.WithTimeout(context.Background(), shutdownTimeout)
	defer closeC()
	finish := make(chan struct{})
	go closemanager(ctx, finish, validatorStoreManager, r, relayWh)

	select {
	case <-finish:
	case <-ctx.Done():
		logger.Warn("Closing manager deadline exceeded ")
	}

}

func waitForSignal(cancel context.CancelFunc, osSig chan os.Signal) {
	for range osSig {
		cancel()
		return
	}
}

func reloadConfigSignal(l log.Logger, osSig chan os.Signal, cfg *config.ConfigManager) {
	for range osSig {
		if err := cfg.Reload(); err != nil {
			l.WithError(err).Warn("errror reloading config")
		}

	}
}

type ValidatorStore interface {
	GetRegistration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
	PutNewerRegistration(ctx context.Context, pk types.PublicKey, registration types.SignedValidatorRegistration) error
	PopulateAllRegistrations(ctx context.Context, out chan structs.ValidatorCacheEntry) error
}

func asyncPopulateAllRegistrations(ctx context.Context, l log.Logger, vs ValidatorStore, ch chan structs.ValidatorCacheEntry) {
	defer close(ch)
	err := vs.PopulateAllRegistrations(ctx, ch)
	if err != nil {
		l.WithError(err).Warn("Cache population error")
	}
}

func preloadValidators(ctx context.Context, l log.Logger, vs ValidatorStore, writeTTL time.Duration, vc *lru.Cache[types.PublicKey, structs.ValidatorCacheEntry]) {
	ch := make(chan structs.ValidatorCacheEntry, 100)
	go asyncPopulateAllRegistrations(ctx, l, vs, ch)
	var refreshedTTLNum uint64
	for v := range ch {
		k := v
		// this is needed for deployments and restarts, so we would not purge cache entirely
		if time.Since(v.Time).Seconds() > writeTTL.Seconds()*0.5 {
			// set initial timer between 1/4 and 1/2 amount cache
			k.Time = time.Now().Add(-1 * (time.Duration(int64(0.25*writeTTL.Seconds())) + time.Duration(rand.Int63n(int64(0.25*writeTTL.Seconds()))*int64(time.Second))))
			refreshedTTLNum++
		}
		vc.ContainsOrAdd(v.Entry.Message.Pubkey, k)
	}
	l.With(log.F{"count": vc.Len(), "refreshed": refreshedTTLNum}).Info("Loaded cache validators")
}

func initBeaconClients(l log.Logger, endpoints []string, v2endpoints []string, m *metrics.Metrics, c bcli.BeaconConfig) (*bcli.MultiBeaconClient, error) {
	clients := make([]bcli.BeaconNode, 0, len(endpoints))

	for _, endpoint := range endpoints {
		client, err := bcli.NewBeaconClient(l, endpoint, c)
		if err != nil {
			return nil, err
		}
		client.AttachMetrics(m) // attach metrics
		clients = append(clients, client)
	}

	mbc := bcli.NewMultiBeaconClient(l, clients)
	for _, endpoint := range v2endpoints {
		client, err := bcli.NewBeaconClient(l, endpoint, c)
		if err != nil {
			return nil, err
		}
		client.AttachMetrics(m) // attach metrics
		mbc.PublishV2Clients = append(mbc.PublishV2Clients, client)
	}
	return mbc, nil
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

func newStreamClient(cfg *config.DistributedConfig, redisClient *redis.Client, l log.Logger, m *metrics.Metrics, st stream.State) *stream.Client {
	bids := &redis_stream.Topic{
		Redis:     redisClient,
		Logger:    l.WithField("topic", stream.BidTopic),
		Name:      path.Join(cfg.Redis.Topic, stream.BidTopic),
		LocalNode: cfg.LocalNode(),
	}

	cache := &redis_stream.Topic{
		Redis:     redisClient,
		Logger:    l.WithField("topic", stream.CacheTopic),
		Name:      path.Join(cfg.Redis.Topic, stream.CacheTopic),
		LocalNode: cfg.LocalNode(),
	}

	return &stream.Client{
		Logger: l.
			WithField("subService", "stream").
			WithField("type", "redis"),
		Metrics:    m,
		State:      st,
		Bids:       bids,
		Cache:      cache,
		QueueSize:  cfg.StreamQueueSize,
		NumWorkers: cfg.WorkerNumber,
	}
}
