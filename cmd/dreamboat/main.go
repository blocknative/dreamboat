package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"time"

	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/metrics"
	pkg "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/blocknative/dreamboat/pkg/auction"
	"github.com/blocknative/dreamboat/pkg/client/sim/fallback"
	"github.com/blocknative/dreamboat/pkg/client/sim/transport/gethhttp"
	"github.com/blocknative/dreamboat/pkg/client/sim/transport/gethrpc"
	"github.com/blocknative/dreamboat/pkg/client/sim/transport/gethws"
	"github.com/blocknative/dreamboat/pkg/stream"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"

	"github.com/blocknative/dreamboat/pkg/datastore"
	blockRedis "github.com/blocknative/dreamboat/pkg/datastore/block/redis"
	relay "github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/validators"
	"github.com/blocknative/dreamboat/pkg/verify"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"

	trPostgres "github.com/blocknative/dreamboat/pkg/datastore/transport/postgres"
	valPostgres "github.com/blocknative/dreamboat/pkg/datastore/validator/postgres"

	valBadger "github.com/blocknative/dreamboat/pkg/datastore/validator/badger"

	lru "github.com/hashicorp/golang-lru/v2"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

const (
	shutdownTimeout = 5 * time.Second
	version         = pkg.Version
)

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "loglvl",
		Usage:   "logging level: trace, debug, info, warn, error or fatal",
		Value:   "info",
		EnvVars: []string{"LOGLVL"},
	},
	&cli.StringFlag{
		Name:    "logfmt",
		Usage:   "format logs as text, json or none",
		Value:   "text",
		EnvVars: []string{"LOGFMT"},
	},
	&cli.BoolFlag{
		Name:  "profile",
		Usage: "activates profiling http endpoint",
		Value: false,
	},
	&cli.StringFlag{
		Name:    "addr",
		Usage:   "server listen address",
		Value:   "localhost:18550",
		EnvVars: []string{"RELAY_ADDR"},
	},
	&cli.StringFlag{
		Name:    "internalAddr",
		Usage:   "server listen address",
		Value:   "0.0.0.0:19550",
		EnvVars: []string{"RELAY_INTERNAL_ADDR"},
	},
	&cli.DurationFlag{
		Name:    "timeout",
		Usage:   "request timeout",
		Value:   time.Second * 2,
		EnvVars: []string{"RELAY_TIMEOUT"},
	},
	&cli.StringSliceFlag{
		Name:    "beacon",
		Usage:   "`url` for beacon endpoint",
		EnvVars: []string{"RELAY_BEACON"},
	},
	&cli.BoolFlag{
		Name:    "check-builders",
		Usage:   "check builder blocks",
		EnvVars: []string{"RELAY_CHECK_BUILDERS"},
	},
	&cli.StringSliceFlag{
		Name:    "builder",
		Usage:   "`url` formatted as schema://pubkey@host",
		EnvVars: []string{"BN_RELAY_BUILDER_URLS"},
	},
	&cli.StringFlag{
		Name:    "network",
		Usage:   "the networks the relay works on",
		Value:   "mainnet",
		EnvVars: []string{"RELAY_NETWORK"},
	},
	&cli.StringFlag{
		Name:     "secretKey",
		Usage:    "secret key used to sign messages",
		Required: true,
		EnvVars:  []string{"RELAY_SECRET_KEY"},
	},
	&cli.StringFlag{
		Name:    "datadir",
		Usage:   "data directory where blocks and validators are stored in the default datastore implementation",
		Value:   "/tmp/relay",
		EnvVars: []string{"RELAY_DATADIR"},
	},
	&cli.DurationFlag{
		Name:    "ttl",
		Usage:   "ttl of the data",
		Value:   24 * time.Hour,
		EnvVars: []string{"BN_RELAY_TTL"},
	},
	&cli.Uint64Flag{
		Name:    "relay-validator-queue-size",
		Usage:   "The size of response queue, should be set to expected number of validators in one request",
		Value:   100_000,
		EnvVars: []string{"RELAY_QUEUE_REQ"},
	},
	&cli.Uint64Flag{
		Name:    "relay-workers-verify",
		Usage:   "number of workers running verify in parallel",
		Value:   2000,
		EnvVars: []string{"RELAY_WORKERS_VERIFY"},
	},
	&cli.Uint64Flag{
		Name:    "relay-workers-store-validator",
		Usage:   "number of workers storing validators in parallel",
		Value:   400,
		EnvVars: []string{"RELAY_WORKERS_STORE_VALIDATOR"},
	},
	&cli.Uint64Flag{
		Name:    "relay-verify-queue-size",
		Usage:   "size of verify queue",
		Value:   100_000,
		EnvVars: []string{"RELAY_VERIFY_QUEUE_SIZE"},
	},
	&cli.Uint64Flag{
		Name:    "relay-store-queue-size",
		Usage:   "size of store queue",
		Value:   100_000,
		EnvVars: []string{"RELAY_STORE_QUEUE_SIZE"},
	},
	&cli.Uint64Flag{
		Name:    "relay-header-memory-slot-lag",
		Usage:   "how many slots from the head relay should keep in memory",
		Value:   200,
		EnvVars: []string{"RELAY_HEADER_MEMORY_SLOT_LAG"},
	},
	&cli.DurationFlag{
		Name:    "relay-header-memory-slot-time-lag",
		Usage:   "how log should it take for lagged slot to be eligible fot purge",
		Value:   time.Minute * 5,
		EnvVars: []string{"RELAY_HEADER_MEMORY_SLOT_TIME_LAG"},
	},
	&cli.DurationFlag{
		Name:    "relay-header-memory-purge-interval",
		Usage:   "how often memory should be purged",
		Value:   time.Minute * 10,
		EnvVars: []string{"RELAY_HEADER_MEMORY_PURGE_INTERVAL"},
	},
	&cli.IntFlag{
		Name:    "relay-payload-cache-size",
		Usage:   "number of payloads to cache for fast in-memory reads",
		Value:   1_000,
		EnvVars: []string{"RELAY_PAYLOAD_CACHE_SIZE"},
	},
	&cli.IntFlag{
		Name:    "relay-registrations-cache-size",
		Usage:   "relay registrations cache size",
		Value:   600_000,
		EnvVars: []string{"RELAY_REGISTRATIONS_CACHE_SIZE"},
	},
	&cli.DurationFlag{
		Name:    "relay-registrations-cache-ttl",
		Usage:   "registrations cache ttl",
		Value:   time.Hour,
		EnvVars: []string{"RELAY_REGISTRATIONS_CACHE_TTL"},
	},
	&cli.BoolFlag{
		Name:    "relay-publish-block",
		Usage:   "flag for publishing payloads to beacon nodes after a delivery",
		Value:   false,
		EnvVars: []string{"RELAY_PUBLISH_BLOCK"},
	},
	&cli.StringFlag{
		Name:    "relay-validator-database-url",
		Usage:   "address of postgress database for validator registrations, if empty - default, badger will be used",
		Value:   "",
		EnvVars: []string{"RELAY_VALIDATOR_DATABASE_URL"},
	},
	&cli.BoolFlag{
		Name:    "relay-fast-boot",
		Usage:   "speed up booting up of relay, adding temporary inconsistency on the builder_blocks_received endpoint",
		Value:   false,
		EnvVars: []string{"RELAY_FAST_BOOT"},
	},
	&cli.StringFlag{
		Name:    "relay-allow-listed-builder",
		Usage:   "comma separated list of allowed builder pubkeys",
		Value:   "",
		EnvVars: []string{"RELAY_ALLOW_LISTED_BUILDER"},
	},
	&cli.IntFlag{
		Name:    "relay-submission-limit-rate",
		Usage:   "submission request limit - rate per second",
		Value:   2,
		EnvVars: []string{"RELAY_SUBMISSION_LIMIT_RATE"},
	},
	&cli.IntFlag{
		Name:    "relay-submission-limit-burst",
		Usage:   "submission request limit - burst",
		Value:   2,
		EnvVars: []string{"RELAY_SUBMISSION_LIMIT_BURST"},
	},
	&cli.StringFlag{
		Name:    "block-validation-endpoint-http",
		Usage:   "http block validation endpoint address",
		Value:   "",
		EnvVars: []string{"BLOCK_VALIDATION_ENDPOINT_HTTP"},
	},
	&cli.StringFlag{
		Name:    "block-validation-endpoint-ws",
		Usage:   "ws block validation endpoint address (comma separated list)",
		Value:   "",
		EnvVars: []string{"BLOCK_VALIDATION_ENDPOINT_WS"},
	},
	&cli.StringFlag{
		Name:    "block-validation-endpoint-rpc",
		Usage:   "rpc block validation rawurl (eg. ipc path)",
		Value:   "",
		EnvVars: []string{"BLOCK_VALIDATION_ENDPOINT_RPC"},
	},

	// streaming layer flags
	&cli.BoolFlag{
		Name:    "relay-distribution",
		Usage:   "run relay as a distributed system with multiple replicas",
		Value:   false,
		EnvVars: []string{"RELAY_DISTRIBUTION"},
	},
	&cli.StringFlag{
		Name:    "relay-distribution-id",
		Usage:   "the id of the relay to differentiate from other replicas",
		Value:   "",
		EnvVars: []string{"RELAY_DISTRIBUTION_ID"},
	},
	&cli.UintFlag{
		Name:    "relay-distribution-stream-workers",
		Usage:   "number of workers publishing and processing subscriptions in the stream",
		Value:   100,
		EnvVars: []string{"RELAY_DISTRIBUTION_STREAM_WORKERS"},
	},
	&cli.BoolFlag{
		Name:    "relay-distribution-publish-submissions",
		Usage:   "publish all submitted blocks into pubsub. If false, only blocks returned in GetHeader are published",
		Value:   false,
		EnvVars: []string{"RELAY_DISTRIBUTION_PUBLISH_SUBMISSIONS"},
	},
	&cli.DurationFlag{
		Name:    "relay-distribution-ttl",
		Usage:   "TTL of the data that is distributed",
		Value:   time.Minute,
		EnvVars: []string{"RELAY_DISTRIBUTION_TTL"},
	},
	&cli.StringFlag{
		Name:    "relay-distribution-pubsub-topic",
		Usage:   "Pubsub topic for streaming payloads",
		Value:   "relay/payload",
		EnvVars: []string{"RELAY_DISTRIBUTION_PUBSUB_TOPIC"},
	},
	&cli.IntFlag{
		Name:    "relay-distribution-publish-queue",
		Usage:   "Pubsub publish queue size",
		Value:   100,
		EnvVars: []string{"RELAY_DISTRIBUTION_PUBLISH_QUEUE"},
	},
	&cli.StringFlag{
		Name:    "relay-distribution-redis-uri",
		Usage:   "Redis URI",
		EnvVars: []string{"RELAY_DISTRIBUTION_REDIS_URI"},
	},
}

