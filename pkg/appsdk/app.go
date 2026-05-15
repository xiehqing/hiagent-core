package appsdk

import (
	"charm.land/fantasy"
	"context"
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/xiehqing/hiagent-core/internal/agent"
	"github.com/xiehqing/hiagent-core/internal/app"
	"github.com/xiehqing/hiagent-core/internal/config"
	"github.com/xiehqing/hiagent-core/internal/db"
	"github.com/xiehqing/hiagent-core/internal/filetracker"
	"github.com/xiehqing/hiagent-core/internal/history"
	"github.com/xiehqing/hiagent-core/internal/message"
	"github.com/xiehqing/hiagent-core/internal/provider"
	"github.com/xiehqing/hiagent-core/internal/pubsub"
	"github.com/xiehqing/hiagent-core/internal/session"
	"log/slog"
	"os"
	"time"
)

type App struct {
	conn        *sql.DB
	AppInstance *app.App
}

type AppService struct {
	Sessions    session.Service
	Messages    message.Service
	Providers   provider.Service
	History     history.Service
	FileTracker filetracker.Service
}

// NewDBService 鍒涘缓db鏈嶅姟
func NewDBService(conn *sql.DB) (*AppService, error) {
	q := db.New(conn)
	sessions := session.NewService(q, conn)
	messages := message.NewService(q)
	files := history.NewService(q, conn)
	providers := provider.NewService(q)
	return &AppService{
		Sessions:    sessions,
		Messages:    messages,
		Providers:   providers,
		History:     files,
		FileTracker: filetracker.NewService(q),
	}, nil
}

