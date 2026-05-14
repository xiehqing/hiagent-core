package config

import (
	"charm.land/catwalk/pkg/catwalk"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	hyperp "github.com/xiehqing/hiagent-core/internal/agent/hyper"
	"github.com/xiehqing/hiagent-core/internal/env"
	"github.com/xiehqing/hiagent-core/internal/home"
	"github.com/xiehqing/hiagent-core/internal/oauth"
	"github.com/xiehqing/hiagent-core/internal/oauth/copilot"
	"github.com/xiehqing/hiagent-core/internal/oauth/hyper"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// configSnapshot captures metadata about a config record or compatibility file
// at a point in time.
type configSnapshot struct {
	Ref       string
	Source    string
	Exists    bool
	UpdatedAt int64
	Size      int64
	ModTime   int64 // UnixNano
}

// RuntimeOverrides holds per-session settings that are never persisted to
// disk. They are applied on top of the loaded Config and survive only for
// the lifetime of the process (or workspace).
type RuntimeOverrides struct {
	SkipPermissionRequests bool
}

// ConfigStore is the single entry point for all config access. It owns the
// pure-data Config, runtime state (working directory, resolver, known
// providers), and persistence to both global and workspace config files.
type ConfigStore struct {
	config             *Config
	workingDir         string
	resolver           VariableResolver
	globalConfigKey    string   // e.g. "global"
	workspaceConfigKey string   // e.g. "workspace:/abs/path"
	loadedPaths        []string // config keys that were successfully loaded
	knownProviders     []catwalk.Provider
	overrides          RuntimeOverrides
	trackedConfigPaths []string                  // unique config refs (DB keys or compatibility file paths)
	snapshots          map[string]configSnapshot // ref -> snapshot at last capture
	autoReloadDisabled bool                      // set during load/reload to prevent re-entrancy
	reloadInProgress   bool                      // set during reload to avoid disk writes mid-reload
	conn               *sql.DB
}

// Config returns the pure-data config struct (read-only after load).
func (s *ConfigStore) Config() *Config {
	return s.config
}

func (s *ConfigStore) Conn() *sql.DB {
	return s.conn
}

// WorkingDir returns the current working directory.
func (s *ConfigStore) WorkingDir() string {
	return s.workingDir
}

// Resolver returns the variable resolver.
func (s *ConfigStore) Resolver() VariableResolver {
	return s.resolver
}

// Resolve resolves a variable reference using the configured resolver.
func (s *ConfigStore) Resolve(key string) (string, error) {
	if s.resolver == nil {
		return "", fmt.Errorf("no variable resolver configured")
	}
	return s.resolver.ResolveValue(key)
}

// KnownProviders returns the list of known providers.
func (s *ConfigStore) KnownProviders() []catwalk.Provider {
	return s.knownProviders
}

// SetupAgents configures the coder and task agents on the config.
func (s *ConfigStore) SetupAgents() {
	s.config.SetupAgents()
}

// Overrides returns the runtime overrides for this store.
func (s *ConfigStore) Overrides() *RuntimeOverrides {
	return &s.overrides
}

func applyRuntimeOverrides(cfg *Config, overrides RuntimeOverrides) {
	if cfg == nil {
		return
	}
	if cfg.Options == nil {
		cfg.Options = &Options{}
	}
}

// LoadedPaths returns the config keys that were successfully loaded.
func (s *ConfigStore) LoadedPaths() []string {
	return slices.Clone(s.loadedPaths)
}

func workspaceConfigKeyForDir(workingDir string) (string, error) {
	if workingDir == "" {
		return "", ErrNoWorkspaceConfig
	}
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", err
	}
	return workspaceConfigPrefix + filepath.Clean(abs), nil
}

// configKey returns the storage key for the given scope.
func (s *ConfigStore) configKey(scope Scope) (string, error) {
	switch scope {
	case ScopeWorkspace:
		if s.workspaceConfigKey == "" {
			return "", ErrNoWorkspaceConfig
		}
		return s.workspaceConfigKey, nil
	default:
		return s.globalConfigKey, nil
	}
}

// HasConfigField checks whether a key exists in the config file for the given
// scope.
func (s *ConfigStore) HasConfigField(scope Scope, key string) bool {
	configKey, err := s.configKey(scope)
	if err != nil {
		return false
	}
	record, err := GetDataConfigByWorkingDir(s.conn, configKey)
	if err != nil {
		return false
	}
	if record == nil {
		return false
	}
	return gjson.Get(record.Config, key).Exists()
}

