package config

import "github.com/blocknative/dreamboat/structs"

type Source interface {
	Load(cfg *Config, initial bool) error
}

type ConfigManager struct {
	*Config
	s Source
}

func NewConfigManager(s Source) *ConfigManager {
	dc := DefaultConfig()
	return &ConfigManager{
		s:      s,
		Config: &dc,
	}
}

func DefaultConfig() Config {
	external := *DefaultHTTPConfig
	internal := *DefaultHTTPConfig
	c := Config{
		ExternalHttp:    &external,
		InternalHttp:    &internal,
		Api:             DefaultApiConfig,
		Relay:           DefaultRelayConfig,
		Beacon:          DefaultBeaconConfig,
		BlockSimulation: DefaultBlockSimulation,
		Verify:          DefaultVerifyConfig,
		Validators:      DefaultValidatorsConfig,
		Payload:         DefaultPayloadConfig,
		DataAPI:         DefaultDataAPIConfig,
		Warehouse:       DefaultWarehouseConfig,
	}
	c.ExternalHttp.Address = "0.0.0.0:18550"
	c.InternalHttp.Address = "0.0.0.0:19550"
	c.Relay.Network = "mainnet"

	return c
}

func (cm *ConfigManager) Reload() error {
	return cm.s.Load(cm.Config, false)
}

func (cm *ConfigManager) Load() error {
	return cm.s.Load(cm.Config, true)
}

type Listener interface {
	OnConfigChange(change structs.OldNew) error
}

type Propagator interface {
	Propagate(change structs.OldNew)
}

type Subscriber struct {
	listeners []Listener
}

func (s *Subscriber) SubscribeForUpdates(l Listener) {
	s.listeners = append(s.listeners, l)
}

func (s Subscriber) Propagate(change structs.OldNew) {
	for _, l := range s.listeners {
		l.OnConfigChange(change)
	}
}
