package config

// NewTestStore creates a lightweight ConfigStore for package-external tests.
// Optional refs are exposed through LoadedPaths and can be used with
// CaptureStalenessSnapshot for file-backed compatibility tests.
func NewTestStore(cfg *Config, refs ...string) *ConfigStore {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.setDefaults(".")

	store := &ConfigStore{
		config:          cfg,
		globalConfigKey: globalConfigKey,
		loadedPaths:     append([]string(nil), refs...),
	}
	if len(refs) > 0 {
		store.workspaceConfigKey = refs[len(refs)-1]
	}
	return store
}