// SetConfigField sets a key/value pair in the config file for the given scope.
// After a successful write, it automatically reloads config to keep in-memory
// state fresh.
func (s *ConfigStore) SetConfigField(scope Scope, key string, value any) error {
	configKey, err := s.configKey(scope)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	record, err := GetDataConfigByWorkingDir(s.conn, configKey)
	if err != nil {
		return fmt.Errorf("failed to read config record: %w", err)
	}
	data := "{}"
	if record != nil && record.Config != "" {
		data = record.Config
	}

	newValue, err := sjson.Set(string(data), key, value)
	if err != nil {
		return fmt.Errorf("failed to set config field %s: %w", key, err)
	}

	_, err = AddDataConfig(s.conn, DataConfig{
		WorkingDir: configKey,
		Config:     newValue,
	})
	if err != nil {
		return fmt.Errorf("failed to persist config record: %w", err)
	}

	//if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
	//	return fmt.Errorf("failed to create config directory %q: %w", path, err)
	//}
	//if err := os.WriteFile(path, []byte(newValue), 0o600); err != nil {
	//	return fmt.Errorf("failed to write config file: %w", err)
	//}

	// Auto-reload to keep in-memory state fresh after config edits.
	// We use context.Background() since this is an internal operation that
	// shouldn't be cancelled by user context.
	if err := s.autoReload(context.Background()); err != nil {
		// Log warning but don't fail the write - disk is already updated.
		slog.Warn("Config file updated but failed to reload in-memory state", "error", err)
	}

	return nil
}

// RemoveConfigField removes a key from the config file for the given scope.
// After a successful write, it automatically reloads config to keep in-memory
// state fresh.
func (s *ConfigStore) RemoveConfigField(scope Scope, key string) error {
	configKey, err := s.configKey(scope)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	record, err := GetDataConfigByWorkingDir(s.conn, configKey)
	if err != nil {
		return fmt.Errorf("failed to read config record: %w", err)
	}
	if record == nil {
		return nil
	}

	newValue, err := sjson.Delete(record.Config, key)
	if err != nil {
		return fmt.Errorf("failed to delete config field %s: %w", key, err)
	}
	_, err = AddDataConfig(s.conn, DataConfig{
		WorkingDir: configKey,
		Config:     newValue,
	})
	if err != nil {
		return fmt.Errorf("failed to persist config record: %w", err)
	}
	//if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
	//	return fmt.Errorf("failed to create config directory %q: %w", path, err)
	//}
	//if err := os.WriteFile(path, []byte(newValue), 0o600); err != nil {
	//	return fmt.Errorf("failed to write config file: %w", err)
	//}

	// Auto-reload to keep in-memory state fresh after config edits.
	if err := s.autoReload(context.Background()); err != nil {
		slog.Warn("Config file updated but failed to reload in-memory state", "error", err)
	}

	return nil
}

// autoReload conditionally reloads config from disk after writes.
// It returns nil (no error) for expected skip cases: when auto-reload is
// disabled during load/reload flows, or when working directory is not set
// (e.g., during testing). Only actual reload failures return an error.
func (s *ConfigStore) autoReload(ctx context.Context) error {
	if s.autoReloadDisabled {
		return nil // Expected skip: already in load/reload flow
	}
	if s.workingDir == "" {
		return nil // Expected skip: working directory not set
	}
	return s.ReloadFromDisk(ctx)
}

