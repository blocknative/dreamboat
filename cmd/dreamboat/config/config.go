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

	//
	Warehouse *WarehouseConfig `config:"warehouse"`

	//
	Distributed *DistributedConfig `config:"distributed"`
}

var DefaultHTTPConfig = &HTTPConfig{
	ReadTimeout:  5 * time.Second,
	WriteTimeout: 5 * time.Second,
	IdleTimeout:  5 * time.Second,
}

type HTTPConfig struct {
	// address (ip+port) on which http should be served
	Address      string        `config:"address"`
	ReadTimeout  time.Duration `config:"read_timeout"`  //time.Second * 2,
	IdleTimeout  time.Duration `config:"idle_timeout"`  //time.Second * 2,
	WriteTimeout time.Duration `config:"write_timeout"` //time.Second * 2,
}

var DefaultSQLConfig = &SQLConfig{
	MaxOpenConns:    15,
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
	TTL: 24 * time.Hour,
}

type BadgerDBConfig struct {
	TTL time.Duration `config:"ttl"`
}

var DefaultApiConfig = &ApiConfig{
	SubmissionLimitRate:  2,
	SubmissionLimitBurst: 2,
	LimitterCacheSize:    1_000,
	DataLimit:            450,
}

type ApiConfig struct {
	Subscriber

	// comma separated list of allowed builder pubkeys"
	AllowedBuilders []string `config:"allowed_builders,allow_dynamic"` // map[[48]byte]struct{}

	// submission request limit - rate per second
	SubmissionLimitRate int `config:"submission_limit_rate,allow_dynamic"`

	// submission request limit - burst value
	SubmissionLimitBurst int `config:"submission_limit_burst,allow_dynamic"`

	// rate limitter cache entries size
	LimitterCacheSize int `config:"limitter_cache_size"`

	// limit of data returned in one response
	DataLimit int `config:"datalimit,allow_dynamic"`

	// this flag set to allows to return errors when endpoint is disabled
	ErrorsOnDisable bool `config:"errors_on_disable,allow_dynamic"`
}

var DefaultRelayConfig = &RelayConfig{
	PublishBlock:               true,
	GetPayloadResponseDelay:    800 * time.Millisecond,
	GetPayloadRequestTimeLimit: 4 * time.Second,
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
	GetPayloadResponseDelay time.Duration `config:"get_payload_response_delay,allow_dynamic"`

	// deadline for calling get Payload
	GetPayloadRequestTimeLimit time.Duration `config:"get_payload_request_time_limit,allow_dynamic"`

	// comma separated list of allowed builder pubkeys"
	AllowedBuilders []string `config:"allowed_builders,allow_dynamic"` // map[[48]byte]struct{}
}

var DefaultBeaconConfig = &BeaconConfig{
	PayloadAttributesSubscription: true,
	EventRestart:                  5,
	EventTimeout:                  16 * time.Second,
	QueryTimeout:                  20 * time.Second,
}

type BeaconConfig struct {
	Subscriber

	// comma separate list of urls to beacon endpoints
	Addresses []string `config:"addresses,allow_dynamic"`

	// comma separate list of urls to beacon endpoints
	PublishAddresses []string `config:"publish_addresses,allow_dynamic"`

	// comma separate list of urls to beacon endpoints
	PublishAddressesV2 []string `config:"publish_addresses_v2,allow_dynamic"`

	// should payload attributes be enabled
	PayloadAttributesSubscription bool `config:"payload_attributes_subscription,allow_dynamic"`

	//
	EventTimeout time.Duration `config:"event_timeout"`

	//
	EventRestart int `config:"event_restart"`

	// timeout of beacon queries
	QueryTimeout time.Duration `config:"query_timeout"`
}

var DefaultBlockSimulation = &BlockSimulationConfig{
	WS: &BlockSimulationWSConfig{
		Retry: true,
	},
	RPC:  &BlockSimulationRPCConfig{},
	HTTP: &BlockSimulationHTTPConfig{},
}

