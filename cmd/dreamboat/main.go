package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/blocknative/dreamboat/metrics"
	pkg "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/blocknative/dreamboat/pkg/auction"
	"github.com/blocknative/dreamboat/pkg/datastore"
	relay "github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/stream"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/go-redis/redis"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
	blst "github.com/supranational/blst/bindings/go"
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
	&cli.BoolFlag{
		Name:    "relay-distribution",
		Usage:   "run relay as a distributed system with multiple replicas",
		EnvVars: []string{"RELAY_DISTRIBUTION"},
	},
	&cli.DurationFlag{
		Name:    "relay-distribution-ttl",
		Usage:   "TTL of the data that is distributed",
		Value:   time.Hour,
		EnvVars: []string{"RELAY_DISTRIBUTION_TTL"},
	},
	&cli.StringFlag{
		Name:    "relay-distribution-pubsub-topic",
		Usage:   "Pubsub topic for streaming payloads",
		EnvVars: []string{"RELAY_DISTRIBUTION_PUBSUB_TOPIC"},
	},
	&cli.StringFlag{
		Name:    "relay-distribution-redis-uri",
		Usage:   "Redis URI",
		EnvVars: []string{"RELAY_DISTRIBUTION_REDIS_URI"},
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
		sk, pk, err := setupKeys(c)
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

func setupKeys(c *cli.Context) (*blst.SecretKey, types.PublicKey, error) {
	skBytes, err := hexutil.Decode(c.String("secretKey"))
	if err != nil {
		return nil, types.PublicKey{}, err
	}
	sk, err := bls.SecretKeyFromBytes(skBytes[:])
	if err != nil {
		return nil, types.PublicKey{}, err
	}

	var pk types.PublicKey
	err = pk.FromSlice(bls.PublicKeyFromSecretKey(sk).Compress())
	return sk, pk, err
}

type Datastore interface {
	pkg.Datastore
	relay.Datastore
	FixOrphanHeaders(context.Context, time.Duration) error
	MemoryCleanup(context.Context, time.Duration, time.Duration) error
	GetAllRegistration() (map[string]types.SignedValidatorRegistration, error)
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		cContext, cancel := context.WithCancel(c.Context)

		if err := config.Validate(); err != nil {
			return err
		}

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

		storage, err := badger.NewDatastore(config.Datadir, &badger.DefaultOptions)
		if err != nil {
			config.Log.WithError(err).Error("failed to initialize datastore")
			return err
		}
		config.Log.With(log.F{
			"service":     "datastore",
			"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
		}).Info("data store initialized")

		timeRelayStart := time.Now()
		as := &pkg.AtomicState{}

		hc := datastore.NewHeaderController(config.RelayHeaderMemorySlotLag, config.RelayHeaderMemorySlotTimeLag)
		hc.AttachMetrics(m)

		var ds Datastore
		badgerDs, err := datastore.NewDatastore(&datastore.TTLDatastoreBatcher{storage}, storage.DB, hc, c.Int("relay-payload-cache-size")) // TODO: make cache size parameter
		if err != nil {
			return fmt.Errorf("fail to create datastore: %w", err)
		}
		ds = badgerDs

		if c.Bool("relay-distribution") {
			redisClient := redis.NewClient(&redis.Options{
				Addr: c.String("relay-distribution-redis-uri"),
			})

			remoteDatastore := &stream.RedisDatastore{redisClient}
			pubsub := &stream.RedisPubsub{redisClient}
			streamConfig := stream.StreamConfig{
				PubsubTopic: c.String("relay-distribution-pubsub-topic"),
				TTL:         c.Duration("relay-distribution-ttl"),
			}

			streamDs := &stream.StreamDatastore{
				Datastore:       badgerDs,
				RemoteDatastore: remoteDatastore,
				Pubsub:          pubsub,
				Config:          streamConfig,
			}

			go func(s *stream.StreamDatastore) error {
				err := s.Run(cContext, config.Log)
				if err != nil {
					cancel()
				}
				return err
			}(streamDs)
		}

		if err = datastore.InitDatastoreMetrics(m); err != nil {
			return err
		}

		if err = ds.FixOrphanHeaders(c.Context, config.TTL); err != nil {
			return err
		}

		go ds.MemoryCleanup(c.Context, config.RelayHeaderMemoryPurgeInterval, config.TTL)

		regMgr := relay.NewProcessManager(config.Log, c.Uint("relay-verify-queue-size"), c.Uint("relay-store-queue-size"))
		regMgr.AttachMetrics(m)
		loadRegistrations(ds, regMgr)

		go regMgr.RunCleanup(uint64(config.TTL), time.Hour)

		auctioneer := auction.NewAuctioneer()
		r := relay.NewRelay(config.Log, relay.RelayConfig{
			BuilderSigningDomain:  domainBuilder,
			ProposerSigningDomain: domainBeaconProposer,
			PubKey:                config.PubKey,
			SecretKey:             config.SecretKey,
			TTL:                   config.TTL,
		}, as, ds, regMgr, auctioneer)
		r.AttachMetrics(m)

		service := pkg.NewService(config.Log, config, ds, r, as)
		service.AttachMetrics(m)

		api := api.NewApi(config.Log, service)
		api.AttachMetrics(m)

		regMgr.RunStore(ds, config.TTL, c.Uint("relay-workers-store-validator"))
		regMgr.RunVerify(c.Uint("relay-workers-verify"))

		config.Log.With(log.F{
			"service":     "relay",
			"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
		}).Info("initialized")

		go func(s *pkg.Service) error {
			err := s.RunBeacon(cContext)
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
			config.Log.Info("internal server listening")
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

		config.Log.Debug("relay service ready")

		mux := http.NewServeMux()
		api.AttachToHandler(mux)

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
			config.Log.Info("http server listening")
			if err = svr.ListenAndServe(); err == http.ErrServerClosed {
				err = nil
			}
			config.Log.Info("http server finished")
			return err
		}(srv)

		<-cContext.Done()

		ctx, _ := context.WithTimeout(context.Background(), shutdownTimeout)
		log.Info("Shutdown initialized")
		err = srv.Shutdown(ctx)
		log.Info("Shutdown returned ", err)

		ctx, _ = context.WithTimeout(context.Background(), shutdownTimeout/2)
		finish := make(chan struct{})
		go closemanager(ctx, finish, regMgr)

		select {
		case <-finish:
		case <-ctx.Done():
			log.Warn("Closing manager deadline exceeded ")
		}
		return fmt.Errorf("properly exiting... %w", err) // this surprisingly has to return error

	}
}

func closemanager(ctx context.Context, finish chan struct{}, regMgr *relay.ProcessManager) {
	regMgr.Close(ctx)
	finish <- struct{}{}
}

func loadRegistrations(ds Datastore, regMgr *relay.ProcessManager) {
	reg, err := ds.GetAllRegistration()
	if err == nil {
		for k, v := range reg {
			regMgr.Set(k, v.Message.Timestamp)
		}

		config.Log.With(log.F{
			"service":        "registration",
			"count-elements": len(reg),
		}).Info("registrations loaded")
	}

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