// mustMarshalConfig marshals the config to JSON bytes, returning empty JSON on
// error.
func mustMarshalConfig(cfg *Config) []byte {
	data, err := json.Marshal(cfg)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// ReloadFromDisk re-runs the config load/merge flow and updates the in-memory
// config atomically. It rebuilds the staleness snapshot after successful reload.
// On failure, the store state is rolled back to its previous state.
func (s *ConfigStore) ReloadFromDisk(ctx context.Context) error {
	if s.workingDir == "" {
		return fmt.Errorf("cannot reload: working directory not set")
	}

	// Disable auto-reload during reload to prevent nested/re-entrant calls.
	s.autoReloadDisabled = true
	s.reloadInProgress = true
	defer func() {
		s.autoReloadDisabled = false
		s.reloadInProgress = false
	}()

	configPaths := lookupConfigs(s.workingDir)
	_ = configPaths
	cfg, loadedPaths, err := loadFromDB(s.conn, s.workingDir)
	if err != nil {
		return fmt.Errorf("failed to reload config from database: %w", err)
	}

	// Apply defaults (using existing data directory if set)
	cfg.setDefaults(s.workingDir)
	cfg.setDataDir(s.workingDir, "")

	// Validate hooks after all config merging is complete so matcher
	// regexes are recompiled on the reloaded config (mirrors Load).
	if err := cfg.ValidateHooks(); err != nil {
		return fmt.Errorf("invalid hook configuration on reload: %w", err)
	}

	// Preserve runtime overrides
	overrides := s.overrides
	applyRuntimeOverrides(cfg, overrides)

	// Reconfigure providers
	env := env.New()
	resolver := NewShellVariableResolver(env)
	providers, err := KnownProviders(s.conn)
	if err != nil {
		return fmt.Errorf("failed to load providers during reload: %w", err)
	}

	if err := cfg.configureProviders(s, env, resolver, providers); err != nil {
		return fmt.Errorf("failed to configure providers during reload: %w", err)
	}

	// Save current state for potential rollback
	oldConfig := s.config
	oldLoadedPaths := s.loadedPaths
	oldResolver := s.resolver
	oldKnownProviders := s.knownProviders
	oldOverrides := s.overrides
	oldWorkspaceKey := s.workspaceConfigKey

	// Update store state BEFORE running model/agent setup (so they see new config)
	s.config = cfg
	s.loadedPaths = loadedPaths
	s.resolver = resolver
	s.knownProviders = providers
	s.overrides = overrides
	s.workspaceConfigKey = oldWorkspaceKey

	// Mirror startup flow: setup models and agents against NEW config
	var setupErr error
	if !cfg.IsConfigured() {
		slog.Warn("No providers configured after reload")
	} else {
		if err := configureSelectedModels(s, providers, false); err != nil {
			setupErr = fmt.Errorf("failed to configure selected models during reload: %w", err)
		} else {
			s.SetupAgents()
		}
	}

	// Rollback on setup failure
	if setupErr != nil {
		s.config = oldConfig
		s.loadedPaths = oldLoadedPaths
		s.resolver = oldResolver
		s.knownProviders = oldKnownProviders
		s.overrides = oldOverrides
		s.workspaceConfigKey = oldWorkspaceKey
		return setupErr
	}

	// Rebuild staleness tracking
	s.captureStalenessSnapshot(loadedPaths)

	return nil
}

// captureStalenessSnapshot is an alias for CaptureStalenessSnapshot for internal use.
func (s *ConfigStore) captureStalenessSnapshot(paths []string) {
	s.CaptureStalenessSnapshot(paths)
}

// CaptureStalenessSnapshot captures snapshots for the given paths, building the
// tracked config paths list. Paths are deduplicated and normalized.
func (s *ConfigStore) CaptureStalenessSnapshot(paths []string) {
	// Build unique set of config refs.
	seen := make(map[string]struct{})
	for _, p := range paths {
		if p == "" {
			continue
		}
		seen[p] = struct{}{}
	}

	// Also track workspace and global config keys if set.
	if s.workspaceConfigKey != "" {
		seen[s.workspaceConfigKey] = struct{}{}
	}
	if s.globalConfigKey != "" {
		seen[s.globalConfigKey] = struct{}{}
	}

	// Build sorted list for deterministic ordering
	s.trackedConfigPaths = make([]string, 0, len(seen))
	for p := range seen {
		s.trackedConfigPaths = append(s.trackedConfigPaths, p)
	}
	slices.Sort(s.trackedConfigPaths)

	// Capture initial snapshots
	s.RefreshStalenessSnapshot()
}

// RefreshStalenessSnapshot captures fresh snapshots of all tracked config files.
// Call this after reloading config to clear dirty state.
func (s *ConfigStore) RefreshStalenessSnapshot() error {
	if s.snapshots == nil {
		s.snapshots = make(map[string]configSnapshot)
	}

	for _, ref := range s.trackedConfigPaths {
		snapshot, err := s.snapshotForRef(ref)
		if err != nil {
			s.snapshots[ref] = configSnapshot{Ref: ref}
			return err
		}
		s.snapshots[ref] = snapshot
	}

	return nil
}

func (s *ConfigStore) snapshotForRef(ref string) (configSnapshot, error) {
	isDBRef := ref == globalConfigKey || strings.HasPrefix(ref, workspaceConfigPrefix)

	if s.conn != nil {
		record, err := GetDataConfigByWorkingDir(s.conn, ref)
		if err != nil {
			return configSnapshot{}, err
		}
		if record != nil {
			return configSnapshot{
				Ref:       ref,
				Source:    "db",
				Exists:    true,
				UpdatedAt: record.UpdatedAt,
			}, nil
		}
		if isDBRef {
			return configSnapshot{
				Ref:    ref,
				Source: "db",
				Exists: false,
			}, nil
		}
	}

	info, err := os.Stat(ref)
	exists := err == nil && !info.IsDir()
	snapshot := configSnapshot{
		Ref:    ref,
		Source: "file",
		Exists: exists,
	}
	if exists {
		snapshot.Size = info.Size()
		snapshot.ModTime = info.ModTime().UnixNano()
	}
	if err != nil && !os.IsNotExist(err) {
		return snapshot, err
	}
	return snapshot, nil
}

func PushPopHiAgentEnv() func() {
	var found []string
	for _, ev := range os.Environ() {
		if strings.HasPrefix(ev, "HIAGENT_") {
			pair := strings.SplitN(ev, "=", 2)
			if len(pair) != 2 {
				continue
			}
			found = append(found, strings.TrimPrefix(pair[0], "HIAGENT_"))
		}
	}
	backups := make(map[string]string)
	for _, ev := range found {
		backups[ev] = os.Getenv(ev)
	}

	for _, ev := range found {
		os.Setenv(ev, os.Getenv("CRUSH_"+ev))
	}

	restore := func() {
		for k, v := range backups {
			os.Setenv(k, v)
		}
	}
	return restore
}

func (c *Config) configureProviders(store *ConfigStore, env env.Env, resolver VariableResolver, knownProviders []catwalk.Provider) error {
	knownProviderNames := make(map[string]bool)
	restore := PushPopHiAgentEnv()
	defer restore()

	// When disable_default_providers is enabled, skip all default/embedded
	// providers entirely. Users must fully specify any providers they want.
	// We skip to the custom provider validation loop which handles all
	// user-configured providers uniformly.
	if c.Options.DisableDefaultProviders {
		knownProviders = nil
	}

	for _, p := range knownProviders {
		knownProviderNames[string(p.ID)] = true
		config, configExists := c.Providers.Get(string(p.ID))
		// if the user configured a known provider we need to allow it to override a couple of parameters
		if configExists {
			if config.BaseURL != "" {
				p.APIEndpoint = config.BaseURL
			}
			if config.APIKey != "" {
				p.APIKey = config.APIKey
			}
			if len(config.Models) > 0 {
				models := []catwalk.Model{}
				seen := make(map[string]bool)

				for _, model := range config.Models {
					if seen[model.ID] {
						continue
					}
					seen[model.ID] = true
					if model.Name == "" {
						model.Name = model.ID
					}
					models = append(models, model)
				}
				for _, model := range p.Models {
					if seen[model.ID] {
						continue
					}
					seen[model.ID] = true
					if model.Name == "" {
						model.Name = model.ID
					}
					models = append(models, model)
				}

				p.Models = models
			}
		}

		headers := map[string]string{}
		if len(p.DefaultHeaders) > 0 {
			maps.Copy(headers, p.DefaultHeaders)
		}
		if len(config.ExtraHeaders) > 0 {
			maps.Copy(headers, config.ExtraHeaders)
		}
		for k, v := range headers {
			resolved, err := resolver.ResolveValue(v)
			if err != nil {
				slog.Error("Could not resolve provider header", "err", err.Error())
				continue
			}
			headers[k] = resolved
		}
		prepared := ProviderConfig{
			ID:                 string(p.ID),
			Name:               p.Name,
			BaseURL:            p.APIEndpoint,
			APIKey:             p.APIKey,
			APIKeyTemplate:     p.APIKey, // Store original template for re-resolution
			OAuthToken:         config.OAuthToken,
			Type:               p.Type,
			Disable:            config.Disable,
			SystemPromptPrefix: config.SystemPromptPrefix,
			ExtraHeaders:       headers,
			ExtraBody:          config.ExtraBody,
			ExtraParams:        make(map[string]string),
			Models:             p.Models,
		}

		switch {
		case p.ID == catwalk.InferenceProviderAnthropic && config.OAuthToken != nil:
			// Claude Code subscription is not supported anymore. Remove to show onboarding.
			if !store.reloadInProgress {
				store.RemoveConfigField(ScopeGlobal, "providers.anthropic")
			}
			c.Providers.Del(string(p.ID))
			continue
		case p.ID == catwalk.InferenceProviderCopilot && config.OAuthToken != nil:
			prepared.SetupGitHubCopilot()
		}

		switch p.ID {
		// Handle specific providers that require additional configuration
		case catwalk.InferenceProviderVertexAI:
			var (
				project  = env.Get("VERTEXAI_PROJECT")
				location = env.Get("VERTEXAI_LOCATION")
			)
			if project == "" || location == "" {
				if configExists {
					slog.Warn("Skipping Vertex AI provider due to missing credentials")
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.ExtraParams["project"] = project
			prepared.ExtraParams["location"] = location
		case catwalk.InferenceProviderAzure:
			endpoint, err := resolver.ResolveValue(p.APIEndpoint)
			if err != nil || endpoint == "" {
				if configExists {
					slog.Warn("Skipping Azure provider due to missing API endpoint", "provider", p.ID, "error", err)
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.BaseURL = endpoint
			prepared.ExtraParams["apiVersion"] = env.Get("AZURE_OPENAI_API_VERSION")
		case catwalk.InferenceProviderBedrock:
			if !hasAWSCredentials(env) {
				if configExists {
					slog.Warn("Skipping Bedrock provider due to missing AWS credentials")
					c.Providers.Del(string(p.ID))
				}
				continue
			}
			prepared.ExtraParams["region"] = env.Get("AWS_REGION")
			if prepared.ExtraParams["region"] == "" {
				prepared.ExtraParams["region"] = env.Get("AWS_DEFAULT_REGION")
			}
			for _, model := range p.Models {
				if !strings.HasPrefix(model.ID, "anthropic.") {
					return fmt.Errorf("bedrock provider only supports anthropic models for now, found: %s", model.ID)
				}
			}
		case catwalk.InferenceProvider("hyper"):
			if apiKey := env.Get("HYPER_API_KEY"); apiKey != "" {
				prepared.APIKey = apiKey
				prepared.APIKeyTemplate = apiKey
			} else {
				v, err := resolver.ResolveValue(p.APIKey)
				if v == "" || err != nil {
					if configExists {
						slog.Warn("Skipping Hyper provider due to missing API key", "provider", p.ID)
						c.Providers.Del(string(p.ID))
					}
					continue
				}
			}
		default:
			// if the provider api or endpoint are missing we skip them
			v, err := resolver.ResolveValue(p.APIKey)
			if v == "" || err != nil {
				if configExists {
					slog.Warn("Skipping provider due to missing API key", "provider", p.ID)
					c.Providers.Del(string(p.ID))
				}
				continue
			}
		}
		c.Providers.Set(string(p.ID), prepared)
	}

	// validate the custom providers
	for id, providerConfig := range c.Providers.Seq2() {
		if knownProviderNames[id] {
			continue
		}

		// Make sure the provider ID is set
		providerConfig.ID = id
		providerConfig.Name = cmp.Or(providerConfig.Name, id) // Use ID as name if not set
		// default to OpenAI if not set
		providerConfig.Type = cmp.Or(providerConfig.Type, catwalk.TypeOpenAICompat)
		if !slices.Contains(catwalk.KnownProviderTypes(), providerConfig.Type) && providerConfig.Type != hyperp.Name {
			slog.Warn("Skipping custom provider due to unsupported provider type", "provider", id)
			c.Providers.Del(id)
			continue
		}

		if providerConfig.Disable {
			slog.Debug("Skipping custom provider due to disable flag", "provider", id)
			c.Providers.Del(id)
			continue
		}
		if providerConfig.APIKey == "" {
			slog.Warn("Provider is missing API key, this might be OK for local providers", "provider", id)
		}
		if providerConfig.BaseURL == "" {
			slog.Warn("Skipping custom provider due to missing API endpoint", "provider", id)
			c.Providers.Del(id)
			continue
		}
		if len(providerConfig.Models) == 0 {
			slog.Warn("Skipping custom provider because the provider has no models", "provider", id)
			c.Providers.Del(id)
			continue
		}
		apiKey, err := resolver.ResolveValue(providerConfig.APIKey)
		if apiKey == "" || err != nil {
			slog.Warn("Provider is missing API key, this might be OK for local providers", "provider", id)
		}
		baseURL, err := resolver.ResolveValue(providerConfig.BaseURL)
		if baseURL == "" || err != nil {
			slog.Warn("Skipping custom provider due to missing API endpoint", "provider", id, "error", err)
			c.Providers.Del(id)
			continue
		}

		for k, v := range providerConfig.ExtraHeaders {
			resolved, err := resolver.ResolveValue(v)
			if err != nil {
				slog.Error("Could not resolve provider header", "err", err.Error())
				continue
			}
			providerConfig.ExtraHeaders[k] = resolved
		}

		c.Providers.Set(id, providerConfig)
	}

	if c.Providers.Len() == 0 && c.Options.DisableDefaultProviders {
		return fmt.Errorf("default providers are disabled and there are no custom providers are configured")
	}

	return nil
}

func (c *ProviderConfig) SetupGitHubCopilot() {
	maps.Copy(c.ExtraHeaders, copilot.Headers())
}

func hasAWSCredentials(env env.Env) bool {
	if env.Get("AWS_BEARER_TOKEN_BEDROCK") != "" {
		return true
	}

	if env.Get("AWS_ACCESS_KEY_ID") != "" && env.Get("AWS_SECRET_ACCESS_KEY") != "" {
		return true
	}

	if env.Get("AWS_PROFILE") != "" || env.Get("AWS_DEFAULT_PROFILE") != "" {
		return true
	}

	if env.Get("AWS_REGION") != "" || env.Get("AWS_DEFAULT_REGION") != "" {
		return true
	}

	if env.Get("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		env.Get("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" {
		return true
	}

	if _, err := os.Stat(filepath.Join(home.Dir(), ".aws/credentials")); err == nil && !testing.Testing() {
		return true
	}

	return false
}

// ValidateHooks normalizes event names and checks that every configured
// hook has a command and a syntactically valid matcher regex. Matcher
// compilation used for matching is owned by hooks.Runner; this function
// only validates up front so the user sees config errors at load time
// rather than on the first tool call.
func (c *Config) ValidateHooks() error {
	// Normalize event name keys.
	for event, eventHooks := range c.Hooks {
		canonical := normalizeHookEvent(event)
		if canonical != event {
			c.Hooks[canonical] = append(c.Hooks[canonical], eventHooks...)
			delete(c.Hooks, event)
		}
	}

	for event, eventHooks := range c.Hooks {
		for i, h := range eventHooks {
			if h.Command == "" {
				return fmt.Errorf("hook %s[%d]: command is required", event, i)
			}
			if h.Matcher == "" {
				continue
			}
			if _, err := regexp.Compile(h.Matcher); err != nil {
				return fmt.Errorf("hook %s[%d]: invalid matcher regex %q: %w", event, i, h.Matcher, err)
			}
		}
	}
	return nil
}

// normalizeHookEvent maps user-provided event names to their canonical
// form. Matching is case-insensitive and accepts snake_case variants
// (e.g. "pre_tool_use" → "PreToolUse").
func normalizeHookEvent(name string) string {
	switch strings.ToLower(strings.ReplaceAll(name, "_", "")) {
	case "pretooluse":
		return "PreToolUse"
	default:
		return name
	}
}

// UpdatePreferredModel updates the preferred model for the given type and
// persists it to the config file at the given scope.
func (s *ConfigStore) UpdatePreferredModel(scope Scope, modelType SelectedModelType, model SelectedModel) error {
	s.config.Models[modelType] = model
	if err := s.SetConfigField(scope, fmt.Sprintf("models.%s", modelType), model); err != nil {
		return fmt.Errorf("failed to update preferred model: %w", err)
	}
	if err := s.recordRecentModel(scope, modelType, model); err != nil {
		return err
	}
	return nil
}

// recordRecentModel records a model in the recent models list.
func (s *ConfigStore) recordRecentModel(scope Scope, modelType SelectedModelType, model SelectedModel) error {
	if model.Provider == "" || model.Model == "" {
		return nil
	}

	if s.config.RecentModels == nil {
		s.config.RecentModels = make(map[SelectedModelType][]SelectedModel)
	}

	eq := func(a, b SelectedModel) bool {
		return a.Provider == b.Provider && a.Model == b.Model
	}

	entry := SelectedModel{
		Provider: model.Provider,
		Model:    model.Model,
	}

	current := s.config.RecentModels[modelType]
	withoutCurrent := slices.DeleteFunc(slices.Clone(current), func(existing SelectedModel) bool {
		return eq(existing, entry)
	})

	updated := append([]SelectedModel{entry}, withoutCurrent...)
	if len(updated) > maxRecentModelsPerType {
		updated = updated[:maxRecentModelsPerType]
	}

	if slices.EqualFunc(current, updated, eq) {
		return nil
	}

	s.config.RecentModels[modelType] = updated

	if err := s.SetConfigField(scope, fmt.Sprintf("recent_models.%s", modelType), updated); err != nil {
		return fmt.Errorf("failed to persist recent models: %w", err)
	}

	return nil
}

// RefreshOAuthToken refreshes the OAuth token for the given provider.
func (s *ConfigStore) RefreshOAuthToken(ctx context.Context, scope Scope, providerID string) error {
	providerConfig, exists := s.config.Providers.Get(providerID)
	if !exists {
		return fmt.Errorf("provider %s not found", providerID)
	}

	if providerConfig.OAuthToken == nil {
		return fmt.Errorf("provider %s does not have an OAuth token", providerID)
	}

	var newToken *oauth.Token
	var refreshErr error
	switch providerID {
	case string(catwalk.InferenceProviderCopilot):
		newToken, refreshErr = copilot.RefreshToken(ctx, providerConfig.OAuthToken.RefreshToken)
	case hyperp.Name:
		newToken, refreshErr = hyper.ExchangeToken(ctx, providerConfig.OAuthToken.RefreshToken)
	default:
		return fmt.Errorf("OAuth refresh not supported for provider %s", providerID)
	}
	if refreshErr != nil {
		return fmt.Errorf("failed to refresh OAuth token for provider %s: %w", providerID, refreshErr)
	}

	slog.Info("Successfully refreshed OAuth token", "provider", providerID)
	providerConfig.OAuthToken = newToken
	providerConfig.APIKey = newToken.AccessToken

	switch providerID {
	case string(catwalk.InferenceProviderCopilot):
		providerConfig.SetupGitHubCopilot()
	}

	s.config.Providers.Set(providerID, providerConfig)

	if err := cmp.Or(
		s.SetConfigField(scope, fmt.Sprintf("providers.%s.api_key", providerID), newToken.AccessToken),
		s.SetConfigField(scope, fmt.Sprintf("providers.%s.oauth", providerID), newToken),
	); err != nil {
		return fmt.Errorf("failed to persist refreshed token: %w", err)
	}

	return nil
}

// StalenessResult contains the result of a staleness check.
type StalenessResult struct {
	Dirty   bool
	Changed []string
	Missing []string
	Errors  map[string]error // stat errors by path
}

// ConfigStaleness checks whether any tracked config files have changed on disk
// since the last snapshot. Returns dirty=true if any files changed or went
// missing, along with sorted lists of affected paths. Stat errors are
// captured in Errors map but still treated as non-existence for dirty detection.
func (s *ConfigStore) ConfigStaleness() StalenessResult {
	var result StalenessResult
	result.Errors = make(map[string]error)

	for _, path := range s.trackedConfigPaths {
		snapshot, hadSnapshot := s.snapshots[path]
		current, err := s.snapshotForRef(path)
		if err != nil {
			result.Errors[path] = err
			result.Dirty = true
		}

		if !current.Exists {
			if hadSnapshot && snapshot.Exists {
				result.Missing = append(result.Missing, path)
				result.Dirty = true
			}
			continue
		}

		if !hadSnapshot || !snapshot.Exists {
			result.Changed = append(result.Changed, path)
			result.Dirty = true
			continue
		}

		if current.Source == "db" || snapshot.Source == "db" {
			if snapshot.UpdatedAt != current.UpdatedAt {
				result.Changed = append(result.Changed, path)
				result.Dirty = true
			}
			continue
		}

		if snapshot.Size != current.Size || snapshot.ModTime != current.ModTime {
			result.Changed = append(result.Changed, path)
			result.Dirty = true
		}
	}

	// Sort for deterministic output
	slices.Sort(result.Changed)
	slices.Sort(result.Missing)

	return result
}

// SetRuntimePreferredModel updates the preferred model for the given type and
// persists it to the config file at the given scope.
func (s *ConfigStore) SetRuntimePreferredModel(provider string, model string) error {
	m := s.config.GetModel(provider, model)
	if m == nil {
		return s.buildRuntimePreferredModelNotFoundError(provider, model)
	}
	selectedModel := SelectedModel{
		Provider:        provider,
		Model:           model,
		MaxTokens:       m.DefaultMaxTokens,
		ReasoningEffort: m.DefaultReasoningEffort,
	}

	if err := s.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeLarge, selectedModel); err != nil {
		return err
	}
	knownProvider, err := s.config.GetProvider(s.conn, provider)
	if err != nil {
		return err
	}
	if knownProvider == nil {
		if err := s.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeSmall, selectedModel); err != nil {
			return err
		}
	} else {
		smallModel := knownProvider.DefaultSmallModelID
		sm := s.config.GetModel(provider, smallModel)
		// should never happen
		if sm == nil {
			err = s.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeSmall, selectedModel)
			if err != nil {
				return err
			}
			return nil
		}
		smallSelectedModel := SelectedModel{
			Model:           smallModel,
			Provider:        provider,
			ReasoningEffort: sm.DefaultReasoningEffort,
			MaxTokens:       sm.DefaultMaxTokens,
		}
		err = s.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeSmall, smallSelectedModel)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ConfigStore) buildRuntimePreferredModelNotFoundError(providerID string, modelID string) error {
	loadedProviders := make([]string, 0)
	for id, providerCfg := range s.config.Providers.Seq2() {
		if providerCfg.Disable {
			continue
		}
		loadedProviders = append(loadedProviders, id)
	}
	slices.Sort(loadedProviders)

	catalogProviderFound := false
	catalogModelFound := false
	for _, provider := range s.knownProviders {
		if string(provider.ID) != providerID {
			continue
		}
		catalogProviderFound = true
		for _, model := range provider.Models {
			if model.ID == modelID {
				catalogModelFound = true
				break
			}
		}
		break
	}

	var reason string
	switch {
	case !catalogProviderFound:
		reason = "provider not found in known providers"
	case !catalogModelFound:
		reason = "model not found under provider in known providers"
	default:
		reason = "provider/model exists in known providers but provider was not loaded into runtime config, likely due to missing api_key or provider configuration"
	}

	loadedText := "none"
	if len(loadedProviders) > 0 {
		loadedText = strings.Join(loadedProviders, ", ")
	}

	return fmt.Errorf(
		"SetRuntimePreferredModel: model %s/%s not found; reason: %s; loaded providers: %s",
		providerID,
		modelID,
		reason,
		loadedText,
	)
}

// ImportCopilot attempts to import a GitHub Copilot token from disk.
func (s *ConfigStore) ImportCopilot() (*oauth.Token, bool) {
	if s.HasConfigField(ScopeGlobal, "providers.copilot.api_key") || s.HasConfigField(ScopeGlobal, "providers.copilot.oauth") {
		return nil, false
	}

	diskToken, hasDiskToken := copilot.RefreshTokenFromDisk()
	if !hasDiskToken {
		return nil, false
	}

	slog.Info("Found existing GitHub Copilot token on disk. Authenticating...")
	token, err := copilot.RefreshToken(context.TODO(), diskToken)
	if err != nil {
		slog.Error("Unable to import GitHub Copilot token", "error", err)
		return nil, false
	}

	if err := s.SetProviderAPIKey(ScopeGlobal, string(catwalk.InferenceProviderCopilot), token); err != nil {
		return token, false
	}

	if err := cmp.Or(
		s.SetConfigField(ScopeGlobal, "providers.copilot.api_key", token.AccessToken),
		s.SetConfigField(ScopeGlobal, "providers.copilot.oauth", token),
	); err != nil {
		slog.Error("Unable to save GitHub Copilot token to disk", "error", err)
	}

	slog.Info("GitHub Copilot successfully imported")
	return token, true
}

// SetCompactMode sets the compact mode setting and persists it.
func (s *ConfigStore) SetCompactMode(scope Scope, enabled bool) error {
	if s.config.Options == nil {
		s.config.Options = &Options{}
	}
	s.config.Options.TUI.CompactMode = enabled
	return s.SetConfigField(scope, "options.tui.compact_mode", enabled)
}

// SetProviderAPIKey sets the API key for a provider and persists it.
func (s *ConfigStore) SetProviderAPIKey(scope Scope, providerID string, apiKey any) error {
	var providerConfig ProviderConfig
	var exists bool
	var setKeyOrToken func()

	switch v := apiKey.(type) {
	case string:
		if err := s.SetConfigField(scope, fmt.Sprintf("providers.%s.api_key", providerID), v); err != nil {
			return fmt.Errorf("failed to save api key to config file: %w", err)
		}
		setKeyOrToken = func() { providerConfig.APIKey = v }
	case *oauth.Token:
		if err := cmp.Or(
			s.SetConfigField(scope, fmt.Sprintf("providers.%s.api_key", providerID), v.AccessToken),
			s.SetConfigField(scope, fmt.Sprintf("providers.%s.oauth", providerID), v),
		); err != nil {
			return err
		}
		setKeyOrToken = func() {
			providerConfig.APIKey = v.AccessToken
			providerConfig.OAuthToken = v
			switch providerID {
			case string(catwalk.InferenceProviderCopilot):
				providerConfig.SetupGitHubCopilot()
			}
		}
	}

	providerConfig, exists = s.config.Providers.Get(providerID)
	if exists {
		setKeyOrToken()
		s.config.Providers.Set(providerID, providerConfig)
		return nil
	}

	var foundProvider *catwalk.Provider
	for _, p := range s.knownProviders {
		if string(p.ID) == providerID {
			foundProvider = &p
			break
		}
	}

	if foundProvider != nil {
		providerConfig = ProviderConfig{
			ID:           providerID,
			Name:         foundProvider.Name,
			BaseURL:      foundProvider.APIEndpoint,
			Type:         foundProvider.Type,
			Disable:      false,
			ExtraHeaders: make(map[string]string),
			ExtraParams:  make(map[string]string),
			Models:       foundProvider.Models,
		}
		setKeyOrToken()
	} else {
		return fmt.Errorf("provider with ID %s not found in known providers", providerID)
	}
	s.config.Providers.Set(providerID, providerConfig)
	return nil
}
