package main

import (
	"context"
	"net"
	"net/http"
	"os"

	"time"

	"github.com/blocknative/dreamboat/metrics"
	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
	blst "github.com/supranational/blst/bindings/go"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

const (
	shutdownTimeout = 5 * time.Second
	version         = relay.Version
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
	&cli.BoolFlag{
		Name:  "checkKnownValidator",
		Usage: "rejects validator registration if it's not a known validator from the beacon",
		Value: false,
	},
}

var (
	config = relay.Config{Log: log.New()}
)

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

	if err := app.Run(os.Args); err != nil {
		config.Log.Fatal(err)
	}
}

func setup() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		sk, pk, err := setupKeys(c)
		if err != nil {
			return err
		}

		config = relay.Config{
			Log:                 logger(c),
			RelayRequestTimeout: c.Duration("timeout"),
			Network:             c.String("network"),
			BuilderCheck:        c.Bool("check-builder"),
			BuilderURLs:         c.StringSlice("builder"),
			BeaconEndpoints:     c.StringSlice("beacon"),
			PubKey:              pk,
			SecretKey:           sk,
			Datadir:             c.String("datadir"),
			TTL:                 c.Duration("ttl"),
			CheckKnownValidator: c.Bool("checkKnownValidator"),
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

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		g, ctx := errgroup.WithContext(c.Context)

		timeDataStoreStart := time.Now()

		storage, err := badger.NewDatastore(config.Datadir, &badger.DefaultOptions)
		if err != nil {
			config.Log.WithError(err).Fatal("failed to initialize datastore")
			return err
		}

		ds := &datastore.Datastore{TTLStorage: &datastore.TTLDatastoreBatcher{storage}}

		config.Log.
			WithFields(logrus.Fields{
				"service":     "datastore",
				"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
			}).Info("data store initialized")

		// setup the relay service
		service := &relay.DefaultService{
			Log:       config.Log,
			Config:    config,
			Datastore: ds,
		}

		g.Go(func() error {
			return service.Run(ctx)
		})

		api := api.NewApi(config.Log, service)

		m := metrics.NewMetrics()
		api.AttachMetrics(m)
		service.AttachMetrics(m)

		// run internal http server
		g.Go(func() (err error) {

			internalMux := http.NewServeMux()
			metrics.AttachProfiler(internalMux)
			if err = datastore.InitDatastoreMetrics(m); err != nil {
				return err
			}

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
		})

		// wait for the relay service to be ready
		select {
		case <-service.Ready():
		case <-ctx.Done():
			return g.Wait()
		}

		config.Log.Debug("relay service ready")

		mux := http.NewServeMux()
		api.AttachToHandler(mux)

		var svr http.Server
		// run the http server
		g.Go(func() (err error) {
			svr = http.Server{
				Addr:         c.String("addr"),
				ReadTimeout:  c.Duration("timeout"),
				WriteTimeout: c.Duration("timeout"),
				IdleTimeout:  time.Second * 2,
				BaseContext: func(l net.Listener) context.Context { // 99% not needed
					return ctx
				},
				Handler:        mux,
				MaxHeaderBytes: 4096,
			}
			config.Log.Info("http server listening")
			if err = svr.ListenAndServe(); err == http.ErrServerClosed {
				err = nil
			}

			return err
		})

		g.Go(func() error {
			defer svr.Close()
			<-ctx.Done()

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			return svr.Shutdown(ctx)
		})

		return g.Wait()
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
		fmt = &logrus.JSONFormatter{PrettyPrint: c.Bool("prettyprint")}
	default:
		fmt = new(logrus.TextFormatter)
	}

	return log.WithFormatter(fmt)
}

func withErrWriter(c *cli.Context) log.Option {
	return log.WithWriter(c.App.ErrWriter)
}
