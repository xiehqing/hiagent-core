package provider

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/xiehqing/hiagent-core/internal/db"
	"github.com/xiehqing/hiagent-core/internal/pubsub"
)

type Provider struct {
	ID                  string
	Name                string
	Type                string
	APIEndpoint         string
	APIKey              string
	DefaultLargeModelID string
	DefaultSmallModelID string
	DefaultHeaders      map[string]string
	Disabled            bool
	SortOrder           int64
	Models              []BigModel
	CreatedAt           int64
	UpdatedAt           int64
}

type BigModel struct {
	ProviderID             string
	ID                     string
	Name                   string
	CostPer1MIn            float64
	CostPer1MOut           float64
	CostPer1MInCached      float64
	CostPer1MOutCached     float64
	ContextWindow          int64
	DefaultMaxTokens       int64
	CanReason              bool
	ReasoningLevels        []string
	DefaultReasoningEffort string
	SupportsImages         bool
	Disabled               bool
	SortOrder              int64
	CreatedAt              int64
	UpdatedAt              int64
}

type Service interface {
	pubsub.Subscriber[Provider]
	Create(ctx context.Context, provider Provider) (Provider, error)
	Get(ctx context.Context, id string) (Provider, error)
	List(ctx context.Context) ([]Provider, error)
	Save(ctx context.Context, provider Provider) (Provider, error)
	Delete(ctx context.Context, id string) error

	CreateModel(ctx context.Context, model BigModel) (BigModel, error)
	GetModel(ctx context.Context, providerID, modelID string) (BigModel, error)
	ListModels(ctx context.Context, providerID string) ([]BigModel, error)
	ListAllModels(ctx context.Context) ([]BigModel, error)
	SaveModel(ctx context.Context, model BigModel) (BigModel, error)
	DeleteModel(ctx context.Context, providerID, modelID string) error
}

type service struct {
	*pubsub.Broker[Provider]
	q db.Querier
}

func NewService(q db.Querier) Service {
	return &service{
		Broker: pubsub.NewBroker[Provider](),
		q:      q,
	}
}

func (s *service) Create(ctx context.Context, provider Provider) (Provider, error) {
	dbProvider, err := s.q.CreateProvider(ctx, db.CreateProviderParams{
		ID:                  provider.ID,
		Name:                provider.Name,
		Type:                provider.Type,
		APIEndpoint:         toNullString(provider.APIEndpoint),
		APIKey:              toNullString(provider.APIKey),
		DefaultLargeModelID: toNullString(provider.DefaultLargeModelID),
		DefaultSmallModelID: toNullString(provider.DefaultSmallModelID),
		DefaultHeaders:      marshalHeaders(provider.DefaultHeaders),
		Disabled:            provider.Disabled,
		SortOrder:           provider.SortOrder,
	})
	if err != nil {
		return Provider{}, err
	}
	created := s.fromDBItem(dbProvider)
	s.Publish(pubsub.CreatedEvent, created)
	return created, nil
}

