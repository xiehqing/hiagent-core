package appsdk

import (
	"fmt"
	"github.com/xiehqing/hiagent-core/internal/history"
	"github.com/xiehqing/hiagent-core/internal/message"
	"log/slog"
	"os"
	"path/filepath"
)

type DatabaseDriver string

const (
	DatabaseDriverSqlite DatabaseDriver = "sqlite"
	DatabaseDriverMysql  DatabaseDriver = "mysql"
)

type AppConfig struct {
	WorkDir                   string         `json:"workDir"`
	DataDir                   string         `json:"dataDir"`
	Database                  DatabaseConfig `json:"database"`
	SkipPermissionRequests    bool           `json:"skipPermissionRequests"`
	DisableProviderAutoUpdate bool           `json:"disableProviderAutoUpdate"`
	AdditionalSystemPrompt    string         `json:"additionalSystemPrompt"`
	Debug                     bool           `json:"debug"`
	SelectedProvider          string         `json:"selectedProvider"`
	SelectedModel             string         `json:"selectedModel"`
}

type DatabaseConfig struct {
	Driver DatabaseDriver `json:"driver"`
	DSN    string         `json:"dsn"`
}

type Options struct {
	cfg AppConfig
}

// Option is a functional option for Engine creation.
type Option func(*Options)

func WithSkipPermissionRequests(skipPermissionRequests bool) Option {
	return func(o *Options) {
		o.cfg.SkipPermissionRequests = skipPermissionRequests
	}
}

func WithDebug(debug bool) Option {
	return func(o *Options) {
		o.cfg.Debug = debug
	}
}

func WithDatabaseDriver(driver DatabaseDriver) Option {
	return func(o *Options) {
		o.cfg.Database.Driver = driver
	}
}

func WithDatabaseDSN(dsn string) Option {
	return func(o *Options) {
		o.cfg.Database.DSN = dsn
	}
}

func WithWorkDir(workDir string) Option {
	return func(o *Options) {
		o.cfg.WorkDir = workDir
	}
}

func WithDataDir(dataDir string) Option {
	return func(o *Options) {
		o.cfg.DataDir = dataDir
	}
}

func WithSelectedProvider(selectedProvider string) Option {
	return func(o *Options) {
		o.cfg.SelectedProvider = selectedProvider
	}
}

func WithSelectedModel(selectedModel string) Option {
	return func(o *Options) {
		o.cfg.SelectedModel = selectedModel
	}
}

func WithDisableProviderAutoUpdate(disableProviderAutoUpdate bool) Option {
	return func(o *Options) {
		o.cfg.DisableProviderAutoUpdate = disableProviderAutoUpdate
	}
}

func WithAdditionalSystemPrompt(additionalSystemPrompt string) Option {
	return func(o *Options) {
		o.cfg.AdditionalSystemPrompt = additionalSystemPrompt
	}
}

// createDotCrushDir creates the .crush directory in th
func createDotCrushDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %q %w", dir, err)
	}

	gitIgnorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); os.IsNotExist(err) {
		if err := os.WriteFile(gitIgnorePath, []byte("*\n"), 0o644); err != nil {
			return fmt.Errorf("failed to create .gitignore file: %q %w", gitIgnorePath, err)
		}
	}
	return nil
}

// DataMessage 数据消息
type DataMessage struct {
	ID               string              `json:"id"`
	Role             message.MessageRole `json:"role"`
	SessionID        string              `json:"session_id"`
	Parts            []ContentPartData   `json:"parts"`
	Model            string              `json:"model"`
	Provider         string              `json:"provider"`
	CreatedAt        string              `json:"createdAt"`
	UpdatedAt        string              `json:"updatedAt"`
	IsSummaryMessage bool                `json:"is_summary_message"`
	Files            []history.File      `json:"files,omitempty"`
}

type ContentPartType string

const (
	ReasoningType  ContentPartType = "reasoning"
	TextType       ContentPartType = "text"
	ImageURLType   ContentPartType = "image_url"
	BinaryType     ContentPartType = "binary"
	ToolCallType   ContentPartType = "tool_call"
	ToolResultType ContentPartType = "tool_result"
	FinishType     ContentPartType = "finish"
)

type ContentPartData struct {
	Type ContentPartType     `json:"type"`
	Data message.ContentPart `json:"data"`
}

func marshalParts(parts []message.ContentPart) []ContentPartData {
	wrappedParts := make([]ContentPartData, len(parts))
	for i, part := range parts {
		var typ ContentPartType
		switch part.(type) {
		case message.ReasoningContent:
			typ = ReasoningType
		case message.TextContent:
			typ = TextType
		case message.ImageURLContent:
			typ = ImageURLType
		case message.BinaryContent:
			typ = BinaryType
		case message.ToolCall:
			typ = ToolCallType
		case message.ToolResult:
			typ = ToolResultType
		case message.Finish:
			typ = FinishType
		default:
			slog.Error("unknown part type: ", "part", part)
			continue
		}
		wrappedParts[i] = ContentPartData{
			Type: typ,
			Data: part,
		}
	}
	return wrappedParts
}
