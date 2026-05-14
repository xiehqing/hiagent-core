package config

import (
	"charm.land/catwalk/pkg/catwalk"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// DBProviders 从数据库中获取提供商
func DBProviders(db *sql.DB) ([]catwalk.Provider, error) {
	if db == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	providers, err := loadDBProviders(ctx, db)
	if err != nil {
		if isMissingProviderCatalogError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load database providers: %w", err)
	}
	if len(providers) > 0 {
		slog.Info("Loaded providers from database", "count", len(providers))
	}
	return providers, nil
}

const providerQuery = `
	SELECT
		id,
		name,
		type,
		COALESCE(api_endpoint, ''),
		COALESCE(api_key, ''),
		COALESCE(default_large_model_id, ''),
		COALESCE(default_small_model_id, ''),
		COALESCE(default_headers, '{}')
		FROM providers
	WHERE disabled = 0
	ORDER BY sort_order ASC, id ASC
`

const modelQuery = `
	SELECT
		provider_id,
		id,
		name,
		cost_per_1m_in,
		cost_per_1m_out,
		cost_per_1m_in_cached,
		cost_per_1m_out_cached,
		context_window,
		default_max_tokens,
		can_reason,
		COALESCE(reasoning_levels, '[]'),
		COALESCE(default_reasoning_effort, ''),
		supports_images
		FROM big_models
	WHERE disabled = 0
	ORDER BY provider_id ASC, sort_order ASC, id ASC
`

func loadDBProviders(ctx context.Context, db *sql.DB) ([]catwalk.Provider, error) {
	rows, err := db.QueryContext(ctx, providerQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := make([]catwalk.Provider, 0)
	indexByID := make(map[string]int)
	for rows.Next() {
		var (
			id                  string
			name                string
			typ                 string
			apiEndpoint         string
			apiKey              string
			defaultLargeModelID string
			defaultSmallModelID string
			defaultHeadersJSON  string
		)
		if err := rows.Scan(
			&id,
			&name,
			&typ,
			&apiEndpoint,
			&apiKey,
			&defaultLargeModelID,
			&defaultSmallModelID,
			&defaultHeadersJSON,
		); err != nil {
			return nil, err
		}

		defaultHeaders := make(map[string]string)
		if defaultHeadersJSON != "" {
			if err := json.Unmarshal([]byte(defaultHeadersJSON), &defaultHeaders); err != nil {
				return nil, fmt.Errorf("failed to unmarshal provider %s default headers: %w", id, err)
			}
		}
		if name == "" {
			name = id
		}

		indexByID[id] = len(providers)
		providers = append(providers, catwalk.Provider{
			Name:                name,
			ID:                  catwalk.InferenceProvider(id),
			APIKey:              apiKey,
			APIEndpoint:         apiEndpoint,
			Type:                catwalk.Type(typ),
			DefaultLargeModelID: defaultLargeModelID,
			DefaultSmallModelID: defaultSmallModelID,
			DefaultHeaders:      defaultHeaders,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, nil
	}
	rows, err = db.QueryContext(ctx, modelQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			providerID             string
			modelID                string
			name                   string
			costPer1MIn            float64
			costPer1MOut           float64
			costPer1MInCached      float64
			costPer1MOutCached     float64
			contextWindow          int64
			defaultMaxTokens       int64
			canReason              bool
			reasoningLevelsJSON    string
			defaultReasoningEffort string
			supportsImages         bool
		)
		if err := rows.Scan(
			&providerID,
			&modelID,
			&name,
			&costPer1MIn,
			&costPer1MOut,
			&costPer1MInCached,
			&costPer1MOutCached,
			&contextWindow,
			&defaultMaxTokens,
			&canReason,
			&reasoningLevelsJSON,
			&defaultReasoningEffort,
			&supportsImages,
		); err != nil {
			return nil, err
		}

		idx, ok := indexByID[providerID]
		if !ok {
			slog.Warn("Skipping database model for unknown provider", "provider", providerID, "model", modelID)
			continue
		}

		reasoningLevels := []string{}
		if reasoningLevelsJSON != "" {
			if err := json.Unmarshal([]byte(reasoningLevelsJSON), &reasoningLevels); err != nil {
				return nil, fmt.Errorf("failed to unmarshal model %s reasoning levels: %w", modelID, err)
			}
		}
		if name == "" {
			name = modelID
		}

		providers[idx].Models = append(providers[idx].Models, catwalk.Model{
			ID:                     modelID,
			Name:                   name,
			CostPer1MIn:            costPer1MIn,
			CostPer1MOut:           costPer1MOut,
			CostPer1MInCached:      costPer1MInCached,
			CostPer1MOutCached:     costPer1MOutCached,
			ContextWindow:          contextWindow,
			DefaultMaxTokens:       defaultMaxTokens,
			CanReason:              canReason,
			ReasoningLevels:        reasoningLevels,
			DefaultReasoningEffort: defaultReasoningEffort,
			SupportsImages:         supportsImages,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	filtered := make([]catwalk.Provider, 0, len(providers))
	for _, provider := range providers {
		if len(provider.Models) == 0 {
			slog.Warn("Skipping database provider because it has no models", "provider", provider.ID)
			continue
		}
		if !providerHasModel(provider, provider.DefaultLargeModelID) {
			provider.DefaultLargeModelID = provider.Models[0].ID
		}
		if !providerHasModel(provider, provider.DefaultSmallModelID) {
			provider.DefaultSmallModelID = provider.Models[0].ID
		}
		filtered = append(filtered, provider)
	}

	return filtered, nil
}

func providerHasModel(provider catwalk.Provider, modelID string) bool {
	if modelID == "" {
		return false
	}
	for _, model := range provider.Models {
		if model.ID == modelID {
			return true
		}
	}
	return false
}

func isMissingProviderCatalogError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table: providers") ||
		strings.Contains(msg, "no such table: big_models") ||
		strings.Contains(msg, "table providers doesn't exist") ||
		strings.Contains(msg, "table big_models doesn't exist") ||
		strings.Contains(msg, "unknown table 'providers'") ||
		strings.Contains(msg, "unknown table 'big_models'")
}
