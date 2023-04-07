package config

type Source interface {
	Load() (Config, error)
}

type ConfigManager struct {
	*Config
	s Source
}

func NewConfigManager(s Source) *ConfigManager {
	return &ConfigManager{
		s: s,
	}
}

func (cm *ConfigManager) Reload() {
	cm.s.Load()
}

func (cm *ConfigManager) Load() {
	//cfg, err := cm.s.Load()
}
