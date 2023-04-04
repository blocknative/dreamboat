package config

/*
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
}
*/

type Config struct {
}