func NewService(ctx context.Context, conn *sql.DB, opts ...Option) (*AppService, error) {
	if conn == nil {
		return nil, fmt.Errorf("sdk.New: conn is required")
	}
	o := &Options{
		cfg: AppConfig{
			SkipPermissionRequests:    true,
			Debug:                     false,
			DisableProviderAutoUpdate: true,
		},
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.cfg.WorkDir == "" {
		return nil, fmt.Errorf("sdk.New: WorkDir is required (use sdk.WithWorkDir)")
	}
	o.cfg.DataDir = config.DefaultDataDir(o.cfg.WorkDir, o.cfg.DataDir)
	q := db.New(conn)
	sessions := session.NewService(q, conn)
	messages := message.NewService(q)
	files := history.NewService(q, conn)
	providers := provider.NewService(q)
	return &AppService{
		Sessions:    sessions,
		Messages:    messages,
		Providers:   providers,
		History:     files,
		FileTracker: filetracker.NewService(q),
	}, nil
}

func New(ctx context.Context, conn *sql.DB, opts ...Option) (*App, error) {
	if conn == nil {
		return nil, fmt.Errorf("sdk.New: conn is required")
	}
	o := &Options{
		cfg: AppConfig{
			SkipPermissionRequests:    true,
			Debug:                     false,
			DisableProviderAutoUpdate: true,
		},
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.cfg.WorkDir == "" {
		return nil, fmt.Errorf("sdk.New: WorkDir is required (use sdk.WithWorkDir)")
	}
	o.cfg.DataDir = config.DefaultDataDir(o.cfg.WorkDir, o.cfg.DataDir)
	cfg, err := config.Init(o.cfg.WorkDir, o.cfg.DataDir, conn, o.cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("sdk.New: failed to initialize config: %w", err)
	}
	cfg.Overrides().SkipPermissionRequests = o.cfg.SkipPermissionRequests
	cfg.Config().Options.DisableProviderAutoUpdate = o.cfg.DisableProviderAutoUpdate
	if o.cfg.SelectedModel != "" && o.cfg.SelectedProvider != "" {
		err = cfg.SetRuntimePreferredModel(o.cfg.SelectedProvider, o.cfg.SelectedModel)
		if err != nil {
			return nil, errors.WithMessage(err, "sdk.New: failed to set runtime preferred model")
		}
	}
	app, err := app.NewWithSystemPrompt(ctx, conn, cfg, o.cfg.AdditionalSystemPrompt)
	if err != nil {
		return nil, fmt.Errorf("sdk.New: failed to create app workspace: %w", err)
	}
	return &App{AppInstance: app}, nil
}

func (a *App) SubmitMessage(ctx context.Context, prompt string, continueSessionID string, useLast bool) (*fantasy.AgentResult, error) {
	if a.AppInstance.AgentCoordinator == nil {
		return nil, fmt.Errorf("sdk.SubmitMessage: agent coordinator is nil")
	}
	session, err := a.resolveSession(ctx, continueSessionID, useLast)

	if err != nil {
		return nil, fmt.Errorf("sdk.SubmitMessage: failed to create session for sdk mode: %w", err)
	}

	if continueSessionID != "" || useLast {
		slog.Info("sdk.SubmitMessage: continuing session for sdk run", "session_id", session.ID)
	} else {
		slog.Info("sdk.SubmitMessage: created session for sdk run", "session_id", session.ID)
	}
	return a.AppInstance.AgentCoordinator.Run(ctx, session.ID, prompt)
}

func (a *App) resolveSession(ctx context.Context, continueSessionID string, useLast bool) (session.Session, error) {
	switch {
	case continueSessionID != "":
		if a.AppInstance.Sessions.IsAgentToolSession(continueSessionID) {
			return session.Session{}, fmt.Errorf("sdk.resolveSession: cannot continue an agent tool session: %s", continueSessionID)
		}
		sess, err := a.AppInstance.Sessions.Get(ctx, continueSessionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Info("sdk.resolveSession: session not found, creating new session", "session_id", continueSessionID)
				return a.AppInstance.Sessions.CreateSession(ctx, agent.DefaultSessionName, continueSessionID)
			}
			return session.Session{}, fmt.Errorf("sdk.resolveSession: session not found: %s", continueSessionID)
		}
		if sess.ParentSessionID != "" {
			return session.Session{}, fmt.Errorf("sdk.resolveSession: cannot continue a child session: %s", continueSessionID)
		}
		return sess, nil

	case useLast:
		sess, err := a.AppInstance.Sessions.GetLast(ctx)
		if err != nil {
			return session.Session{}, fmt.Errorf("sdk.resolveSession: no sessions found to continue")
		}
		return sess, nil

	default:
		return a.AppInstance.Sessions.Create(ctx, agent.DefaultSessionName)
	}
}

// SubscribeMessage subscribes to the message channel.
func (a *App) SubscribeMessage(ctx context.Context) <-chan pubsub.Event[message.Message] {
	return a.AppInstance.Messages.Subscribe(ctx)
}

// Shutdown shuts down the app.
func (a *App) Shutdown() {
	a.AppInstance.Shutdown()
}

// Providers returns the available providers.
// func (a *App) Providers() ([]config.ProviderItem, error) {
//	providers, err := a.AppInstance.Store().Providers()
//	if err != nil {
//		return nil, fmt.Errorf("sdk.Providers: failed to get providers: %w", err)
//	}
//	return providers, nil
//}

// SessionFiles returns the files associated with a session.
func (a *AppService) SessionFiles(ctx context.Context, sessionID string) ([]history.File, error) {
	files, err := a.History.ListLatestSessionFiles(ctx, sessionID)
	if err != nil {
		slog.Error("sdk.SessionFiles: failed to list session files", "session_id", sessionID, "err", err)
		return nil, fmt.Errorf("sdk.SessionFiles: failed to list session files: %w", err)
	}
	for i := 0; i < len(files); i++ {
		file := files[i]
		if file.Content == "" && file.Path != "" {
			cd, _ := os.ReadFile(file.Path)
			files[i].Content = string(cd)
		}
	}
	return files, nil
}

// SessionReadFiles returns the files read during a session.
func (a *AppService) SessionReadFiles(ctx context.Context, sessionID string) ([]string, error) {
	return a.FileTracker.ListReadFiles(ctx, sessionID)
}

// DeleteSession deletes a session.
func (a *AppService) DeleteSession(ctx context.Context, sessionID string) error {
	return a.Sessions.Delete(ctx, sessionID)
}

func (a *AppService) ListSession(ctx context.Context) ([]session.Session, error) {
	return a.Sessions.List(ctx)
}

func (a *AppService) Session(ctx context.Context, sessionID string) (session.Session, error) {
	return a.Sessions.Get(ctx, sessionID)
}

func (a *AppService) SessionByIDs(ctx context.Context, sessionIDs []string) ([]session.Session, error) {
	return a.Sessions.ListByIDs(ctx, sessionIDs)
}

// SessionMessages 鑾峰彇浼氳瘽娑堟伅
func (a *AppService) SessionMessages(ctx context.Context, sessionID string) ([]DataMessage, error) {
	messages, err := a.Messages.List(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sdk.Sessions: failed to list messages: %w", err)
	}
	files, err := a.SessionFiles(ctx, sessionID)
	if err != nil {
		return nil, errors.WithMessage(err, "sdk.Sessions: failed to list session files")
	}
	//mergeMessages := a.mergeMessages(messages)
	messageList := make([]DataMessage, 0)
	for i, msg := range messages {
		contentPartData := marshalParts(msg.Parts)
		dm := DataMessage{
			ID:               msg.ID,
			Role:             msg.Role,
			SessionID:        msg.SessionID,
			Parts:            contentPartData,
			Model:            msg.Model,
			Provider:         msg.Provider,
			IsSummaryMessage: msg.IsSummaryMessage,
		}
		if msg.CreatedAt != 0 {
			dm.CreatedAt = time.Unix(msg.CreatedAt, 0).Format("2006-01-02 15:04:05")
		}
		if msg.UpdatedAt != 0 {
			dm.UpdatedAt = time.Unix(msg.UpdatedAt, 0).Format("2006-01-02 15:04:05")
		}
		if i == len(messages)-1 {
			dm.Files = files
		}
		messageList = append(messageList, dm)
	}
	return messageList, nil
}
