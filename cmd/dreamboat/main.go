package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"time"

	"github.com/blocknative/dreamboat/api"
	"github.com/blocknative/dreamboat/auction"
	"github.com/blocknative/dreamboat/beacon"
	bcli "github.com/blocknative/dreamboat/beacon/client"
	"github.com/blocknative/dreamboat/blstools"
	"github.com/blocknative/dreamboat/client/sim/fallback"
	"github.com/blocknative/dreamboat/client/sim/transport/gethhttp"
	"github.com/blocknative/dreamboat/client/sim/transport/gethrpc"
	"github.com/blocknative/dreamboat/client/sim/transport/gethws"
	"github.com/blocknative/dreamboat/data"
	"github.com/blocknative/dreamboat/metrics"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/datastore"
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
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

const (
	shutdownTimeout = 15 * time.Second
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
	&cli.StringSliceFlag{ // TODO: Remove
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
	&cli.StringFlag{
		Name:    "relay-dataapi-database-url",
		Usage:   "address of postgress database for dataapi, if empty - default, badger will be used",
		Value:   "",
		EnvVars: []string{"RELAY_DATAAPI_DATABASE_URL"},
	},
	&cli.BoolFlag{ // TODO: Remove
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
	&cli.BoolFlag{
		Name:    "block-validation-ws-retry",
		Usage:   "retry to other connection on failure",
		Value:   false,
		EnvVars: []string{"BLOCK_VALIDATION_WS_RETRY"},
	},
	&cli.StringFlag{
		Name:    "block-validation-endpoint-rpc",
		Usage:   "rpc block validation rawurl (eg. ipc path)",
		Value:   "",
		EnvVars: []string{"BLOCK_VALIDATION_ENDPOINT_RPC"},
	},
	&cli.DurationFlag{
		Name:    "block-publication-delay",
		Usage:   "Delay between lock publication and returning request to validator",
		Value:   time.Second,
		EnvVars: []string{"BLOCK_PUBLICATION_DELAY"},
	},

	// data export flags
	&cli.BoolFlag{
		Name:    "data-export",
		Usage:   "Export data from the relay to the filesystem",
		Value:   false,
		EnvVars: []string{"DATA_EXPORT"},
	},
	&cli.StringFlag{
		Name:    "data-export-dir",
		Usage:   "Data directory where the data is exported",
		Value:   "/data/relay/export",
		EnvVars: []string{"DATA_EXPORT_DIR"},
	},
	&cli.IntFlag{
		Name:    "data-export-workers",
		Usage:   "Number of workers for exporting the data",
		Value:   32,
		EnvVars: []string{"DATA_EXPORT_WORKERS"},
	},
}

const (
	gethSimNamespace = "flashbots"
)

// Main starts the relay
func main() {
	app := &cli.App{
		Name:    "dreamboat",
		Usage:   "ethereum 2.0 relay, commissioned and put to sea by Blocknative",
		Version: "0.4.2",
		Flags:   flags,
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

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		cfg := config.NewConfig()
		cfg.LoadNetwork(c.String("network"))
		if cfg.GenesisForkVersion == "" {
			if err := cfg.ReadNetworkConfig(c.String("datadir"), c.String("network")); err != nil {
				return err
			}
		}

		TTL := c.Duration("ttl")
		logger := logger(c)

		timeDataStoreStart := time.Now()
		m := metrics.NewMetrics()

		storage, err := trBadger.Open(c.String("datadir"))
		if err != nil {
			logger.WithError(err).Error("failed to initialize datastore")
			return err
		}

		if err = trBadger.InitDatastoreMetrics(m); err != nil {
			logger.WithError(err).Error("failed to initialize datastore metrics")
			return err
		}

		logger.With(log.F{
			"service":     "datastore",
			"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
		}).Info("data store initialized")

		timeRelayStart := time.Now()
		state := &beacon.MultiSlotState{}
		ds, err := datastore.NewDatastore(storage, c.Int("relay-payload-cache-size"))
		if err != nil {
			return fmt.Errorf("fail to create datastore: %w", err)
		}

		beaconCli, err := initBeaconClients(c.Context, logger, c.StringSlice("beacon"), m)
		if err != nil {
			return fmt.Errorf("fail to initialize beacon: %w", err)
		}

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
			simWSCli := gethws.NewClient(simWSConn, gethSimNamespace, c.Bool("block-validation-ws-retry"), logger)
			simFallb.AddClient(simWSCli)
		}

		if simHttpAddr := c.String("block-validation-endpoint-http"); simHttpAddr != "" {
			simHTTPCli := gethhttp.NewClient(simHttpAddr, gethSimNamespace, logger)
			simFallb.AddClient(simHTTPCli)
		}

		verificator := verify.NewVerificationManager(logger, c.Uint("relay-verify-queue-size"))
		verificator.RunVerify(c.Uint("relay-workers-verify"))

		dbURL := c.String("relay-validator-database-url")
		// VALIDATOR MANAGEMENT
		var valDS ValidatorStore
		if dbURL != "" {
			valPG, err := trPostgres.Open(dbURL, 10, 10, 10)
			if err != nil {
				return fmt.Errorf("failed to connect to the database: %w", err)
			}
			m.RegisterDB(valPG, "registrations")
			valDS = valPostgres.NewDatastore(valPG)
		} else { // by default use existsing storage
			valDS = valBadger.NewDatastore(storage, TTL)
		}

		validatorCache, err := lru.New[types.PublicKey, structs.ValidatorCacheEntry](c.Int("relay-registrations-cache-size"))
		if err != nil {
			return fmt.Errorf("fail to initialize validator cache: %w", err)
		}

		dbdApiURL := c.String("relay-dataapi-database-url")
		// DATAAPI
		var daDS relay.DataAPIStore
		if dbdApiURL != "" {
			valPG, err := trPostgres.Open(dbdApiURL, 10, 10, 10) // TODO(l): make configurable
			if err != nil {
				return fmt.Errorf("failed to connect to the database: %w", err)
			}
			m.RegisterDB(valPG, "dataapi")
			daDS = daPostgres.NewDatastore(valPG, 0)
			defer valPG.Close()
		} else { // by default use existsing storage
			daDS = daBadger.NewDatastore(storage, storage.DB, TTL)
		}

		// lazyload validators cache, it's optional and we don't care if it errors out
		go preloadValidators(c.Context, logger, valDS, validatorCache)

		validatorStoreManager := validators.NewStoreManager(logger, validatorCache, valDS, int(math.Floor(TTL.Seconds()/2)), c.Uint("relay-store-queue-size"))
		validatorStoreManager.AttachMetrics(m)
		if c.Uint("relay-workers-store-validator") > 0 {
			validatorStoreManager.RunStore(c.Uint("relay-workers-store-validator"))
		}

		domainBuilder, err := ComputeDomain(types.DomainTypeAppBuilder, cfg.GenesisForkVersion, types.Root{}.String())
		if err != nil {
			return err
		}

		validatorRelay := validators.NewRegister(logger, domainBuilder, state, verificator, validatorStoreManager)
		validatorRelay.AttachMetrics(m)
		b := beacon.NewManager(logger, beacon.Config{
			BellatrixForkVersion: cfg.BellatrixForkVersion,
			CapellaForkVersion:   cfg.CapellaForkVersion,
		})

		auctioneer := auction.NewAuctioneer()

		var allowed map[[48]byte]struct{}
		albString := c.String("relay-allow-listed-builder")
		if albString != "" {
			allowed = make(map[[48]byte]struct{})
			for _, k := range strings.Split(albString, ",") {
				var pk types.PublicKey
				if err := pk.UnmarshalText([]byte(k)); err != nil {
					logger.WithError(err).With(log.F{"key": k}).Error("ALLOWED BUILDER NOT ADDED - wrong public key")
					continue
				}
				allowed[pk] = struct{}{}
			}
		}

		skBytes, err := hexutil.Decode(c.String("secretKey"))
		if err != nil {
			return err
		}
		sk, pk, err := blstools.SecretKeyFromBytes(skBytes)
		if err != nil {
			return err
		}

		bellatrixBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, cfg.BellatrixForkVersion, cfg.GenesisValidatorsRoot)
		if err != nil {
			return err
		}

		capellaBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, cfg.CapellaForkVersion, cfg.GenesisValidatorsRoot)
		if err != nil {
			return err
		}

		var exporter relay.DataExporter
		if c.Bool("data-export") {
			exportService := data.NewExportService(logger, c.String("data-export-dir"), c.Int("data-export-workers"))
			if err := exportService.RunParallel(c.Context, c.Int("data-export-workers")); err != nil {
				return fmt.Errorf("failed to run data exporter: %w", err)
			}

			logger.With(log.F{
				"service": "data-exporter",
				"datadir": c.String("data-export-dir"),
				"workers": c.Int("data-export-workers"),
			}).Info("initialized")

			exporter = exportService
		}

		r := relay.NewRelay(logger, relay.RelayConfig{
			BuilderSigningDomain: domainBuilder,
			BlockPublishDelay:    c.Duration("block-publication-delay"),
			ProposerSigningDomain: map[structs.ForkVersion]types.Domain{
				structs.ForkBellatrix: bellatrixBeaconProposer,
				structs.ForkCapella:   capellaBeaconProposer},
			PubKey:                pk,
			SecretKey:             sk,
			RegistrationCacheTTL:  c.Duration("relay-registrations-cache-ttl"),
			TTL:                   TTL,
			AllowedListedBuilders: allowed,
			PublishBlock:          c.Bool("relay-publish-block"),
			ExportData:            c.Bool("data-export"),
		}, beaconCli, validatorCache, valDS, verificator, state, ds, daDS, auctioneer, simFallb, exporter)
		r.AttachMetrics(m)

		a := api.NewApi(logger, r, validatorRelay, state, api.NewLimitter(c.Int("relay-submission-limit-rate"), c.Int("relay-submission-limit-burst"), allowed))
		a.AttachMetrics(m)
		logger.With(log.F{
			"service":     "relay",
			"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
		}).Info("initialized")

		if err := b.Init(c.Context, state, beaconCli, validatorStoreManager, validatorCache); err != nil {
			return fmt.Errorf("failed to init beacon manager: %w", err)
		}

		go b.Run(c.Context, state, beaconCli, validatorStoreManager, validatorCache)

		logger.Info("beacon manager ready")

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

		<-c.Context.Done()

		ctx, closeC := context.WithTimeout(context.Background(), shutdownTimeout/2)
		defer closeC()
		logger.Info("Shutdown initialized")
		err = srv.Shutdown(ctx)
		logger.Info("Shutdown returned ", err)

		ctx, closeC = context.WithTimeout(context.Background(), shutdownTimeout)
		defer closeC()
		finish := make(chan struct{})
		go closemanager(ctx, finish, validatorStoreManager, r)

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

func initBeaconClients(ctx context.Context, l log.Logger, endpoints []string, m *metrics.Metrics) (*bcli.MultiBeaconClient, error) {
	clients := make([]bcli.BeaconNode, 0, len(endpoints))

	for _, endpoint := range endpoints {
		client, err := bcli.NewBeaconClient(l, endpoint)
		if err != nil {
			return nil, err
		}
		client.AttachMetrics(m) // attach metrics
		clients = append(clients, client)
	}
	return bcli.NewMultiBeaconClient(l, clients), nil
}

func closemanager(ctx context.Context, finish chan struct{}, regMgr *validators.StoreManager, r *relay.Relay) {
	regMgr.Close(ctx)
	r.Close(ctx)
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
