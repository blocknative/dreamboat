package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	"github.com/blocknative/dreamboat/sim/client/fallback"
	"github.com/blocknative/dreamboat/sim/client/transport/gethhttp"
	"github.com/blocknative/dreamboat/sim/client/transport/gethrpc"
	"github.com/blocknative/dreamboat/sim/client/transport/gethws"
	"github.com/blocknative/dreamboat/stream"
	"github.com/google/uuid"
	badger "github.com/ipfs/go-ds-badger2"

	fileS "github.com/blocknative/dreamboat/cmd/dreamboat/config/source/file"

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

	redisStream "github.com/blocknative/dreamboat/stream/transport/redis"

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

	flagAddr         string
	flagInternalAddr string

	flagTimeout           time.Duration
	flagBeaconList        string
	flagBeaconPublishList string

	flagNetwork   string
	flagSecretKey string
	flagTTL       time.Duration

	flagWorkersVerify         uint64
	flagWorkersStoreValidator uint64
	flagVerifyQueueSize       uint64
	flagStoreQueueSize        uint64
	flagPayloadCacheSize      int

	flagRegistrationsCacheSize     int
	flagRegistrationsCacheReadTTL  time.Duration
	flagRegistrationsCacheWriteTTL time.Duration

	flagPublishBlock bool

	flagValidatorDatabaseUrl string
	flagDataapiDatabaseUrl   string

	flagAllowListedBuilderList string

	flagBlockValidationEndpointHTTP   string
	flagBlockValidationEndpointWSList string
	flagBlockValidationWSRetry        bool
	flagBlockValidationRPC            string

	flagDistribution   bool
	flagDistributionID string

	flagDistributionStreamWorkers    uint64
	flagDistributionStreamServedBids bool

	flagDistributionStreamTTL   time.Duration
	flagDistributionStreamTopic string
	flagDistributionStreamQueue int

	flagStorageReadRedisURI  string
	flagStorageWriteRedisURI string
	flagPubsubRedisURI       string

	flagGetPayloadResponseDelay    time.Duration
	flagGetPayloadRequestTimeLimit time.Duration

	flagWarehouse        bool
	flagWarehouseDir     string
	flagWarehouseWorkers int
	flagWarehouseBuffer  int

	flagBeaconEventRestart                  int
	flagBeaconEventTimeout                  time.Duration
	flagBeaconQueryTimeout                  time.Duration
	flagBeaconPayloadAttributesSubscription bool
)

