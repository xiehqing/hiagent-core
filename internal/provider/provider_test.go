package provider

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xiehqing/hiagent-core/internal/db"
	_ "modernc.org/sqlite"
)

func TestProviderServiceCRUD(t *testing.T) {
	t.Parallel()

	conn, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	_, err = conn.Exec(`
CREATE TABLE providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    api_endpoint TEXT,
    api_key TEXT,
    default_large_model_id TEXT,
    default_small_model_id TEXT,
    default_headers TEXT,
    disabled INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE TABLE big_models (
    provider_id TEXT NOT NULL,
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    cost_per_1m_in REAL NOT NULL DEFAULT 0,
    cost_per_1m_out REAL NOT NULL DEFAULT 0,
    cost_per_1m_in_cached REAL NOT NULL DEFAULT 0,
    cost_per_1m_out_cached REAL NOT NULL DEFAULT 0,
    context_window INTEGER NOT NULL DEFAULT 0,
    default_max_tokens INTEGER NOT NULL DEFAULT 0,
    can_reason INTEGER NOT NULL DEFAULT 0,
    reasoning_levels TEXT,
    default_reasoning_effort TEXT,
    supports_images INTEGER NOT NULL DEFAULT 0,
    disabled INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (provider_id, id),
    FOREIGN KEY (provider_id) REFERENCES providers (id) ON DELETE CASCADE
);
`)
	require.NoError(t, err)

	svc := NewService(db.New(conn))
	ctx := context.Background()

	created, err := svc.Create(ctx, Provider{
		ID:                  "openai-local",
		Name:                "OpenAI Local",
		Type:                "openai-compat",
		APIEndpoint:         "http://localhost:11434/v1",
		APIKey:              "token",
		DefaultLargeModelID: "gpt-large",
		DefaultSmallModelID: "gpt-small",
		DefaultHeaders: map[string]string{
			"X-Test": "1",
		},
		SortOrder: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "openai-local", created.ID)

	model, err := svc.CreateModel(ctx, BigModel{
		ProviderID:             "openai-local",
		ID:                     "gpt-large",
		Name:                   "GPT Large",
		ContextWindow:          128000,
		DefaultMaxTokens:       8192,
		CanReason:              true,
		ReasoningLevels:        []string{"low", "high"},
		DefaultReasoningEffort: "high",
		SupportsImages:         true,
	})
	require.NoError(t, err)
	require.Equal(t, "gpt-large", model.ID)

	_, err = svc.CreateModel(ctx, BigModel{
		ProviderID:       "openai-local",
		ID:               "gpt-small",
		Name:             "GPT Small",
		ContextWindow:    64000,
		DefaultMaxTokens: 4096,
		SortOrder:        2,
	})
	require.NoError(t, err)

	got, err := svc.Get(ctx, "openai-local")
	require.NoError(t, err)
	require.Len(t, got.Models, 2)
	require.Equal(t, []string{"low", "high"}, got.Models[0].ReasoningLevels)

	saved, err := svc.Save(ctx, Provider{
		ID:                  "openai-local",
		Name:                "OpenAI Edge",
		Type:                "openai-compat",
		APIEndpoint:         "http://127.0.0.1:11434/v1",
		APIKey:              "token-2",
		DefaultLargeModelID: "gpt-large",
		DefaultSmallModelID: "gpt-small",
		DefaultHeaders: map[string]string{
			"X-Test": "2",
		},
		Disabled:  true,
		SortOrder: 2,
	})
	require.NoError(t, err)
	require.Equal(t, "OpenAI Edge", saved.Name)
	require.True(t, saved.Disabled)

	updatedModel, err := svc.SaveModel(ctx, BigModel{
		ProviderID:             "openai-local",
		ID:                     "gpt-small",
		Name:                   "GPT Small v2",
		ContextWindow:          96000,
		DefaultMaxTokens:       6144,
		CanReason:              true,
		ReasoningLevels:        []string{"medium"},
		DefaultReasoningEffort: "medium",
		SupportsImages:         true,
		SortOrder:              3,
	})
	require.NoError(t, err)
	require.Equal(t, "GPT Small v2", updatedModel.Name)

	models, err := svc.ListModels(ctx, "openai-local")
	require.NoError(t, err)
	require.Len(t, models, 2)

	err = svc.DeleteModel(ctx, "openai-local", "gpt-small")
	require.NoError(t, err)

	all, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Len(t, all[0].Models, 1)

	err = svc.Delete(ctx, "openai-local")
	require.NoError(t, err)

	all, err = svc.List(ctx)
	require.NoError(t, err)
	require.Empty(t, all)
}
