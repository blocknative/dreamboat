package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/metrics"
	pkg "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/blocknative/dreamboat/pkg/auction"
	relay "github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/validators"
	"github.com/blocknative/dreamboat/pkg/verify"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
	lru "github.com/hashicorp/golang-lru/v2"

	blBadger "github.com/blocknative/dreamboat/pkg/datastore/block/badger"
	"github.com/blocknative/dreamboat/pkg/datastore/block/headerscontroller"
	evBadger "github.com/blocknative/dreamboat/pkg/datastore/evidence/badger"
	evPostgres "github.com/blocknative/dreamboat/pkg/datastore/evidence/postgres"
	trBadger "github.com/blocknative/dreamboat/pkg/datastore/transport/badger"
	trPostgres "github.com/blocknative/dreamboat/pkg/datastore/transport/postgres"
	valBadger "github.com/blocknative/dreamboat/pkg/datastore/validator/badger"
	valPostgres "github.com/blocknative/dreamboat/pkg/datastore/validator/postgres"
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
	&cli.BoolFlag{
		Name:    "relay-publish-block",
		Usage:   "flag for publishing payloads to beacon nodes after a delivery",
		Value:   false,
		EnvVars: []string{"RELAY_PUBLISH_BLOCK"},
	},
	&cli.BoolFlag{
		Name:    "relay-distribution",
		Usage:   "run the relay as a distributed application",
		Value:   false,
		EnvVars: []string{"RELAY_DISTRIBUTION"},
	},
}

var (
	config pkg.Config
)

func waitForSignal(cancel context.CancelFunc, osSig chan os.Signal) {
	for range osSig {
		cancel()
		return
	}
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

type BlockDatastore interface {
	PutPayload(context.Context, structs.PayloadKey, *structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockBidAndTrace, bool, error)

	PutHeader(ctx context.Context, hd structs.HeaderData, ttl time.Duration) error
	CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error
	GetMaxProfitHeader(ctx context.Context, slot uint64) (structs.HeaderAndTrace, error)
}

type ValidatorDatastore interface {
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
	PutNewerRegistration(ctx context.Context, pk structs.PubKey, registration types.SignedValidatorRegistration) error
}

type EvidenceDatastore interface {
	CheckSlotDelivered(context.Context, uint64) (bool, error)
	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error

	GetDeliveredPayloads(ctx context.Context, headSlot uint64, queryArgs structs.PayloadTraceQuery) (bts []structs.BidTraceExtended, err error)
	GetBuilderBlockSubmissions(ctx context.Context, headSlot uint64, payload structs.SubmissionTraceQuery) ([]structs.BidTraceWithTimestamp, error)
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		if err := config.Validate(); err != nil {
			return err
		}

		logger := config.Log
		m := metrics.NewMetrics()

		var (
			dsBlock     BlockDatastore
			dsValidator ValidatorDatastore
			dsEvidence  EvidenceDatastore
		)

		// initialize datastores
		timeStoreStart := time.Now()
		if c.Bool("relay-distribution") {
			storage, err := trPostgres.Open("", 0, 0, time.Hour) // TODO: add cli parameters for setting postgreSQL
			if err != nil {
				return fmt.Errorf("fail to create datastore: %w", err)
			}

			// TODO: initialize dsStore
			dsValidator = valPostgres.NewDatastore(storage)
			dsEvidence = evPostgres.NewDatastore(storage, 0) // TODO: add relay-id as parameter
		} else {
			hc := headerscontroller.NewHeaderController(config.RelayHeaderMemorySlotLag, config.RelayHeaderMemorySlotTimeLag)
			hc.AttachMetrics(m)

			storage, err := trBadger.Open(config.Datadir)
			if err != nil {
				return fmt.Errorf("fail to create datastore: %w", err)
			}

			if err = trBadger.InitDatastoreMetrics(m); err != nil {
				return err
			}

			badgerBlock, err := blBadger.NewDatastore(storage, hc, 0) // TODO: set payload cache size
			if err != nil {
				return fmt.Errorf("fail to create block store: %w", err)
			}

			dsValidator = valBadger.NewDatastore(storage, config.TTL)
			dsEvidence = evBadger.NewDatastore(&evBadger.TTLDatastoreBatcher{TTLDatastore: storage}, storage.DB, config.TTL)
			if err = badgerBlock.FixOrphanHeaders(c.Context, config.TTL); err != nil {
				return err
			}

			go badgerBlock.MemoryCleanup(c.Context, config.RelayHeaderMemoryPurgeInterval, config.TTL)
		}

		logger.With(log.F{
			"service":     "datastore",
			"startTimeMs": time.Since(timeStoreStart).Milliseconds(),
		}).Info("data store initialized")

		domainBuilder, err := pkg.ComputeDomain(types.DomainTypeAppBuilder, config.GenesisForkVersion, types.Root{}.String())
		if err != nil {
			return err
		}

		domainBeaconProposer, err := pkg.ComputeDomain(types.DomainTypeBeaconProposer, config.BellatrixForkVersion, config.GenesisValidatorsRoot)
		if err != nil {
			return err
		}

		timeRelayStart := time.Now()

		state := &pkg.AtomicSharedState{}

		beacon, err := initBeacon(c.Context, config)
		if err != nil {
			return fmt.Errorf("fail to initialize beacon: %w", err)
		}
		beacon.AttachMetrics(m)

		verificator := verify.NewVerificationManager(config.Log, c.Uint("relay-verify-queue-size"))
		verificator.RunVerify(c.Uint("relay-workers-verify"))

		blockRelay := relay.NewRelay(config.Log, relay.RelayConfig{
			BuilderSigningDomain:  domainBuilder,
			ProposerSigningDomain: domainBeaconProposer,
			PubKey:                config.PubKey,
			SecretKey:             config.SecretKey,
			TTL:                   config.TTL,
			PublishBlock:          c.Bool("relay-publish-block"),
		}, beacon, verificator, state, dsBlock, dsEvidence, auction.NewAuctioneer())
		blockRelay.AttachMetrics(m)

		cache, err := lru.New[types.PublicKey, validators.CacheEntry](c.Int("relay-registrations-cache-size"))
		if err != nil {
			return fmt.Errorf("fail to initialize validator cache: %w", err)
		}

		validatorStoreManager := validators.NewStoreManager(config.Log, cache, dsValidator, int(math.Floor(config.TTL.Seconds()/2)), c.Uint("relay-store-queue-size"))
		validatorStoreManager.AttachMetrics(m)
		validatorStoreManager.RunStore(c.Uint("relay-workers-store-validator"))
		validatorRelay := validators.NewRegister(config.Log, domainBuilder, state, verificator, validatorStoreManager)

		a := api.NewApi(config.Log, blockRelay, validatorRelay)
		a.AttachMetrics(m)

		logger.With(log.F{
			"service":     "relay",
			"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
		}).Info("initialized")

		cContext, cancel := context.WithCancel(c.Context)

		service := pkg.NewService(config.Log, config, dsValidator, state)
		go func(s *pkg.Service) error {
			config.Log.Info("initialized beacon")
			err := s.RunBeacon(cContext, beacon)
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