func init() {
	flag.StringVar(&loglvl, "loglvl", "info", "logging level: trace, debug, info, warn, error or fatal")
	flag.StringVar(&logfmt, "logfmt", "text", "format logs as text, json or none")
	flag.StringVar(&configFile, "config", "", "configuration file needed for relay to run")
	flag.StringVar(&datadir, "datadir", "/tmp/relay", "data directory where blocks and validators are stored in the default datastore implementation")

	flag.StringVar(&flagAddr, "addr", "localhost:18550", "server listen address")
	flag.StringVar(&flagInternalAddr, "internalAddr", "0.0.0.0:19550", "internal server listen address")
	flag.DurationVar(&flagTimeout, "timeout", time.Second*5, "request timeout")
	flag.StringVar(&flagBeaconList, "beacon", "", "`url` for beacon endpoint")
	flag.StringVar(&flagBeaconPublishList, "beacon-publish", "", "`url` for beacon endpoints that publish blocks")

	flag.StringVar(&flagNetwork, "network", "mainnet", "the networks the relay works on")

	flag.StringVar(&flagSecretKey, "secretKey", "", "secret key used to sign messages")
	flag.DurationVar(&flagTTL, "ttl", 24*time.Hour, "ttl of the data")

	flag.Uint64Var(&flagWorkersVerify, "relay-workers-verify", 2000, "number of workers running verify in parallel")

	flag.Uint64Var(&flagWorkersStoreValidator, "relay-workers-store-validator", 400, "number of workers storing validators in parallel")

	flag.Uint64Var(&flagVerifyQueueSize, "relay-verify-queue-size", 100_000, "size of verify queue")
	flag.Uint64Var(&flagStoreQueueSize, "relay-store-queue-size", 100_000, "size of store queue")

	flag.IntVar(&flagPayloadCacheSize, "relay-payload-cache-size", 1_000, "number of payloads to cache for fast in-memory reads")

	flag.IntVar(&flagRegistrationsCacheSize, "relay-registrations-cache-size", 600_000, "relay registrations cache size")

	flag.DurationVar(&flagRegistrationsCacheReadTTL, "relay-registrations-cache-read-ttl", time.Hour, "registrations cache ttl for reading")
	flag.DurationVar(&flagRegistrationsCacheWriteTTL, "relay-registrations-cache-write-ttl", 12*time.Hour, "registrations cache ttl for writing")

	flag.BoolVar(&flagPublishBlock, "relay-publish-block", true, "relay registrations cache size")

	flag.StringVar(&flagValidatorDatabaseUrl, "relay-validator-database-url", "", "address of postgress database for validator registrations, if empty - default, badger will be used")
	flag.StringVar(&flagDataapiDatabaseUrl, "relay-dataapi-database-url", "", "address of postgress database for dataapi, if empty - default, badger will be used")

	flag.StringVar(&flagAllowListedBuilderList, "relay-allow-listed-builder", "", "comma separated list of allowed builder pubkeys")

	flag.StringVar(&flagBlockValidationEndpointHTTP, "block-validation-endpoint-http", "", "http block validation endpoint address")
	flag.StringVar(&flagBlockValidationEndpointWSList, "block-validation-endpoint-ws", "", "ws block validation endpoint address (comma separated list)")
	flag.BoolVar(&flagBlockValidationWSRetry, "block-validation-ws-retry", false, "retry to other connection on failure")
	flag.StringVar(&flagBlockValidationRPC, "block-validation-endpoint-rpc", "", "rpc block validation rawurl (eg. ipc path)")

	flag.BoolVar(&flagDistribution, "relay-distribution", false, "run relay as a distributed system with multiple replicas")
	flag.StringVar(&flagDistributionID, "relay-distribution-id", "", "the id of the relay to differentiate from other replicas")
	flag.Uint64Var(&flagDistributionStreamWorkers, "relay-distribution-stream-workers", 100, "number of workers publishing and processing subscriptions in the stream")

	flag.BoolVar(&flagDistributionStreamServedBids, "relay-distribution-stream-served-bids", true, "stream entire block for every bid that is served in GetHeader requests.")

	flag.DurationVar(&flagDistributionStreamTTL, "relay-distribution-stream-ttl", time.Minute, "TTL of the data that is distributed")

	flag.StringVar(&flagDistributionStreamTopic, "relay-distribution-stream-topic", "relay", "Pubsub topic for streaming payloads")
	flag.IntVar(&flagDistributionStreamQueue, "relay-distribution-stream-queue", 100, "Pubsub publish queue size")

	flag.StringVar(&flagStorageReadRedisURI, "relay-storage-read-redis-uri", "", "Redis Storage URI for read replica")
	flag.StringVar(&flagStorageWriteRedisURI, "relay-storage-write-redis-uri", "", "Redis Storage URI for write")
	flag.StringVar(&flagPubsubRedisURI, "relay-pubsub-redis-uri", "", "Redis Pub/Sub URI")

	flag.DurationVar(&flagGetPayloadResponseDelay, "getpayload-response-delay", 1*time.Second, "Delay between block publication and returning request to validator")
	flag.DurationVar(&flagGetPayloadRequestTimeLimit, "getpayload-request-time-limit", 4*time.Second, "Time allowed for GetPayload requests since the slot started")

	flag.BoolVar(&flagWarehouse, "warehouse", true, "Enable warehouse storage of data")

	flag.StringVar(&flagWarehouseDir, "warehouse-dir", "/data/relay/warehouse", "Data directory where the data is stored in the warehouse")

	flag.IntVar(&flagWarehouseWorkers, "warehouse-workers", 32, "Number of workers for storing data in warehouse, if 0, then data is not exported")
	flag.IntVar(&flagWarehouseBuffer, "warehouse-buffer", 1_000, "Size of the buffer for processing requests")

	flag.DurationVar(&flagBeaconEventTimeout, "beacon-event-timeout", 16*time.Second, "The maximum time allowed to wait for head events from the beacon, we recommend setting it to 'durationPerSlot * 1.25'")
	flag.IntVar(&flagBeaconEventRestart, "beacon-event-restart", 5, "The number of consecutive timeouts allowed before restarting the head event subscription")
	flag.DurationVar(&flagBeaconQueryTimeout, "beacon-query-timeout", 20*time.Second, "The maximum time allowed to wait for a response from the beacon")
	flag.BoolVar(&flagBeaconPayloadAttributesSubscription, "beacon-payload-attributes-subscription", true, "instead of polling withdrawals+prevRandao, use SSE event (requires Prysm v4+)")

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
		go reloadConfigSignal(reloadSig, cfg)
	}

	chainCfg := config.NewChainConfig()
	chainCfg.LoadNetwork(flagNetwork)
	if chainCfg.GenesisForkVersion == "" {
		if err := chainCfg.ReadNetworkConfig(datadir, flagNetwork); err != nil {
			logger.WithError(err).Fatal("failed read chain configuration")
			return
		}
	}

	timeDataStoreStart := time.Now()
	m := metrics.NewMetrics()

	var (
		storage  datastore.TTLStorage
		badgerDs *badger.Datastore
		streamer relay.Streamer
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

	if flagDistribution {
		readClient := redis.NewClient(&redis.Options{
			Addr: flagStorageReadRedisURI,
		})
		m.RegisterRedis("redis", "readreplica", readClient)

		writeClient := redis.NewClient(&redis.Options{
			Addr: flagStorageWriteRedisURI,
		})
		m.RegisterRedis("redis", "master", writeClient)
		storage = &dsRedis.RedisDatastore{Read: readClient, Write: writeClient}
	} else {
		storage = badgerDs
	}

	logger.With(log.F{
		"subService":  "datastore",
		"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
	}).Info("data store initialized")

	timeRelayStart := time.Now()
	state := &beacon.MultiSlotState{}
	ds := datastore.NewDatastore(storage, badgerDs.DB)
	if err != nil {
		logger.WithError(err).Error("failed to create datastore")
		return
	}

	if flagDistribution {
		redisClient := redis.NewClient(&redis.Options{
			Addr: flagPubsubRedisURI,
		})

		m.RegisterRedis("redis", "stream", redisClient)
		streamer, err = initStreamer(ctx, redisClient, logger, m, state)
		if err != nil {
			logger.WithError(err).Error("fail to create streamer")
			return
		}
	}

	beaconConfig := bcli.BeaconConfig{
		BeaconEventTimeout: flagBeaconEventTimeout,
		BeaconEventRestart: flagBeaconEventRestart,
		BeaconQueryTimeout: flagBeaconQueryTimeout,
	}

	beaconCli, err := initBeaconClients(logger, strings.Split(flagBeaconList, ","), m, beaconConfig)
	if err != nil {
		logger.WithError(err).Error("fail to initialize beacon")
		return
	}

	beaconPubCli, err := initBeaconClients(logger, strings.Split(flagBeaconPublishList, ","), m, beaconConfig)
	if err != nil {
		logger.WithError(err).Error("fail to initialize publish beacon")
		return
	}

	// SIM Client
	simFallb := fallback.NewFallback()
	simFallb.AttachMetrics(m)
	if simHttpAddr := flagBlockValidationRPC; simHttpAddr != "" {
		simRPCCli := gethrpc.NewClient(gethSimNamespace, simHttpAddr)
		if err := simRPCCli.Dial(ctx); err != nil {
			logger.WithError(err).WithField("address", simHttpAddr).Error("fail to initialize rpc connection")
			return
		}
		simFallb.AddClient(simRPCCli)
	}

	if simWSAddr := flagBlockValidationEndpointWSList; simWSAddr != "" {
		simWSConn := gethws.NewReConn(logger)
		for _, s := range strings.Split(simWSAddr, ",") {
			input := make(chan []byte, 1000)
			go simWSConn.KeepConnection(s, input)
		}
		simWSCli := gethws.NewClient(simWSConn, gethSimNamespace, flagBlockValidationWSRetry, logger)
		simFallb.AddClient(simWSCli)
	}

	if simHttpAddr := flagBlockValidationEndpointHTTP; simHttpAddr != "" {
		simHTTPCli := gethhttp.NewClient(simHttpAddr, gethSimNamespace, logger)
		simFallb.AddClient(simHTTPCli)
	}

	verificator := verify.NewVerificationManager(logger, uint(flagVerifyQueueSize))
	verificator.RunVerify(uint(flagWorkersVerify))

	// VALIDATOR MANAGEMENT
	var valDS ValidatorStore
	if flagValidatorDatabaseUrl != "" {
		valPG, err := trPostgres.Open(flagValidatorDatabaseUrl, 10, 10, 10*time.Second)
		if err != nil {
			logger.WithError(err).Error("failed to connect to validator database")
			return
		}
		m.RegisterDB(valPG, "registrations")
		valDS = valPostgres.NewDatastore(valPG)
	} else { // by default use existsing storage
		valDS = valBadger.NewDatastore(storage, flagTTL)
	}

	validatorCache, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](flagRegistrationsCacheSize)
	if err != nil {
		logger.WithError(err).Error("fail to initialize validator cache")
		return
	}

	// DATAAPI
	var daDS relay.DataAPIStore
	if flagDataapiDatabaseUrl != "" {
		valPG, err := trPostgres.Open(flagDataapiDatabaseUrl, 10, 10, 10*time.Second) // TODO(l): make configurable
		if err != nil {
			logger.WithError(err).Error("failed to connect to dataapi database")
		}
		m.RegisterDB(valPG, "dataapi")
		daDS = daPostgres.NewDatastore(valPG, 0)
		defer valPG.Close()
	} else { // by default use badger
		daDS = daBadger.NewDatastore(storage, badgerDs.DB, flagTTL)
	}

	// lazyload validators cache, it's optional and we don't care if it errors out
	go preloadValidators(ctx, logger, valDS, validatorCache)

	validatorStoreManager := validators.NewStoreManager(logger, validatorCache, valDS, flagRegistrationsCacheWriteTTL, uint(flagStoreQueueSize))
	validatorStoreManager.AttachMetrics(m)
	if flagWorkersStoreValidator > 0 {
		validatorStoreManager.RunStore(uint(flagWorkersStoreValidator))
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
		RunPayloadAttributesSubscription: flagBeaconPayloadAttributesSubscription,
	})

	auctioneer := auction.NewAuctioneer()

	var allowed map[[48]byte]struct{}
	if flagAllowListedBuilderList != "" {
		allowed = make(map[[48]byte]struct{})
		for _, k := range strings.Split(flagAllowListedBuilderList, ",") {
			var pk types.PublicKey
			if err := pk.UnmarshalText([]byte(k)); err != nil {
				logger.WithError(err).With(log.F{"key": k}).Error("ALLOWED BUILDER NOT ADDED - wrong public key")
				continue
			}
			allowed[pk] = struct{}{}
		}
	}

	skBytes, err := hexutil.Decode(flagSecretKey)
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
		warehouse := wh.NewWarehouse(logger, flagWarehouseBuffer)

		if err := os.MkdirAll(flagWarehouseDir, 0755); err != nil {
			logger.WithError(err).Error("failed to create datadir")
			return
		}

		if err := warehouse.RunParallel(ctx, flagWarehouseDir, flagWarehouseWorkers); err != nil {
			logger.WithError(err).Error("failed to run data exporter")
			return
		}

		warehouse.AttachMetrics(m)

		logger.With(log.F{
			"subService": "warehouse",
			"datadir":    flagWarehouseDir,
			"workers":    flagWarehouseWorkers,
		}).Info("initialized")

		relayWh = warehouse
	}

	payloadCache, err := structs.NewMultiSlotPayloadCache(flagPayloadCacheSize)
	if err != nil {
		logger.WithError(err).Error("fail to initialize stream cache")
		return
	}

	r := relay.NewRelay(logger, relay.RelayConfig{
		BuilderSigningDomain:       domainBuilder,
		GetPayloadResponseDelay:    flagGetPayloadResponseDelay,
		GetPayloadRequestTimeLimit: flagGetPayloadRequestTimeLimit,
		ProposerSigningDomain: map[structs.ForkVersion]types.Domain{
			structs.ForkBellatrix: bellatrixBeaconProposer,
			structs.ForkCapella:   capellaBeaconProposer},
		PubKey:                pk,
		SecretKey:             sk,
		RegistrationCacheTTL:  flagRegistrationsCacheReadTTL,
		TTL:                   flagTTL,
		AllowedListedBuilders: allowed,
		PublishBlock:          flagPublishBlock,
		Distributed:           flagDistribution,
		StreamServedBids:      flagDistributionStreamServedBids,
	}, beaconPubCli, validatorCache, valDS, verificator, state, payloadCache, ds, daDS, auctioneer, simFallb, relayWh, streamer)
	r.AttachMetrics(m)

	if flagDistribution {
		r.RunSubscribersParallel(ctx, uint(flagDistributionStreamWorkers))
	}

	ee := &api.EnabledEndpoints{
		GetHeader:   true,
		GetPayload:  true,
		SubmitBlock: true,
	}
	iApi := inner.NewAPI(ee, ds)

	limitterCache, _ := lru.New[[48]byte, *rate.Limiter](cfg.Api.LimitterCacheSize)
	apiLimitter := api.NewLimitter(cfg.Api.SubmissionLimitRate, cfg.Api.SubmissionLimitBurst, limitterCache, allowed)
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
			Addr:    flagInternalAddr,
			Handler: internalMux,
		}
		if err = internalSrv.ListenAndServe(); err == http.ErrServerClosed {
			err = nil
		}
		return err
	}(m, internalMux)

	mux := http.NewServeMux()
	a.AttachToHandler(mux)

	var srv *http.Server
	// run the http server
	go func(srv *http.Server) (err error) {
		svr := &http.Server{
			Addr:           flagAddr,
			ReadTimeout:    flagTimeout,
			WriteTimeout:   flagTimeout,
			IdleTimeout:    flagTimeout,
			Handler:        mux,
			MaxHeaderBytes: 4096,
		}
		logger.Info("http server listening")
		if err = svr.ListenAndServe(); err == http.ErrServerClosed {
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

func reloadConfigSignal(osSig chan os.Signal, cfg *config.ConfigManager) {
	for range osSig {
		cfg.Reload()
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

func preloadValidators(ctx context.Context, l log.Logger, vs ValidatorStore, vc *lru.Cache[types.PublicKey, structs.ValidatorCacheEntry]) {
	ch := make(chan structs.ValidatorCacheEntry, 100)
	go asyncPopulateAllRegistrations(ctx, l, vs, ch)
	for v := range ch {
		v := v
		vc.ContainsOrAdd(v.Entry.Message.Pubkey, v)
	}
	l.With(log.F{"count": vc.Len()}).Info("Loaded cache validators")
}

func initBeaconClients(l log.Logger, endpoints []string, m *metrics.Metrics, c bcli.BeaconConfig) (*bcli.MultiBeaconClient, error) {
	clients := make([]bcli.BeaconNode, 0, len(endpoints))

	for _, endpoint := range endpoints {
		client, err := bcli.NewBeaconClient(l, endpoint, c)
		if err != nil {
			return nil, err
		}
		client.AttachMetrics(m) // attach metrics
		clients = append(clients, client)
	}
	return bcli.NewMultiBeaconClient(l, clients), nil
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

func initStreamer(ctx context.Context, redisClient *redis.Client, l log.Logger, m *metrics.Metrics, st stream.State) (relay.Streamer, error) {
	timeStreamStart := time.Now()

	pubsub := &redisStream.Pubsub{Redis: redisClient, Logger: l}

	id := flagDistributionID
	if id == "" {
		id = uuid.NewString()
	}

	streamConfig := stream.StreamConfig{
		Logger:          l,
		ID:              id,
		TTL:             flagDistributionStreamTTL,
		PubsubTopic:     flagDistributionStreamTopic,
		StreamQueueSize: flagDistributionStreamQueue,
	}

	redisStreamer := stream.NewClient(pubsub, st, streamConfig)
	redisStreamer.AttachMetrics(m)

	if err := redisStreamer.RunSubscriberParallel(ctx, uint(flagDistributionStreamWorkers)); err != nil {
		return nil, fmt.Errorf("fail to start stream subscriber: %w", err)
	}

	l.With(log.F{
		"relay-service": "stream-subscriber",
		"startTimeMs":   time.Since(timeStreamStart).Milliseconds(),
	}).Info("initialized")

	return redisStreamer, nil
}
