package appsdk

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xiehqing/hiagent-core/internal/config"
)

func TestWithProviderAPIFormat(t *testing.T) {
	store := config.NewTestStore(nil)
	store.Config().Providers.Set("relay", config.ProviderConfig{ID: "relay", Type: "openai-compat"})
	options := &Options{}
	WithProviderAPIFormat("relay", APIFormatResponses)(options)
	require.NoError(t, applyAppConfigOptions(store, options.cfg))
	provider, _ := store.Config().Providers.Get("relay")
	require.Equal(t, APIFormatResponses, provider.APIFormat)
	require.Equal(t, config.APIFormatResponses, store.Overrides().ProviderAPIFormats["relay"])
	options.cfg.ProviderAPIFormats["relay"] = APIFormatChatCompletions
	require.Equal(t, config.APIFormatResponses, store.Overrides().ProviderAPIFormats["relay"])

	WithProviderAPIFormat("missing", APIFormatResponses)(options)
	require.ErrorContains(t, applyAppConfigOptions(store, options.cfg), "missing")
}