type BlockSimulationConfig struct {
	RPC  *BlockSimulationRPCConfig  `config:"rpc"`
	WS   *BlockSimulationWSConfig   `config:"ws"`
	HTTP *BlockSimulationHTTPConfig `config:"http"`
}

type BlockSimulationRPCConfig struct {
	// block validation rawurl (eg. ipc path)
	Address string `config:"address"`
}

type BlockSimulationWSConfig struct {
	Subscriber
	//  block validation endpoint address (comma separated list)
	Address []string `config:"address,allow_dynamic"`
	// retry to other websocket connections on failure"
	Retry bool `config:"retry,allow_dynamic"`
}

type BlockSimulationHTTPConfig struct {
	Subscriber
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
	StoreWorkersNum uint `config:"store_workers"`
	// Registrations cache size
	RegistrationsCacheSize int `config:"registrations_cache_size"`
	// Registrations cache ttl
	RegistrationsReadCacheTTL time.Duration `config:"registrations_cache_ttl"`
	// Registrations cache ttl
	RegistrationsWriteCacheTTL time.Duration `config:"registrations_write_cache_ttl"`
}

var DefaultValidatorsConfig = &ValidatorsConfig{
	DB:                         DefaultSQLConfig,
	Badger:                     *DefaultBadgerDBConfig,
	QueueSize:                  100_000,
	StoreWorkersNum:            400,
	RegistrationsCacheSize:     600_000,
	RegistrationsReadCacheTTL:  time.Hour,
	RegistrationsWriteCacheTTL: 12 * time.Hour,
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
	// BadgerDB config
	Badger BadgerDBConfig `config:"badger"`
	// number of payloads to cache for fast in-memory reads

	CacheSize int `config:"cache_size"`

	// Redis config
	Redis RedisDBConfig `config:"redis"`

	// TTL of payload data
	TTL time.Duration `config:"ttl,allow_dynamic"`
}

var DefaultPayloadConfig = &PayloadConfig{
	Badger:    *DefaultBadgerDBConfig,
	TTL:       24 * time.Hour,
	CacheSize: 1_000,
	Redis:     *DefaultRedisDBConfig,
}

type RedisDBConfig struct {
	Read  *RedisConfig `config:"read"`
	Write *RedisConfig `config:"write"`
}

var DefaultRedisDBConfig = &RedisDBConfig{
	Read:  &RedisConfig{},
	Write: &RedisConfig{},
}

type WarehouseConfig struct {
	// Data directory where the data is stored in the warehouse
	Directory string `config:"directory"`

	// Number of workers for storing data in warehouse, if 0, then data is not exported
	WorkerNumber int `config:"workers"`

	// Size of the buffer for processing requests
	Buffer int `config:"buffer"`
}

var DefaultWarehouseConfig = &WarehouseConfig{
	Directory:    "/data/relay/warehouse",
	WorkerNumber: 32,
	Buffer:       1_000,
}

type DistributedConfig struct {
	Redis *RedisStreamConfig `config:"redis"`

	InstanceID string `config:"id"`

	// Number of workers for storing data in warehouse, if 0, then data is not exported
	WorkerNumber int `config:"workers"`

	// Stream internal channel size
	StreamQueueSize int `config:"stream_queue_size"`

	// stream entire block for every bid that is served in GetHeader requests.
	StreamServedBids bool `config:"stream_served_bids"`
}

var DefaultDistributedConfig = &DistributedConfig{
	Redis: &RedisStreamConfig{
		Topic: "relay",
	},
	WorkerNumber:     100,
	StreamQueueSize:  100,
	StreamServedBids: true,
}

type RedisStreamConfig struct {
	Topic   string `config:"topic"`
	Address string `config:"address"`
}

var DefaultRedisStreamConfig = &RedisStreamConfig{
	Topic: "relay",
}

type RedisConfig struct {
	Address string `config:"address"`
}