func (s *service) Get(ctx context.Context, id string) (Provider, error) {
	dbProvider, err := s.q.GetProvider(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	provider := s.fromDBItem(dbProvider)
	models, err := s.ListModels(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	provider.Models = models
	return provider, nil
}

func (s *service) List(ctx context.Context) ([]Provider, error) {
	dbProviders, err := s.q.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	providers := make([]Provider, len(dbProviders))
	for i, item := range dbProviders {
		provider := s.fromDBItem(item)
		models, err := s.ListModels(ctx, provider.ID)
		if err != nil {
			return nil, err
		}
		provider.Models = models
		providers[i] = provider
	}
	return providers, nil
}

func (s *service) Save(ctx context.Context, provider Provider) (Provider, error) {
	dbProvider, err := s.q.UpdateProvider(ctx, db.UpdateProviderParams{
		ID:                  provider.ID,
		Name:                provider.Name,
		Type:                provider.Type,
		APIEndpoint:         toNullString(provider.APIEndpoint),
		APIKey:              toNullString(provider.APIKey),
		DefaultLargeModelID: toNullString(provider.DefaultLargeModelID),
		DefaultSmallModelID: toNullString(provider.DefaultSmallModelID),
		DefaultHeaders:      marshalHeaders(provider.DefaultHeaders),
		Disabled:            provider.Disabled,
		SortOrder:           provider.SortOrder,
	})
	if err != nil {
		return Provider{}, err
	}
	saved := s.fromDBItem(dbProvider)
	models, err := s.ListModels(ctx, provider.ID)
	if err != nil {
		return Provider{}, err
	}
	saved.Models = models
	s.Publish(pubsub.UpdatedEvent, saved)
	return saved, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	provider, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.q.DeleteProvider(ctx, id); err != nil {
		return err
	}
	s.Publish(pubsub.DeletedEvent, provider)
	return nil
}

func (s *service) CreateModel(ctx context.Context, model BigModel) (BigModel, error) {
	dbModel, err := s.q.CreateBigModel(ctx, db.CreateBigModelParams{
		ProviderID:             model.ProviderID,
		ID:                     model.ID,
		Name:                   model.Name,
		CostPer1MIn:            model.CostPer1MIn,
		CostPer1MOut:           model.CostPer1MOut,
		CostPer1MInCached:      model.CostPer1MInCached,
		CostPer1MOutCached:     model.CostPer1MOutCached,
		ContextWindow:          model.ContextWindow,
		DefaultMaxTokens:       model.DefaultMaxTokens,
		CanReason:              model.CanReason,
		ReasoningLevels:        marshalReasoningLevels(model.ReasoningLevels),
		DefaultReasoningEffort: toNullString(model.DefaultReasoningEffort),
		SupportsImages:         model.SupportsImages,
		Disabled:               model.Disabled,
		SortOrder:              model.SortOrder,
	})
	if err != nil {
		return BigModel{}, err
	}
	return fromDBModel(dbModel), nil
}

func (s *service) GetModel(ctx context.Context, providerID, modelID string) (BigModel, error) {
	dbModel, err := s.q.GetBigModel(ctx, db.GetBigModelParams{
		ProviderID: providerID,
		ID:         modelID,
	})
	if err != nil {
		return BigModel{}, err
	}
	return fromDBModel(dbModel), nil
}

func (s *service) ListModels(ctx context.Context, providerID string) ([]BigModel, error) {
	dbModels, err := s.q.ListBigModelsByProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	models := make([]BigModel, len(dbModels))
	for i, item := range dbModels {
		models[i] = fromDBModel(item)
	}
	return models, nil
}

func (s *service) ListAllModels(ctx context.Context) ([]BigModel, error) {
	dbModels, err := s.q.ListBigModels(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]BigModel, len(dbModels))
	for i, item := range dbModels {
		models[i] = fromDBModel(item)
	}
	return models, nil
}

func (s *service) SaveModel(ctx context.Context, model BigModel) (BigModel, error) {
	dbModel, err := s.q.UpdateBigModel(ctx, db.UpdateBigModelParams{
		ProviderID:             model.ProviderID,
		ID:                     model.ID,
		Name:                   model.Name,
		CostPer1MIn:            model.CostPer1MIn,
		CostPer1MOut:           model.CostPer1MOut,
		CostPer1MInCached:      model.CostPer1MInCached,
		CostPer1MOutCached:     model.CostPer1MOutCached,
		ContextWindow:          model.ContextWindow,
		DefaultMaxTokens:       model.DefaultMaxTokens,
		CanReason:              model.CanReason,
		ReasoningLevels:        marshalReasoningLevels(model.ReasoningLevels),
		DefaultReasoningEffort: toNullString(model.DefaultReasoningEffort),
		SupportsImages:         model.SupportsImages,
		Disabled:               model.Disabled,
		SortOrder:              model.SortOrder,
	})
	if err != nil {
		return BigModel{}, err
	}
	return fromDBModel(dbModel), nil
}

func (s *service) DeleteModel(ctx context.Context, providerID, modelID string) error {
	return s.q.DeleteBigModel(ctx, db.DeleteBigModelParams{
		ProviderID: providerID,
		ID:         modelID,
	})
}

func (s *service) fromDBItem(item db.Provider) Provider {
	return Provider{
		ID:                  item.ID,
		Name:                item.Name,
		Type:                item.Type,
		APIEndpoint:         item.APIEndpoint.String,
		APIKey:              item.APIKey.String,
		DefaultLargeModelID: item.DefaultLargeModelID.String,
		DefaultSmallModelID: item.DefaultSmallModelID.String,
		DefaultHeaders:      unmarshalHeaders(item.DefaultHeaders.String),
		Disabled:            item.Disabled,
		SortOrder:           item.SortOrder,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	}
}

func fromDBModel(item db.BigModel) BigModel {
	return BigModel{
		ProviderID:             item.ProviderID,
		ID:                     item.ID,
		Name:                   item.Name,
		CostPer1MIn:            item.CostPer1MIn,
		CostPer1MOut:           item.CostPer1MOut,
		CostPer1MInCached:      item.CostPer1MInCached,
		CostPer1MOutCached:     item.CostPer1MOutCached,
		ContextWindow:          item.ContextWindow,
		DefaultMaxTokens:       item.DefaultMaxTokens,
		CanReason:              item.CanReason,
		ReasoningLevels:        unmarshalReasoningLevels(item.ReasoningLevels.String),
		DefaultReasoningEffort: item.DefaultReasoningEffort.String,
		SupportsImages:         item.SupportsImages,
		Disabled:               item.Disabled,
		SortOrder:              item.SortOrder,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
}

func toNullString(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

func marshalHeaders(headers map[string]string) sql.NullString {
	if len(headers) == 0 {
		return sql.NullString{}
	}
	data, err := json.Marshal(headers)
	if err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func unmarshalHeaders(data string) map[string]string {
	if data == "" {
		return nil
	}
	headers := map[string]string{}
	if err := json.Unmarshal([]byte(data), &headers); err != nil {
		return nil
	}
	return headers
}

func marshalReasoningLevels(levels []string) sql.NullString {
	if len(levels) == 0 {
		return sql.NullString{}
	}
	data, err := json.Marshal(levels)
	if err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func unmarshalReasoningLevels(data string) []string {
	if data == "" {
		return nil
	}
	var levels []string
	if err := json.Unmarshal([]byte(data), &levels); err != nil {
		return nil
	}
	return levels
}