const (
	gethSimNamespace = "flashbots"
)

var (
	config pkg.Config
)

type Datastore interface {
	relay.Datastore
	relay.BlockDatastore
	FixOrphanHeaders(context.Context, time.Duration) error
	MemoryCleanup(context.Context, time.Duration, time.Duration) error
}

// Main starts the relay
func main() {
	app := &cli.App{
		Name:    "dreamboat",
		Usage:   "ethereum 2.0 relay, commissioned and put to sea by Blocknative",
		Version: version,
		Flags:   flags,
		Before:  setup(),
		Action:  run(),
	}

	osSig := make(chan os.Signal, 2)

	signal.Notify(osSig, syscall.SIGTERM)
	signal.Notify(osSig, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())

	go waitForSignal(cancel, osSig)
	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Fatal(err)
	}

}

func setup() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		skBytes, err := hexutil.Decode(c.String("secretKey"))
		if err != nil {
			return err
		}
		sk, pk, err := blstools.SecretKeyFromBytes(skBytes)
		if err != nil {
			return err
		}

		config = pkg.Config{
			Log:                      logger(c),
			RelayQueueProcessingSize: c.Uint64("relay-validator-queue-size"),

			RelayHeaderMemorySlotLag:       c.Uint64("relay-header-memory-slot-lag"),
			RelayHeaderMemorySlotTimeLag:   c.Duration("relay-header-memory-slot-time-lag"),
			RelayHeaderMemoryPurgeInterval: c.Duration("relay-header-memory-purge-interval"),

			RelayRequestTimeout: c.Duration("timeout"),
			Network:             c.String("network"),
			BuilderCheck:        c.Bool("check-builder"),
			BuilderURLs:         c.StringSlice("builder"),
			BeaconEndpoints:     c.StringSlice("beacon"),
			PubKey:              pk,
			SecretKey:           sk,
			Datadir:             c.String("datadir"),
			TTL:                 c.Duration("ttl"),
		}

		return
	}
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		cContext, cancel := context.WithCancel(c.Context)

		if err := config.Validate(); err != nil {
			return err
		}

		logger := config.Log.WithField("fast-boot", c.Bool("relay-fast-boot"))

		domainBuilder, err := pkg.ComputeDomain(types.DomainTypeAppBuilder, config.GenesisForkVersion, types.Root{}.String())
		if err != nil {
			return err
		}

		domainBeaconProposer, err := pkg.ComputeDomain(types.DomainTypeBeaconProposer, config.BellatrixForkVersion, config.GenesisValidatorsRoot)
		if err != nil {
			return err
		}

		timeDataStoreStart := time.Now()
		m := metrics.NewMetrics()

		var (
			ds       Datastore
			bds      relay.BlockDatastore
			streamer relay.Streamer
		)

		storage, err := badger.NewDatastore(config.Datadir, &badger.DefaultOptions)
		if err != nil {
			logger.WithError(err).Error("failed to initialize datastore")
			return err
		}

		logger.With(log.F{
			"service":     "datastore",
			"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
		}).Info("data store initialized")

		timeRelayStart := time.Now()
		state := &pkg.AtomicState{}

		hc := datastore.NewHeaderController(config.RelayHeaderMemorySlotLag, config.RelayHeaderMemorySlotTimeLag)
		hc.AttachMetrics(m)

		ds, err = datastore.NewDatastore(&datastore.TTLDatastoreBatcher{TTLDatastore: storage}, storage.DB, hc, c.Int("relay-payload-cache-size"))
		if err != nil {
			return fmt.Errorf("fail to create datastore: %w", err)
		}
		if err = datastore.InitDatastoreMetrics(m); err != nil {
			return err
		}
		bds = ds

		errCh := make(chan error, 1)

		go func(ctx context.Context, l log.Logger) {
			if err := ds.FixOrphanHeaders(ctx, config.TTL); err != nil {
				l.WithError(err).Error("fail to fix orphan headers")
			} else {
				l.Info("fixed orphan headers")
			}
			errCh <- err
		}(c.Context, config.Log)

		if !c.Bool("relay-fast-boot") {
			if err := <-errCh; err != nil {
				return fmt.Errorf("fail to fix orphan headers: %w", err)
			}
		}

		if c.Bool("relay-distribution") {
			timeStreamStart := time.Now()
			redisClient := redis.NewClient(&redis.Options{
				Addr: c.String("relay-distribution-redis-uri"),
			})

			// init datastore
			redisDatastore := &blockRedis.RedisDatastore{Redis: redisClient}
			ldDatastore := datastore.NewLocalRemoteDatastore(ds, redisDatastore, config.Log)
			ldDatastore.AttachMetrics(m)

			bds = ldDatastore

			// init streamer
			pubsub := &stream.RedisPubsub{Redis: redisClient, Logger: config.Log}

			id := c.String("relay-distribution-id")
			if id == "" {
				id = uuid.NewString()
			}

			streamConfig := stream.StreamConfig{
				Logger:          config.Log,
				ID:              id,
				TTL:             c.Duration("relay-distribution-ttl"),
				PubsubTopic:     c.String("relay-distribution-pubsub-topic"),
				StreamQueueSize: c.Int("relay-distribution-publish-queue"),
			}

			redisStreamer := stream.NewRedisStream(pubsub, streamConfig)
			redisStreamer.AttachMetrics(m)

			streamer = redisStreamer

			redisStreamer.RunPublisherParallel(cContext, c.Uint("relay-distribution-stream-workers"))
			config.Log.With(log.F{
				"relay-service": "stream-publisher",
				"startTimeMs":   time.Since(timeStreamStart).Milliseconds(),
			}).Info("initialized")

			if err := redisStreamer.RunSubscriberParallel(cContext, ds, c.Uint("relay-distribution-stream-workers")); err != nil {
				return fmt.Errorf("fail to start stream subscriber: %w", err)
			}
			config.Log.With(log.F{
				"relay-service": "stream-subscriber",
				"startTimeMs":   time.Since(timeStreamStart).Milliseconds(),
			}).Info("initialized")
		}

		go ds.MemoryCleanup(c.Context, config.RelayHeaderMemoryPurgeInterval, config.TTL)

		beacon, err := initBeacon(c.Context, config)
		if err != nil {
			return fmt.Errorf("fail to initialize beacon: %w", err)
		}
		beacon.AttachMetrics(m)

		// SIM Client
		simFallb := fallback.NewFallback()
		simFallb.AttachMetrics(m)
		if simHttpAddr := c.String("block-validation-endpoint-rpc"); simHttpAddr != "" {
			simRPCCli := gethrpc.NewClient(gethSimNamespace, simHttpAddr)
			if err := simRPCCli.Dial(c.Context); err != nil {
				return fmt.Errorf("fail to initialize rpc connection (%s): %w", simHttpAddr, err)
			}
			simFallb.AddClient(simRPCCli)
		}

		if simWSAddr := c.String("block-validation-endpoint-ws"); simWSAddr != "" {
			simWSConn := gethws.NewReConn(logger)
			for _, s := range strings.Split(simWSAddr, ",") {
				input := make(chan []byte, 1000)
				go simWSConn.KeepConnection(s, input)
			}
			simWSCli := gethws.NewClient(simWSConn, gethSimNamespace, logger)
			simFallb.AddClient(simWSCli)
		}

		if simHttpAddr := c.String("block-validation-endpoint-http"); simHttpAddr != "" {
			simHTTPCli := gethhttp.NewClient(simHttpAddr, gethSimNamespace, logger)
			simFallb.AddClient(simHTTPCli)
		}

		verificator := verify.NewVerificationManager(config.Log, c.Uint("relay-verify-queue-size"))
		verificator.RunVerify(c.Uint("relay-workers-verify"))

		dbURL := c.String("relay-validator-database-url")
		// VALIDATOR MANAGEMENT
		var valDS ValidatorStore
		if dbURL != "" {
			valPG, err := trPostgres.Open(dbURL, 10, 10, 10) // TODO(l): make configurable
			if err != nil {
				return fmt.Errorf("failed to connect to the database: %w", err)
			}
			m.RegisterDB(valPG, "registrations")
			valDS = valPostgres.NewDatastore(valPG)
		} else { // by default use existsing storage
			valDS = valBadger.NewDatastore(storage, config.TTL)
		}

		validatorCache, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](c.Int("relay-registrations-cache-size"))
		if err != nil {
			return fmt.Errorf("fail to initialize validator cache: %w", err)
		}

		// lazyload validators cache, it's optional and we don't care if it errors out
		go preloadValidators(c.Context, logger, valDS, validatorCache)

		validatorStoreManager := validators.NewStoreManager(config.Log, validatorCache, valDS, int(math.Floor(config.TTL.Seconds()/2)), c.Uint("relay-store-queue-size"))
		validatorStoreManager.AttachMetrics(m)
		validatorStoreManager.RunStore(c.Uint("relay-workers-store-validator"))

		validatorRelay := validators.NewRegister(config.Log, domainBuilder, state, verificator, validatorStoreManager)
		validatorRelay.AttachMetrics(m)
		service := pkg.NewService(config.Log, config, state)

		auctioneer := auction.NewAuctioneer()

		var allowed map[[48]byte]struct{}
		albString := c.String("relay-allow-listed-builder")
		if albString != "" {
			allowed = make(map[[48]byte]struct{})
			for _, k := range strings.Split(albString, ",") {
				var pk types.PublicKey
				if err := pk.UnmarshalText([]byte(k)); err != nil {
					config.Log.WithError(err).With(log.F{"key": k}).Error("ALLOWED BUILDER NOT ADDED - wrong public key")
					continue
				}
				allowed[pk] = struct{}{}
			}
		}

		r := relay.NewRelay(config.Log, relay.RelayConfig{
			BuilderSigningDomain:  domainBuilder,
			ProposerSigningDomain: domainBeaconProposer,
			PubKey:                config.PubKey,
			SecretKey:             config.SecretKey,
			RegistrationCacheTTL:  c.Duration("relay-registrations-cache-ttl"),
			TTL:                   config.TTL,
			AllowedListedBuilders: allowed,
			PublishBlock:          c.Bool("relay-publish-block"),
			Distributed:           c.Bool("relay-distribution"),
			StreamSubmissions:     c.Bool("relay-distribution-publish-submissions"),
		}, beacon, validatorCache, valDS, verificator, state, ds, bds, auctioneer, simFallb, streamer)
		r.AttachMetrics(m)

		go func(s *pkg.Service) error {
			err := r.RunSlotDeliveredUpdater(cContext)
			if err != nil {
				cancel()
			}
			return err
		}(service)

		a := api.NewApi(config.Log, r, validatorRelay, api.NewLimitter(c.Int("relay-submission-limit-rate"), c.Int("relay-submission-limit-burst"), allowed))
		a.AttachMetrics(m)
		logger.With(log.F{
			"service":     "relay",
			"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
		}).Info("initialized")

		go func(s *pkg.Service) error {
			config.Log.Info("initialized beacon")
			err := s.RunBeacon(cContext, beacon, validatorStoreManager, validatorCache)
			if err != nil {
				cancel()
			}
			return err
		}(service)

		// run internal http server
		go func(m *metrics.Metrics) (err error) {
			internalMux := http.NewServeMux()
			metrics.AttachProfiler(internalMux)

			internalMux.Handle("/metrics", m.Handler())
			logger.Info("internal server listening")
			internalSrv := http.Server{
				Addr:    c.String("internalAddr"),
				Handler: internalMux,
			}

			if err = internalSrv.ListenAndServe(); err == http.ErrServerClosed {
				err = nil
			}
			return err
		}(m)
		// wait for the relay service to be ready
		select {
		case <-cContext.Done():
			return err
		case <-service.Ready():
		}

		logger.Info("relay service ready")

		mux := http.NewServeMux()
		a.AttachToHandler(mux)

		var srv http.Server
		// run the http server
		go func(srv http.Server) (err error) {
			svr := http.Server{
				Addr:           c.String("addr"),
				ReadTimeout:    c.Duration("timeout"),
				WriteTimeout:   c.Duration("timeout"),
				IdleTimeout:    time.Second * 2,
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

		<-cContext.Done()

		ctx, closeC := context.WithTimeout(context.Background(), shutdownTimeout)
		defer closeC()
		logger.Info("Shutdown initialized")
		err = srv.Shutdown(ctx)
		logger.Info("Shutdown returned ", err)

		ctx, closeC = context.WithTimeout(context.Background(), shutdownTimeout/2)
		defer closeC()
		finish := make(chan struct{})
		go closemanager(ctx, finish, validatorStoreManager)

		select {
		case <-finish:
		case <-ctx.Done():
			logger.Warn("Closing manager deadline exceeded ")
		}

		return nil
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

func initBeacon(ctx context.Context, config pkg.Config) (pkg.BeaconClient, error) {
	clients := make([]pkg.BeaconClient, 0, len(config.BeaconEndpoints))

	for _, endpoint := range config.BeaconEndpoints {
		client, err := pkg.NewBeaconClient(endpoint, config)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return pkg.NewMultiBeaconClient(config.Log, clients), nil
}

func closemanager(ctx context.Context, finish chan struct{}, regMgr *validators.StoreManager) {
	regMgr.Close(ctx)
	finish <- struct{}{}
}

func logger(c *cli.Context) log.Logger {
	return log.New(
		withLevel(c),
		withFormat(c),
		withErrWriter(c))
}

func withLevel(c *cli.Context) (opt log.Option) {
	var level = log.FatalLevel
	defer func() {
		opt = log.WithLevel(level)
	}()

	if c.Bool("trace") {
		level = log.TraceLevel
		return
	}

	if c.String("logfmt") == "none" {
		return
	}

	switch c.String("loglvl") {
	case "trace", "t":
		level = log.TraceLevel
	case "debug", "d":
		level = log.DebugLevel
	case "info", "i":
		level = log.InfoLevel
	case "warn", "warning", "w":
		level = log.WarnLevel
	case "error", "err", "e":
		level = log.ErrorLevel
	case "fatal", "f":
		level = log.FatalLevel
	default:
		level = log.InfoLevel
	}

	return
}

func withFormat(c *cli.Context) log.Option {
	var fmt logrus.Formatter

	switch c.String("logfmt") {
	case "none":
	case "json":
		fmt = &logrus.JSONFormatter{
			PrettyPrint:     c.Bool("prettyprint"),
			TimestampFormat: time.RFC3339Nano,
		}
	default:
		fmt = new(logrus.TextFormatter)
	}
	return log.WithFormatter(fmt)
}

func withErrWriter(c *cli.Context) log.Option {
	return log.WithWriter(c.App.ErrWriter)
}
