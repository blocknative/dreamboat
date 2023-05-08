package config

import (
	"time"
)

type Config struct {

	// http server on which relay serves external connections
	ExternalHttp *HTTPConfig `config:"external_http"`

	// internal port for metrics profiling and management
	InternalHttp *HTTPConfig `config:"internal_http"`

	//
	Api *ApiConfig `config:"api"`

	//
	Relay *RelayConfig `config:"relay"`

	// configuration of beacon nodes
	Beacon *BeaconConfig `config:"beacon"`

	//
	Verify *VerifyConfig `config:"verify"`

	//
	Validators *ValidatorsConfig `config:"validators"`

	//
	BlockSimulation *BlockSimulationConfig `config:"block_simulation"`

	//
	Payload *PayloadConfig `config:"payload"`

	//
	DataAPI *DataAPIConfig `config:"dataapi"`
}

var DefaultHTTPConfig = &HTTPConfig{
	ReadTimeout:  2 * time.Second,
	WriteTimeout: 2 * time.Second,
}

type HTTPConfig struct {
	// address (ip+port) on which http should be served
	Address      string        `config:"address"`
	ReadTimeout  time.Duration `config:"read_timeout"`  //time.Second * 2,
	WriteTimeout time.Duration `config:"write_timeout"` //time.Second * 2,
}

var DefaultSQLConfig = &SQLConfig{
	MaxOpenConns:    10,
	MaxIdleConns:    10,
	ConnMaxIdleTime: 15 * time.Second,
}

type SQLConfig struct {
	// database connection query string
	URL string `config:"url"`

	MaxOpenConns    int           `config:"max_open_conns"`
	MaxIdleConns    int           `config:"max_idle_conns"`
	ConnMaxIdleTime time.Duration `config:"conn_max_idle_time"`
}

var DefaultBadgerDBConfig = &BadgerDBConfig{
	TTL: 48 * time.Hour,
}

type BadgerDBConfig struct {
	TTL time.Duration `config:"ttl"`
}

var DefaultApiConfig = &ApiConfig{
	SubmissionLimitRate:  2,
	SubmissionLimitBurst: 2,
}

type ApiConfig struct {
	Subscriber

	// submission request limit - rate per second
	SubmissionLimitRate int `config:"submission_limit_rate"`

	// submission request limit - burst value
	SubmissionLimitBurst int `config:"submission_limit_burst"`
}

var DefaultRelayConfig = &RelayConfig{
	PublishBlock:         true,
	MaxBlockPublishDelay: 500 * time.Millisecond,
}

type RelayConfig struct {
	Subscriber

	// name of the network in which relay oparates
	Network string `config:"network"` // mainnet

	// secret key used to sign messages
	SecretKey string `config:"secret_key"`

	// for publishing payloads to beacon nodes after a delivery
	PublishBlock bool `config:"publish_block"`

	// block publish delay
	MaxBlockPublishDelay time.Duration `config:"max_block_publish_delay"`

	// comma separated list of allowed builder pubkeys"
	AllowedBuilders []string `config:"allowed_builders,allow_dynamic"` // map[[48]byte]struct{}
}

var DefaultBeaconConfig = &BeaconConfig{
	PayloadAttributesSubscription: true,
}

type BeaconConfig struct {
	Subscriber

	// comma separate list of urls to beacon endpoints
	Addresses []string `config:"addresses"`
	// should payload attributes be enabled
	PayloadAttributesSubscription bool `config:"payload_attributes_subscription"`
	//
	EventTimeout time.Duration `config:"event_timeout"`
	//
	EventRestart int `config:"event_restart"`
	// timeout of beacon queries
	QueryTimeout time.Duration `config:"query_timeout"`
}

var DefaultBlockSimulation = &BlockSimulationConfig{}

type BlockSimulationConfig struct {
	RPC  BlockSimulationRPCConfig  `config:"rpc"`
	WS   BlockSimulationWSConfig   `config:"ws"`
	HTTP BlockSimulationHTTPConfig `config:"http"`
}

type BlockSimulationRPCConfig struct {
	// block validation rawurl (eg. ipc path)
	Address string `config:"address"`
}

type BlockSimulationWSConfig struct {
	//  block validation endpoint address (comma separated list)
	Address []string `config:"address"`
	// retry to other websocket connections on failure"
	Retry bool `config:"retry"`
}

type BlockSimulationHTTPConfig struct {
	Address string `config:"address"`
}

type ValidatorsConfig struct {
	// Address of postgress database for validator registrations, if empty - default, badger will be used",
	DB *SQLConfig `config:"db"`
	// BadgerDB config if sql is not used
	Badger BadgerDBConfig `config:"badger"`
	// The size of response queue, should be set to expected number of validators in one request
	QueueSize uint `config:"queue_size"`
	// Number of workers storing validators in parallel
	StoreWorkersNum uint64 `config:"store_workers"`
	// Registrations cache size
	RegistrationsCacheSize int `config:"registrations_cache_size"`
	// Registrations cache ttl
	RegistrationsCacheTTL time.Duration `config:"registrations_cache_ttl"`
}

var DefaultValidatorsConfig = &ValidatorsConfig{
	DB:                     DefaultSQLConfig,
	Badger:                 *DefaultBadgerDBConfig,
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

var DefaultVerifyConfig = &VerifyConfig{
	WorkersNum: 2000,
	QueueSize:  100_000,
}

type DataAPIConfig struct {
	// Address of postgress database for validator registrations, if empty - default, badger will be used",
	DB SQLConfig `config:"db"`
	// BadgerDB config if sql is not used
	Badger BadgerDBConfig `config:"badger"`
}

var DefaultDataAPIConfig = &DataAPIConfig{
	DB:     *DefaultSQLConfig,
	Badger: *DefaultBadgerDBConfig,
}

type PayloadConfig struct {
	// BadgerDB config if sql is not used
	Badger BadgerDBConfig `config:"badger"`
	// number of payloads to cache for fast in-memory reads
	CacheSize int `config:"cache_size"`
}

var DefaultPayloadConfig = &PayloadConfig{
	Badger:    *DefaultBadgerDBConfig,
	CacheSize: 1_000,
}
