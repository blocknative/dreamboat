package config

type Source interface {
	Load(*Config) error
}

type ConfigManager struct {
	*Config
	s Source
}

func NewConfigManager(s Source) *ConfigManager {
	return &ConfigManager{
		s:      s,
		Config: defaultConfig(),
	}
}

func defaultConfig() *Config {
	c := &Config{
		ExternalHttp: DefaultHTTPConfig,
		InternalHttp: DefaultHTTPConfig,
		Api:          DefaultApiConfig,
		Relay:        DefaultRelayConfig,
		Verify:       DefaultVerifyConfig,
		Validators:   DefaultValidatorsConfig,
		Payload:      DefaultPayloadConfig,
		DataAPI:      DefaultDataAPIConfig,
	}
	c.ExternalHttp.Address = "0.0.0.0:18550"
	c.InternalHttp.Address = "0.0.0.0:19550"

	return c
}

func (cm *ConfigManager) Reload() error {

	testC := &Config{}
	// check file before loading content
	if err := cm.s.Load(testC); err != nil {
		return err
	}

	return cm.s.Load(cm.Config)
}

func (cm *ConfigManager) Load() error {
	return cm.s.Load(cm.Config)
}
