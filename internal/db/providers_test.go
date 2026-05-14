package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderManagementQueries(t *testing.T) {
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

	q := New(conn)
	ctx := context.Background()

	provider, err := q.CreateProvider(ctx, CreateProviderParams{
		ID:                  "openai-local",
		Name:                "OpenAI Local",
		Type:                "openai-compat",
		APIEndpoint:         sql.NullString{String: "http://localhost:11434/v1", Valid: true},
		APIKey:              sql.NullString{String: "token", Valid: true},
		DefaultLargeModelID: sql.NullString{String: "gpt-large", Valid: true},
		DefaultSmallModelID: sql.NullString{String: "gpt-small", Valid: true},
		DefaultHeaders:      sql.NullString{String: `{"X-Test":"1"}`, Valid: true},
		SortOrder:           10,
	})
	require.NoError(t, err)
	require.Equal(t, "openai-local", provider.ID)

	model, err := q.CreateBigModel(ctx, CreateBigModelParams{
		ProviderID:             "openai-local",
		ID:                     "gpt-large",
		Name:                   "GPT Large",
		CostPer1MIn:            1.2,
		CostPer1MOut:           2.4,
		ContextWindow:          128000,
		DefaultMaxTokens:       8192,
		CanReason:              true,
		ReasoningLevels:        sql.NullString{String: `["low","high"]`, Valid: true},
		DefaultReasoningEffort: sql.NullString{String: "high", Valid: true},
		SupportsImages:         true,
		SortOrder:              1,
	})
	require.NoError(t, err)
	require.Equal(t, "gpt-large", model.ID)

	_, err = q.CreateBigModel(ctx, CreateBigModelParams{
		ProviderID:       "openai-local",
		ID:               "gpt-small",
		Name:             "GPT Small",
		ContextWindow:    64000,
		DefaultMaxTokens: 4096,
		SortOrder:        2,
	})
	require.NoError(t, err)

	providers, err := q.ListProviders(ctx)
	require.NoError(t, err)
	require.Len(t, providers, 1)

	models, err := q.ListBigModelsByProvider(ctx, "openai-local")
	require.NoError(t, err)
	require.Len(t, models, 2)

	updatedProvider, err := q.UpdateProvider(ctx, UpdateProviderParams{
		ID:                  "openai-local",
		Name:                "OpenAI Edge",
		Type:                "openai-compat",
		APIEndpoint:         sql.NullString{String: "http://127.0.0.1:11434/v1", Valid: true},
		APIKey:              sql.NullString{String: "token-2", Valid: true},
		DefaultLargeModelID: sql.NullString{String: "gpt-large", Valid: true},
		DefaultSmallModelID: sql.NullString{String: "gpt-small", Valid: true},
		DefaultHeaders:      sql.NullString{String: `{"X-Test":"2"}`, Valid: true},
		Disabled:            true,
		SortOrder:           20,
	})
	require.NoError(t, err)
	require.Equal(t, "OpenAI Edge", updatedProvider.Name)
	require.True(t, updatedProvider.Disabled)

	updatedModel, err := q.UpdateBigModel(ctx, UpdateBigModelParams{
		ProviderID:             "openai-local",
		ID:                     "gpt-small",
		Name:                   "GPT Small v2",
		CostPer1MIn:            0.2,
		CostPer1MOut:           0.3,
		ContextWindow:          96000,
		DefaultMaxTokens:       6144,
		CanReason:              true,
		ReasoningLevels:        sql.NullString{String: `["medium"]`, Valid: true},
		DefaultReasoningEffort: sql.NullString{String: "medium", Valid: true},
		SupportsImages:         true,
		SortOrder:              3,
	})
	require.NoError(t, err)
	require.Equal(t, "GPT Small v2", updatedModel.Name)
	require.True(t, updatedModel.CanReason)

	err = q.DeleteBigModel(ctx, DeleteBigModelParams{ProviderID: "openai-local", ID: "gpt-small"})
	require.NoError(t, err)

	models, err = q.ListBigModels(ctx)
	require.NoError(t, err)
	require.Len(t, models, 1)

	err = q.DeleteProvider(ctx, "openai-local")
	require.NoError(t, err)

	providers, err = q.ListProviders(ctx)
	require.NoError(t, err)
	require.Empty(t, providers)
}
