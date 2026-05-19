package appsdk

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	internalconfig "github.com/xiehqing/hiagent-core/internal/config"
	internalenv "github.com/xiehqing/hiagent-core/internal/env"
	"github.com/xiehqing/hiagent-core/internal/home"
)

type MCPType string

const (
	MCPTypeStdio MCPType = "stdio"
	MCPTypeSSE   MCPType = "sse"
	MCPTypeHTTP  MCPType = "http"
)

type MCPConfig struct {
	Command       string            `json:"command,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Type          MCPType           `json:"type"`
	URL           string            `json:"url,omitempty"`
	Disabled      bool              `json:"disabled,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
	Timeout       int               `json:"timeout,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

type MCPConnectivityResult struct {
	Supported       bool    `json:"supported"`
	Connected       bool    `json:"connected"`
	Type            MCPType `json:"type"`
	Message         string  `json:"message,omitempty"`
	ProtocolVersion string  `json:"protocol_version,omitempty"`
	ServerName      string  `json:"server_name,omitempty"`
	ServerTitle     string  `json:"server_title,omitempty"`
	ServerVersion   string  `json:"server_version,omitempty"`
	Instructions    string  `json:"instructions,omitempty"`
	LatencyMS       int64   `json:"latency_ms,omitempty"`
}

type MCPToolInfo struct {
	Name         string `json:"name"`
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	InputSchema  any    `json:"input_schema,omitempty"`
	OutputSchema any    `json:"output_schema,omitempty"`
}

type MCPPromptArgumentInfo struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

type MCPPromptInfo struct {
	Name        string                  `json:"name"`
	Title       string                  `json:"title,omitempty"`
	Description string                  `json:"description,omitempty"`
	Arguments   []MCPPromptArgumentInfo `json:"arguments,omitempty"`
}

type MCPResourceInfo struct {
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	URI         string `json:"uri"`
	MIMEType    string `json:"mime_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

type MCPServerDetails struct {
	Connectivity MCPConnectivityResult `json:"connectivity"`
	Tools        []MCPToolInfo         `json:"tools"`
	Prompts      []MCPPromptInfo       `json:"prompts"`
	Resources    []MCPResourceInfo     `json:"resources"`
}

type NamedMCPServerDetails struct {
	Name    string            `json:"name"`
	Config  MCPConfig         `json:"config"`
	Details *MCPServerDetails `json:"details,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// TestMCPConnectivity tests whether an MCP server can be reached and initialized.
func TestMCPConnectivity(ctx context.Context, cfg MCPConfig) (*MCPConnectivityResult, error) {
	if err := validateMCPInspectConfig(cfg); err != nil {
		return nil, err
	}

	startedAt := time.Now()
	session, cancel, err := connectMCP(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer closeMCPSession(session, cancel)

	pingCtx, pingCancel := context.WithTimeout(ctx, mcpTimeout(cfg))
	defer pingCancel()
	if err := session.Ping(pingCtx, nil); err != nil {
		return nil, fmt.Errorf("sdk.TestMCPConnectivity: ping failed: %w", err)
	}

	result := connectivityFromSession(cfg, session)
	result.Connected = true
	result.LatencyMS = time.Since(startedAt).Milliseconds()
	result.Message = "connected"
	return &result, nil
}

// ListMCPTools lists tools exposed by an MCP server.
func ListMCPTools(ctx context.Context, cfg MCPConfig) ([]MCPToolInfo, error) {
	if err := validateMCPInspectConfig(cfg); err != nil {
		return nil, err
	}

	session, cancel, err := connectMCP(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer closeMCPSession(session, cancel)

	result, err := session.ListTools(ctx, &gomcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("sdk.ListMCPTools: failed to list tools: %w", err)
	}

	tools := make([]MCPToolInfo, 0, len(result.Tools))
	for _, tool := range result.Tools {
		if tool == nil {
			continue
		}
		tools = append(tools, MCPToolInfo{
			Name:         tool.Name,
			Title:        tool.Title,
			Description:  tool.Description,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
		})
	}
	return tools, nil
}

// ListMCPPrompts lists prompts exposed by an MCP server.
func ListMCPPrompts(ctx context.Context, cfg MCPConfig) ([]MCPPromptInfo, error) {
	if err := validateMCPInspectConfig(cfg); err != nil {
		return nil, err
	}

	session, cancel, err := connectMCP(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer closeMCPSession(session, cancel)

	if session.InitializeResult() == nil || session.InitializeResult().Capabilities == nil || session.InitializeResult().Capabilities.Prompts == nil {
		return nil, nil
	}

	result, err := session.ListPrompts(ctx, &gomcp.ListPromptsParams{})
	if err != nil {
		return nil, fmt.Errorf("sdk.ListMCPPrompts: failed to list prompts: %w", err)
	}

	prompts := make([]MCPPromptInfo, 0, len(result.Prompts))
	for _, prompt := range result.Prompts {
		if prompt == nil {
			continue
		}
		item := MCPPromptInfo{
			Name:        prompt.Name,
			Title:       prompt.Title,
			Description: prompt.Description,
		}
		if len(prompt.Arguments) > 0 {
			item.Arguments = make([]MCPPromptArgumentInfo, 0, len(prompt.Arguments))
			for _, arg := range prompt.Arguments {
				if arg == nil {
					continue
				}
				item.Arguments = append(item.Arguments, MCPPromptArgumentInfo{
					Name:        arg.Name,
					Title:       arg.Title,
					Description: arg.Description,
					Required:    arg.Required,
				})
			}
		}
		prompts = append(prompts, item)
	}
	return prompts, nil
}

// ListMCPResources lists resources exposed by an MCP server.
func ListMCPResources(ctx context.Context, cfg MCPConfig) ([]MCPResourceInfo, error) {
	if err := validateMCPInspectConfig(cfg); err != nil {
		return nil, err
	}

	session, cancel, err := connectMCP(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer closeMCPSession(session, cancel)

	if session.InitializeResult() == nil || session.InitializeResult().Capabilities == nil || session.InitializeResult().Capabilities.Resources == nil {
		return nil, nil
	}

	result, err := session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		if isMethodNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("sdk.ListMCPResources: failed to list resources: %w", err)
	}

	resources := make([]MCPResourceInfo, 0, len(result.Resources))
	for _, resource := range result.Resources {
		if resource == nil {
			continue
		}
		resources = append(resources, MCPResourceInfo{
			Name:        resource.Name,
			Title:       resource.Title,
			Description: resource.Description,
			URI:         resource.URI,
			MIMEType:    resource.MIMEType,
			Size:        resource.Size,
		})
	}
	return resources, nil
}

// DescribeMCP connects to an MCP server and returns connectivity,
// tools, prompts, and resources in one call.
func DescribeMCP(ctx context.Context, cfg MCPConfig) (*MCPServerDetails, error) {
	if err := validateMCPInspectConfig(cfg); err != nil {
		return nil, err
	}

	startedAt := time.Now()
	session, cancel, err := connectMCP(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer closeMCPSession(session, cancel)

	connectivity := connectivityFromSession(cfg, session)
	connectivity.Connected = true
	connectivity.LatencyMS = time.Since(startedAt).Milliseconds()
	connectivity.Message = "connected"

	toolsResult, err := session.ListTools(ctx, &gomcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("sdk.DescribeMCP: failed to list tools: %w", err)
	}

	var promptsResult *gomcp.ListPromptsResult
	if session.InitializeResult() != nil && session.InitializeResult().Capabilities != nil && session.InitializeResult().Capabilities.Prompts != nil {
		promptsResult, err = session.ListPrompts(ctx, &gomcp.ListPromptsParams{})
		if err != nil {
			return nil, fmt.Errorf("sdk.DescribeMCP: failed to list prompts: %w", err)
		}
	}

	var resourcesResult *gomcp.ListResourcesResult
	if session.InitializeResult() != nil && session.InitializeResult().Capabilities != nil && session.InitializeResult().Capabilities.Resources != nil {
		resourcesResult, err = session.ListResources(ctx, &gomcp.ListResourcesParams{})
		if err != nil && !isMethodNotFoundError(err) {
			return nil, fmt.Errorf("sdk.DescribeMCP: failed to list resources: %w", err)
		}
	}

	details := &MCPServerDetails{
		Connectivity: connectivity,
		Tools:        mapMCPTools(toolsResult.Tools),
		Prompts:      mapMCPPrompts(nilIfNil(promptsResult)),
		Resources:    mapMCPResources(nilIfNilResources(resourcesResult)),
	}
	return details, nil
}

// DescribeMCPMap batch-inspects a named MCP config map and returns one result
// per configured MCP. Individual failures are captured in the result slice.
func DescribeMCPMap(ctx context.Context, configs map[string]MCPConfig) []NamedMCPServerDetails {
	if len(configs) == 0 {
		return nil
	}

	names := slices.Collect(maps.Keys(configs))
	slices.Sort(names)

	results := make([]NamedMCPServerDetails, 0, len(names))
	for _, name := range names {
		cfg := configs[name]
		item := NamedMCPServerDetails{
			Name:   name,
			Config: cfg,
		}

		details, err := DescribeMCP(ctx, cfg)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Details = details
		}
		results = append(results, item)
	}

	return results
}

func mapMCPTools(items []*gomcp.Tool) []MCPToolInfo {
	if len(items) == 0 {
		return nil
	}
	tools := make([]MCPToolInfo, 0, len(items))
	for _, tool := range items {
		if tool == nil {
			continue
		}
		tools = append(tools, MCPToolInfo{
			Name:         tool.Name,
			Title:        tool.Title,
			Description:  tool.Description,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
		})
	}
	return tools
}

func mapMCPPrompts(items []*gomcp.Prompt) []MCPPromptInfo {
	if len(items) == 0 {
		return nil
	}
	prompts := make([]MCPPromptInfo, 0, len(items))
	for _, prompt := range items {
		if prompt == nil {
			continue
		}
		item := MCPPromptInfo{
			Name:        prompt.Name,
			Title:       prompt.Title,
			Description: prompt.Description,
		}
		if len(prompt.Arguments) > 0 {
			item.Arguments = make([]MCPPromptArgumentInfo, 0, len(prompt.Arguments))
			for _, arg := range prompt.Arguments {
				if arg == nil {
					continue
				}
				item.Arguments = append(item.Arguments, MCPPromptArgumentInfo{
					Name:        arg.Name,
					Title:       arg.Title,
					Description: arg.Description,
					Required:    arg.Required,
				})
			}
		}
		prompts = append(prompts, item)
	}
	return prompts
}

func mapMCPResources(items []*gomcp.Resource) []MCPResourceInfo {
	if len(items) == 0 {
		return nil
	}
	resources := make([]MCPResourceInfo, 0, len(items))
	for _, resource := range items {
		if resource == nil {
			continue
		}
		resources = append(resources, MCPResourceInfo{
			Name:        resource.Name,
			Title:       resource.Title,
			Description: resource.Description,
			URI:         resource.URI,
			MIMEType:    resource.MIMEType,
			Size:        resource.Size,
		})
	}
	return resources
}

func validateMCPInspectConfig(cfg MCPConfig) error {
	if cfg.Disabled {
		return fmt.Errorf("sdk.MCP: mcp config is disabled")
	}
	if cfg.Type != MCPTypeHTTP && cfg.Type != MCPTypeSSE && cfg.Type != MCPTypeStdio {
		return fmt.Errorf("sdk.MCP: unsupported mcp type: %s", cfg.Type)
	}
	if cfg.Type == MCPTypeStdio && strings.TrimSpace(cfg.Command) == "" {
		return fmt.Errorf("sdk.MCP: mcp stdio config requires a non-empty command")
	}
	if (cfg.Type == MCPTypeHTTP || cfg.Type == MCPTypeSSE) && strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("sdk.MCP: mcp %s config requires a non-empty url", cfg.Type)
	}
	return nil
}

func connectMCP(ctx context.Context, cfg MCPConfig) (*gomcp.ClientSession, context.CancelFunc, error) {
	timeout := mcpTimeout(cfg)
	connectCtx, cancel := context.WithTimeout(ctx, timeout)

	transport, err := createMCPTransport(connectCtx, cfg)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	client := gomcp.NewClient(
		&gomcp.Implementation{
			Name:    "hiagent-appsdk",
			Title:   "HiAgent AppSDK",
			Version: "dev",
		},
		nil,
	)

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("sdk.MCP: failed to connect: %w", err)
	}
	return session, cancel, nil
}

func createMCPTransport(ctx context.Context, cfg MCPConfig) (gomcp.Transport, error) {
	httpClient := &http.Client{
		Transport: mcpHeaderRoundTripper{
			headers: resolveMCPHeaders(cfg.Headers),
		},
	}

	switch cfg.Type {
	case MCPTypeStdio:
		resolver := internalconfig.NewShellVariableResolver(internalenv.New())
		command, err := resolver.ResolveValue(cfg.Command)
		if err != nil {
			return nil, fmt.Errorf("sdk.MCP: invalid mcp command: %w", err)
		}
		cmd := exec.CommandContext(ctx, home.Long(command), cfg.Args...)
		cmd.Env = append(os.Environ(), internalMCPConfig(cfg).ResolvedEnv()...)
		return &gomcp.CommandTransport{
			Command:           cmd,
			TerminateDuration: mcpTimeout(cfg),
		}, nil
	case MCPTypeHTTP:
		return &gomcp.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}, nil
	case MCPTypeSSE:
		return &gomcp.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}, nil
	default:
		return nil, fmt.Errorf("sdk.MCP: unsupported mcp type: %s", cfg.Type)
	}
}

func resolveMCPHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	resolver := internalconfig.NewShellVariableResolver(internalenv.New())
	resolved := make(map[string]string, len(headers))
	for key, value := range headers {
		v, err := resolver.ResolveValue(value)
		if err != nil {
			resolved[key] = value
			continue
		}
		resolved[key] = v
	}
	return resolved
}

func internalMCPConfig(cfg MCPConfig) internalconfig.MCPConfig {
	return internalconfig.MCPConfig{
		Command:       cfg.Command,
		Env:           cfg.Env,
		Args:          cfg.Args,
		Type:          internalconfig.MCPType(cfg.Type),
		URL:           cfg.URL,
		Disabled:      cfg.Disabled,
		DisabledTools: cfg.DisabledTools,
		Timeout:       cfg.Timeout,
		Headers:       cfg.Headers,
	}
}

func connectivityFromSession(cfg MCPConfig, session *gomcp.ClientSession) MCPConnectivityResult {
	result := MCPConnectivityResult{
		Supported: true,
		Type:      cfg.Type,
	}
	initResult := session.InitializeResult()
	if initResult == nil {
		return result
	}
	result.ProtocolVersion = initResult.ProtocolVersion
	result.Instructions = initResult.Instructions
	if initResult.ServerInfo != nil {
		result.ServerName = initResult.ServerInfo.Name
		result.ServerTitle = initResult.ServerInfo.Title
		result.ServerVersion = initResult.ServerInfo.Version
	}
	return result
}

func closeMCPSession(session *gomcp.ClientSession, cancel context.CancelFunc) {
	if session != nil {
		_ = session.Close()
	}
	if cancel != nil {
		cancel()
	}
}

func mcpTimeout(cfg MCPConfig) time.Duration {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15
	}
	return time.Duration(timeout) * time.Second
}

func isMethodNotFoundError(err error) bool {
	var rpcErr *jsonrpc.Error
	return errors.As(err, &rpcErr) && rpcErr != nil && rpcErr.Code == jsonrpc.CodeMethodNotFound
}

func nilIfNil(result *gomcp.ListPromptsResult) []*gomcp.Prompt {
	if result == nil {
		return nil
	}
	return result.Prompts
}

func nilIfNilResources(result *gomcp.ListResourcesResult) []*gomcp.Resource {
	if result == nil {
		return nil
	}
	return result.Resources
}

type mcpHeaderRoundTripper struct {
	headers map[string]string
}

func (rt mcpHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for key, value := range rt.headers {
		clone.Header.Set(key, value)
	}
	return http.DefaultTransport.RoundTrip(clone)
}
