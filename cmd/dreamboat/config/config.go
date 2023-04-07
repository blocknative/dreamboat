package config

import "time"

type Config struct {
	configFile string

	// http server on which relay serves external connections
	ExternalHttp HTTPConfig `config:"external_http"` // localhost:18550

	// internal port for metrics profiling and management
	InternalHttp HTTPConfig `config:"internal_http"` //"0.0.0.0:19550"

	//
	Relay RelayConfig `config:"relay"`

	//
	Beacon BeaconConfig `config:"beacon"`

	//
	Verify VerifyConfig `config:"verify"`

	//
	Validators ValidatorsConfig `config:"validators"`

	//
	Payload PayloadConfig `config:"payload"`

	//
	DataAPI DataAPIConfig `config:"dataapi"`
}

func NewConfig(configFile string) *Config {
	return &Config{
		configFile: configFile,
	}
}

type HTTPConfig struct {
	// address (ip+port) on which http should be served
	Address      string        `config:"address"`
	ReadTimeout  time.Duration `config:"read_timeout"`  //time.Second * 2,
	WriteTimeout time.Duration `config:"write_timeout"` //time.Second * 2,
}

var DefaultHTTPConfig = HTTPConfig{
	ReadTimeout:  2 * time.Second,
	WriteTimeout: 2 * time.Second,
}

type SQLConfig struct {
	// database connection query string
	URL string `config:"url"`

	MaxOpenConns    int           `config:"max_open_conns"`
	MaxIdleConns    int           `config:"max_idle_conns"`
	ConnMaxIdleTime time.Duration `config:"conn_max_idle_time"`
}

var DefaultSQLConfig = SQLConfig{
	MaxOpenConns:    10,
	MaxIdleConns:    10,
	ConnMaxIdleTime: 15 * time.Second,
}

type BadgerDBConfig struct {
	TTL time.Duration `config:"ttl"`
}

var DefaultBadgerDBConfig = BadgerDBConfig{
	TTL: 48 * time.Hour,
}

type RelayConfig struct {
	// name of the network in which relay oparates
	Network string `config:"network"` // mainnet

	// secret key used to sign messages
	SecretKey string `config:"secret_key"`

	// for publishing payloads to beacon nodes after a delivery
	PublishBlock bool `config:"publish_block"`

	// block publish delay
	MaxBlockPublishDelay time.Duration `config:"max_block_publish_delay"`
}

var DefaultRelayConfig = RelayConfig{
	PublishBlock:         true,
	MaxBlockPublishDelay: 500 * time.Millisecond,
}

type BeaconConfig struct {
	// comma separate list of urls to beacon endpoints
	Addresses []string `config:"addresses"`
}

type BlockSimulationConfig struct {
}

type ValidatorsConfig struct {
	// Address of postgress database for validator registrations, if empty - default, badger will be used",
	DB SQLConfig `config:"db"`
	// BadgerDB config if sql is not used
	Badger BadgerDBConfig `config:"badger"`
	// The size of response queue, should be set to expected number of validators in one request
	QueueSize uint64 `config:"queue_size"`
	// Number of workers storing validators in parallel
	StoreWorkersNum uint64 `config:"store_workers"`
	// Registrations cache size
	RegistrationsCacheSize int `config:"registrations_cache_size"`
	// Registrations cache ttl
	RegistrationsCacheTTL time.Duration `config:"registrations_cache_ttl"`
}

var DefaultValidatorsConfig = ValidatorsConfig{
	QueueSize:              100_000,
	StoreWorkersNum:        400,
	RegistrationsCacheSize: 600_000,
	RegistrationsCacheTTL:  time.Hour,
}

type VerifyConfig struct {
	// Number of workers running verify in parallel
	WorkersNum uint64 `config:"workers"`
	//size of verify queue
	QueueSize uint `config:"queue_size"`
}

var DefaultVerifyConfig = VerifyConfig{
	WorkersNum: 2000,
	QueueSize:  100_000,
}

type DataAPIConfig struct {
	// Address of postgress database for validator registrations, if empty - default, badger will be used",
	DB SQLConfig `config:"db"`
	// BadgerDB config if sql is not used
	Badger BadgerDBConfig `config:"badger"`
}

var DefaultDataAPIConfig = DataAPIConfig{}

type PayloadConfig struct {
	// BadgerDB config if sql is not used
	Badger BadgerDBConfig `config:"badger"`
	// number of payloads to cache for fast in-memory reads
	CacheSize int `config:"cache_size"`
}

var DefaultPayloadConfig = PayloadConfig{
	Badger:    DefaultBadgerDBConfig,
	CacheSize: 1_000,
}

/*
var flags = []cli.Flag{
	&cli.DurationFlag{
		Name:    "ttl",
		Usage:   "ttl of the data",
		Value:   24 * time.Hour,
		EnvVars: []string{"BN_RELAY_TTL"},
	},
	&cli.Uint64Flag{
		Name:    "relay-store-queue-size",
		Usage:   "size of store queue",
		Value:   100_000,
		EnvVars: []string{"RELAY_STORE_QUEUE_SIZE"},
	},
	&cli.StringFlag{
		Name:    "relay-dataapi-database-url",
		Usage:   "address of postgress database for dataapi, if empty - default, badger will be used",
		Value:   "",
		EnvVars: []string{"RELAY_DATAAPI_DATABASE_URL"},
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
}
*/
