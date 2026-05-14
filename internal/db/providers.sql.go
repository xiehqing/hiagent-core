package db

import (
	"context"
	"database/sql"
)

const createProvider = `-- name: CreateProvider :one
INSERT INTO providers (
    id,
    name,
    type,
    api_endpoint,
    api_key,
    default_large_model_id,
    default_small_model_id,
    default_headers,
    disabled,
    sort_order,
    created_at,
    updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now')
)
RETURNING id, name, type, api_endpoint, api_key, default_large_model_id, default_small_model_id, default_headers, disabled, sort_order, created_at, updated_at
`

type CreateProviderParams struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	APIEndpoint         sql.NullString `json:"api_endpoint"`
	APIKey              sql.NullString `json:"api_key"`
	DefaultLargeModelID sql.NullString `json:"default_large_model_id"`
	DefaultSmallModelID sql.NullString `json:"default_small_model_id"`
	DefaultHeaders      sql.NullString `json:"default_headers"`
	Disabled            bool           `json:"disabled"`
	SortOrder           int64          `json:"sort_order"`
}

func (q *Queries) CreateProvider(ctx context.Context, arg CreateProviderParams) (Provider, error) {
	if q.dialect == dialectMySQL {
		_, err := q.exec(ctx, nil, createProvider,
			arg.ID,
			arg.Name,
			arg.Type,
			arg.APIEndpoint,
			arg.APIKey,
			arg.DefaultLargeModelID,
			arg.DefaultSmallModelID,
			arg.DefaultHeaders,
			arg.Disabled,
			arg.SortOrder,
		)
		if err != nil {
			return Provider{}, err
		}
		return q.GetProvider(ctx, arg.ID)
	}

	row := q.queryRow(ctx, nil, createProvider,
		arg.ID,
		arg.Name,
		arg.Type,
		arg.APIEndpoint,
		arg.APIKey,
		arg.DefaultLargeModelID,
		arg.DefaultSmallModelID,
		arg.DefaultHeaders,
		arg.Disabled,
		arg.SortOrder,
	)
	var i Provider
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Type,
		&i.APIEndpoint,
		&i.APIKey,
		&i.DefaultLargeModelID,
		&i.DefaultSmallModelID,
		&i.DefaultHeaders,
		&i.Disabled,
		&i.SortOrder,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const deleteProvider = `-- name: DeleteProvider :exec
DELETE FROM providers
WHERE id = ?
`

func (q *Queries) DeleteProvider(ctx context.Context, id string) error {
	_, err := q.exec(ctx, nil, deleteProvider, id)
	return err
}

const getProvider = `-- name: GetProvider :one
SELECT id, name, type, api_endpoint, api_key, default_large_model_id, default_small_model_id, default_headers, disabled, sort_order, created_at, updated_at
FROM providers
WHERE id = ? LIMIT 1
`

func (q *Queries) GetProvider(ctx context.Context, id string) (Provider, error) {
	row := q.queryRow(ctx, nil, getProvider, id)
	var i Provider
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Type,
		&i.APIEndpoint,
		&i.APIKey,
		&i.DefaultLargeModelID,
		&i.DefaultSmallModelID,
		&i.DefaultHeaders,
		&i.Disabled,
		&i.SortOrder,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const listProviders = `-- name: ListProviders :many
SELECT id, name, type, api_endpoint, api_key, default_large_model_id, default_small_model_id, default_headers, disabled, sort_order, created_at, updated_at
FROM providers
ORDER BY sort_order ASC, id ASC
`

func (q *Queries) ListProviders(ctx context.Context) ([]Provider, error) {
	rows, err := q.query(ctx, nil, listProviders)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Provider{}
	for rows.Next() {
		var i Provider
		if err := rows.Scan(
			&i.ID,
			&i.Name,
			&i.Type,
			&i.APIEndpoint,
			&i.APIKey,
			&i.DefaultLargeModelID,
			&i.DefaultSmallModelID,
			&i.DefaultHeaders,
			&i.Disabled,
			&i.SortOrder,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const updateProvider = `-- name: UpdateProvider :exec
UPDATE providers
SET
    name = ?,
    type = ?,
    api_endpoint = ?,
    api_key = ?,
    default_large_model_id = ?,
    default_small_model_id = ?,
    default_headers = ?,
    disabled = ?,
    sort_order = ?
WHERE id = ?
`

type UpdateProviderParams struct {
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	APIEndpoint         sql.NullString `json:"api_endpoint"`
	APIKey              sql.NullString `json:"api_key"`
	DefaultLargeModelID sql.NullString `json:"default_large_model_id"`
	DefaultSmallModelID sql.NullString `json:"default_small_model_id"`
	DefaultHeaders      sql.NullString `json:"default_headers"`
	Disabled            bool           `json:"disabled"`
	SortOrder           int64          `json:"sort_order"`
	ID                  string         `json:"id"`
}

func (q *Queries) UpdateProvider(ctx context.Context, arg UpdateProviderParams) (Provider, error) {
	_, err := q.exec(ctx, nil, updateProvider,
		arg.Name,
		arg.Type,
		arg.APIEndpoint,
		arg.APIKey,
		arg.DefaultLargeModelID,
		arg.DefaultSmallModelID,
		arg.DefaultHeaders,
		arg.Disabled,
		arg.SortOrder,
		arg.ID,
	)
	if err != nil {
		return Provider{}, err
	}
	return q.GetProvider(ctx, arg.ID)
}

const createBigModel = `-- name: CreateBigModel :one
INSERT INTO big_models (
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
    reasoning_levels,
    default_reasoning_effort,
    supports_images,
    disabled,
    sort_order,
    created_at,
    updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now')
)
RETURNING provider_id, id, name, cost_per_1m_in, cost_per_1m_out, cost_per_1m_in_cached, cost_per_1m_out_cached, context_window, default_max_tokens, can_reason, reasoning_levels, default_reasoning_effort, supports_images, disabled, sort_order, created_at, updated_at
`

type CreateBigModelParams struct {
	ProviderID             string         `json:"provider_id"`
	ID                     string         `json:"id"`
	Name                   string         `json:"name"`
	CostPer1MIn            float64        `json:"cost_per_1m_in"`
	CostPer1MOut           float64        `json:"cost_per_1m_out"`
	CostPer1MInCached      float64        `json:"cost_per_1m_in_cached"`
	CostPer1MOutCached     float64        `json:"cost_per_1m_out_cached"`
	ContextWindow          int64          `json:"context_window"`
	DefaultMaxTokens       int64          `json:"default_max_tokens"`
	CanReason              bool           `json:"can_reason"`
	ReasoningLevels        sql.NullString `json:"reasoning_levels"`
	DefaultReasoningEffort sql.NullString `json:"default_reasoning_effort"`
	SupportsImages         bool           `json:"supports_images"`
	Disabled               bool           `json:"disabled"`
	SortOrder              int64          `json:"sort_order"`
}

func (q *Queries) CreateBigModel(ctx context.Context, arg CreateBigModelParams) (BigModel, error) {
	if q.dialect == dialectMySQL {
		_, err := q.exec(ctx, nil, createBigModel,
			arg.ProviderID,
			arg.ID,
			arg.Name,
			arg.CostPer1MIn,
			arg.CostPer1MOut,
			arg.CostPer1MInCached,
			arg.CostPer1MOutCached,
			arg.ContextWindow,
			arg.DefaultMaxTokens,
			arg.CanReason,
			arg.ReasoningLevels,
			arg.DefaultReasoningEffort,
			arg.SupportsImages,
			arg.Disabled,
			arg.SortOrder,
		)
		if err != nil {
			return BigModel{}, err
		}
		return q.GetBigModel(ctx, GetBigModelParams{ProviderID: arg.ProviderID, ID: arg.ID})
	}

	row := q.queryRow(ctx, nil, createBigModel,
		arg.ProviderID,
		arg.ID,
		arg.Name,
		arg.CostPer1MIn,
		arg.CostPer1MOut,
		arg.CostPer1MInCached,
		arg.CostPer1MOutCached,
		arg.ContextWindow,
		arg.DefaultMaxTokens,
		arg.CanReason,
		arg.ReasoningLevels,
		arg.DefaultReasoningEffort,
		arg.SupportsImages,
		arg.Disabled,
		arg.SortOrder,
	)
	var i BigModel
	err := row.Scan(
		&i.ProviderID,
		&i.ID,
		&i.Name,
		&i.CostPer1MIn,
		&i.CostPer1MOut,
		&i.CostPer1MInCached,
		&i.CostPer1MOutCached,
		&i.ContextWindow,
		&i.DefaultMaxTokens,
		&i.CanReason,
		&i.ReasoningLevels,
		&i.DefaultReasoningEffort,
		&i.SupportsImages,
		&i.Disabled,
		&i.SortOrder,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const deleteBigModel = `-- name: DeleteBigModel :exec
DELETE FROM big_models
WHERE provider_id = ? AND id = ?
`

type DeleteBigModelParams struct {
	ProviderID string `json:"provider_id"`
	ID         string `json:"id"`
}

func (q *Queries) DeleteBigModel(ctx context.Context, arg DeleteBigModelParams) error {
	_, err := q.exec(ctx, nil, deleteBigModel, arg.ProviderID, arg.ID)
	return err
}

const getBigModel = `-- name: GetBigModel :one
SELECT provider_id, id, name, cost_per_1m_in, cost_per_1m_out, cost_per_1m_in_cached, cost_per_1m_out_cached, context_window, default_max_tokens, can_reason, reasoning_levels, default_reasoning_effort, supports_images, disabled, sort_order, created_at, updated_at
FROM big_models
WHERE provider_id = ? AND id = ?
LIMIT 1
`

type GetBigModelParams struct {
	ProviderID string `json:"provider_id"`
	ID         string `json:"id"`
}

func (q *Queries) GetBigModel(ctx context.Context, arg GetBigModelParams) (BigModel, error) {
	row := q.queryRow(ctx, nil, getBigModel, arg.ProviderID, arg.ID)
	var i BigModel
	err := row.Scan(
		&i.ProviderID,
		&i.ID,
		&i.Name,
		&i.CostPer1MIn,
		&i.CostPer1MOut,
		&i.CostPer1MInCached,
		&i.CostPer1MOutCached,
		&i.ContextWindow,
		&i.DefaultMaxTokens,
		&i.CanReason,
		&i.ReasoningLevels,
		&i.DefaultReasoningEffort,
		&i.SupportsImages,
		&i.Disabled,
		&i.SortOrder,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const listBigModels = `-- name: ListBigModels :many
SELECT provider_id, id, name, cost_per_1m_in, cost_per_1m_out, cost_per_1m_in_cached, cost_per_1m_out_cached, context_window, default_max_tokens, can_reason, reasoning_levels, default_reasoning_effort, supports_images, disabled, sort_order, created_at, updated_at
FROM big_models
ORDER BY provider_id ASC, sort_order ASC, id ASC
`

func (q *Queries) ListBigModels(ctx context.Context) ([]BigModel, error) {
	rows, err := q.query(ctx, nil, listBigModels)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []BigModel{}
	for rows.Next() {
		var i BigModel
		if err := rows.Scan(
			&i.ProviderID,
			&i.ID,
			&i.Name,
			&i.CostPer1MIn,
			&i.CostPer1MOut,
			&i.CostPer1MInCached,
			&i.CostPer1MOutCached,
			&i.ContextWindow,
			&i.DefaultMaxTokens,
			&i.CanReason,
			&i.ReasoningLevels,
			&i.DefaultReasoningEffort,
			&i.SupportsImages,
			&i.Disabled,
			&i.SortOrder,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const listBigModelsByProvider = `-- name: ListBigModelsByProvider :many
SELECT provider_id, id, name, cost_per_1m_in, cost_per_1m_out, cost_per_1m_in_cached, cost_per_1m_out_cached, context_window, default_max_tokens, can_reason, reasoning_levels, default_reasoning_effort, supports_images, disabled, sort_order, created_at, updated_at
FROM big_models
WHERE provider_id = ?
ORDER BY sort_order ASC, id ASC
`

func (q *Queries) ListBigModelsByProvider(ctx context.Context, providerID string) ([]BigModel, error) {
	rows, err := q.query(ctx, nil, listBigModelsByProvider, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []BigModel{}
	for rows.Next() {
		var i BigModel
		if err := rows.Scan(
			&i.ProviderID,
			&i.ID,
			&i.Name,
			&i.CostPer1MIn,
			&i.CostPer1MOut,
			&i.CostPer1MInCached,
			&i.CostPer1MOutCached,
			&i.ContextWindow,
			&i.DefaultMaxTokens,
			&i.CanReason,
			&i.ReasoningLevels,
			&i.DefaultReasoningEffort,
			&i.SupportsImages,
			&i.Disabled,
			&i.SortOrder,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const updateBigModel = `-- name: UpdateBigModel :exec
UPDATE big_models
SET
    name = ?,
    cost_per_1m_in = ?,
    cost_per_1m_out = ?,
    cost_per_1m_in_cached = ?,
    cost_per_1m_out_cached = ?,
    context_window = ?,
    default_max_tokens = ?,
    can_reason = ?,
    reasoning_levels = ?,
    default_reasoning_effort = ?,
    supports_images = ?,
    disabled = ?,
    sort_order = ?
WHERE provider_id = ? AND id = ?
`

type UpdateBigModelParams struct {
	Name                   string         `json:"name"`
	CostPer1MIn            float64        `json:"cost_per_1m_in"`
	CostPer1MOut           float64        `json:"cost_per_1m_out"`
	CostPer1MInCached      float64        `json:"cost_per_1m_in_cached"`
	CostPer1MOutCached     float64        `json:"cost_per_1m_out_cached"`
	ContextWindow          int64          `json:"context_window"`
	DefaultMaxTokens       int64          `json:"default_max_tokens"`
	CanReason              bool           `json:"can_reason"`
	ReasoningLevels        sql.NullString `json:"reasoning_levels"`
	DefaultReasoningEffort sql.NullString `json:"default_reasoning_effort"`
	SupportsImages         bool           `json:"supports_images"`
	Disabled               bool           `json:"disabled"`
	SortOrder              int64          `json:"sort_order"`
	ProviderID             string         `json:"provider_id"`
	ID                     string         `json:"id"`
}

func (q *Queries) UpdateBigModel(ctx context.Context, arg UpdateBigModelParams) (BigModel, error) {
	_, err := q.exec(ctx, nil, updateBigModel,
		arg.Name,
		arg.CostPer1MIn,
		arg.CostPer1MOut,
		arg.CostPer1MInCached,
		arg.CostPer1MOutCached,
		arg.ContextWindow,
		arg.DefaultMaxTokens,
		arg.CanReason,
		arg.ReasoningLevels,
		arg.DefaultReasoningEffort,
		arg.SupportsImages,
		arg.Disabled,
		arg.SortOrder,
		arg.ProviderID,
		arg.ID,
	)
	if err != nil {
		return BigModel{}, err
	}
	return q.GetBigModel(ctx, GetBigModelParams{ProviderID: arg.ProviderID, ID: arg.ID})
}
