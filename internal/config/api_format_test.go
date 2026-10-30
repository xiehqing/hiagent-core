package config

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/stretchr/testify/require"
	"github.com/xiehqing/hiagent-core/internal/env"
)

func TestValidateAPIFormat(t *testing.T) {
	for _, tt := range []struct {
		format APIFormat
		typ    catwalk.Type
		valid  bool
	}{
		{"", "openai-compat", true},
		{APIFormatAuto, "openai", true},
		{APIFormatResponses, "openai-compat", true},
		{APIFormatChatCompletions, "openai", true},
		{"response", "openai-compat", false},
		{APIFormatResponses, "anthropic", false},
	} {
		t.Run(string(tt.typ)+"/"+string(tt.format), func(t *testing.T) {
			err := (ProviderConfig{ID: "relay", Type: tt.typ, APIFormat: tt.format}).ValidateAPIFormat()
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "relay")
			}
		})
	}
}

func TestAPIFormatSurvivesCatalogPreparation(t *testing.T) {
	store := NewTestStore(nil)
	store.Config().Providers.Set("relay", ProviderConfig{
		APIFormat:       APIFormatResponses,
		ProviderOptions: map[string]any{"reasoning_effort": "high"},
	})
	err := store.Config().configureProviders(store, env.New(), IdentityResolver(), []catwalk.Provider{{
		ID: "relay", Type: "openai-compat", APIEndpoint: "https://relay.invalid/v1", APIKey: "test",
		Models: []catwalk.Model{{ID: "gpt-6.1-sol"}},
	}})
	require.NoError(t, err)
	provider, ok := store.Config().Providers.Get("relay")
	require.True(t, ok)
	require.Equal(t, APIFormatResponses, provider.APIFormat)
	require.Equal(t, "high", provider.ProviderOptions["reasoning_effort"])
}

func TestAPIFormatPersistenceAndRuntimeOverride(t *testing.T) {
	conn := openConfigTestDB(t)
	workingDir := t.TempDir()
	_, err := AddDataConfig(conn, DataConfig{WorkingDir: "global", Config: `{
		"providers":{"relay":{"type":"openai-compat","base_url":"https://relay.invalid/v1","api_key":"test","models":[{"id":"gpt-6.1-sol","name":"Sol"}]}}
	}`})
	require.NoError(t, err)
	store, err := Load(workingDir, "", conn, false)
	require.NoError(t, err)
	require.NoError(t, store.SetConfigField(ScopeWorkspace, "providers.relay.api_format", APIFormatChatCompletions))
	provider, _ := store.Config().Providers.Get("relay")
	require.Equal(t, APIFormatChatCompletions, provider.APIFormat)

	reopened, err := Load(workingDir, "", conn, false)
	require.NoError(t, err)
	provider, _ = reopened.Config().Providers.Get("relay")
	require.Equal(t, APIFormatChatCompletions, provider.APIFormat)

	store.Overrides().ProviderAPIFormats = map[string]APIFormat{"relay": APIFormatResponses}
	require.NoError(t, store.ReloadFromDisk(t.Context()))
	provider, _ = store.Config().Providers.Get("relay")
	require.Equal(t, APIFormatResponses, provider.APIFormat)
	// An instance-local SDK override must not alter the stored preference.
	provider, _ = reopened.Config().Providers.Get("relay")
	require.Equal(t, APIFormatChatCompletions, provider.APIFormat)
}
