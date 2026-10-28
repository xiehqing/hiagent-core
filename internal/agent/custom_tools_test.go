package agent

import (
	"context"
	"os/exec"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"
	"github.com/xiehqing/hiagent-core/internal/config"
	"github.com/xiehqing/hiagent-core/internal/hooks"
	"github.com/xiehqing/hiagent-core/internal/permission"
)

func testCustomTool(name string) fantasy.AgentTool {
	return fantasy.NewAgentTool(name, "Echo a value", func(ctx context.Context, input struct {
		Value string `json:"value"`
	}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if err := ctx.Err(); err != nil {
			return fantasy.ToolResponse{}, err
		}
		return fantasy.NewTextResponse(input.Value), nil
	})
}

func newCustomToolCoordinator(customTools ...fantasy.AgentTool) *coordinator {
	store := config.NewTestStore(&config.Config{})
	cfg := store.Config()
	cfg.Options.AutoLSP = new(false)
	cfg.Models[config.SelectedModelTypeLarge] = config.SelectedModel{Provider: "test", Model: "test"}
	cfg.Models[config.SelectedModelTypeSmall] = cfg.Models[config.SelectedModelTypeLarge]
	cfg.Providers.Set("test", config.ProviderConfig{
		ID: "test", Type: catwalk.TypeOpenAICompat, APIKey: "test", BaseURL: "http://127.0.0.1:1/v1",
		Models: []catwalk.Model{{ID: "test", Name: "Test", ContextWindow: 8192, DefaultMaxTokens: 1024}},
	})
	cfg.Agents = map[string]config.Agent{
		config.AgentCoder: {AllowedTools: []string{"view"}, AllowedMCP: map[string][]string{}},
	}
	return &coordinator{
		cfg: store, customTools: customTools,
		permissions:  permission.NewPermissionService(".", true, nil),
		currentAgent: NewSessionAgent(SessionAgentOptions{}),
	}
}

func TestValidateCustomTools(t *testing.T) {
	var nilTool *customTool
	for _, tt := range []struct {
		name  string
		tools []fantasy.AgentTool
		err   string
	}{
		{name: "nil", tools: []fantasy.AgentTool{nil}, err: "index 0 is nil"},
		{name: "typed nil", tools: []fantasy.AgentTool{nilTool}, err: "index 0 is nil"},
		{name: "empty name", tools: []fantasy.AgentTool{testCustomTool(" ")}, err: "empty name"},
		{name: "duplicate", tools: []fantasy.AgentTool{testCustomTool("lookup"), testCustomTool("lookup")}, err: `duplicate custom tool name "lookup"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, ValidateCustomTools(tt.tools), tt.err)
		})
	}
}

func TestCustomToolsSurviveRefreshAndRespectDisabling(t *testing.T) {
	c := newCustomToolCoordinator(testCustomTool("lookup"))
	for range 2 {
		require.NoError(t, c.UpdateModels(t.Context()))
		toolList := c.currentAgent.(*sessionAgent).tools.Copy()
		require.Equal(t, []string{"lookup", "view"}, toolNames(toolList))
		response, err := toolList[0].Run(t.Context(), fantasy.ToolCall{Name: "lookup", Input: `{"value":"order-42"}`})
		require.NoError(t, err)
		require.Equal(t, "order-42", response.Content)
	}
	c.cfg.Config().Options.DisabledTools = []string{"lookup"}
	require.NoError(t, c.UpdateModels(t.Context()))
	require.Equal(t, []string{"view"}, toolNames(c.currentAgent.(*sessionAgent).tools.Copy()))
	c.cfg.Config().Options.DisabledTools = nil
	require.NoError(t, c.UpdateModels(t.Context()))
	require.Equal(t, []string{"lookup", "view"}, toolNames(c.currentAgent.(*sessionAgent).tools.Copy()))
}

func TestCustomToolsExcludedFromSubAgents(t *testing.T) {
	c := newCustomToolCoordinator(testCustomTool("lookup"))
	agentCfg := c.cfg.Config().Agents[config.AgentCoder]
	agentCfg.AllowedTools = append(agentCfg.AllowedTools, "lookup")
	toolList, err := c.buildTools(t.Context(), agentCfg, true)
	require.NoError(t, err)
	require.Equal(t, []string{"view"}, toolNames(toolList))
}

func TestCustomToolsRejectConflictingNames(t *testing.T) {
	c := newCustomToolCoordinator(testCustomTool("view"))
	_, err := c.buildTools(t.Context(), c.cfg.Config().Agents[config.AgentCoder], false)
	require.ErrorContains(t, err, `"view" conflicts with an enabled tool`)

	// MCP tools enter the same merged list using their public mcp_* names.
	_, err = mergeCustomTools([]fantasy.AgentTool{testCustomTool("mcp_orders_lookup")},
		[]fantasy.AgentTool{testCustomTool("mcp_orders_lookup")}, nil)
	require.ErrorContains(t, err, `"mcp_orders_lookup" conflicts with an enabled tool`)
}

func TestCustomToolInstancesAndProviderOptionsAreIsolated(t *testing.T) {
	source := testCustomTool("lookup")
	first := newCustomToolCoordinator(source)
	second := newCustomToolCoordinator(testCustomTool("other"))
	require.NoError(t, first.UpdateModels(t.Context()))
	require.NoError(t, second.UpdateModels(t.Context()))
	require.Equal(t, []string{"lookup", "view"}, toolNames(first.currentAgent.(*sessionAgent).tools.Copy()))
	require.Equal(t, []string{"other", "view"}, toolNames(second.currentAgent.(*sessionAgent).tools.Copy()))

	// The same caller-owned tool may also be reused across instances/builds.
	third := newCustomToolCoordinator(source)
	require.NoError(t, third.UpdateModels(t.Context()))
	first.currentAgent.(*sessionAgent).tools.Copy()[0].SetProviderOptions(fantasy.ProviderOptions{
		"anthropic": &anthropic.ProviderOptions{},
	})
	require.Empty(t, source.ProviderOptions())
	require.Empty(t, third.currentAgent.(*sessionAgent).tools.Copy()[0].ProviderOptions())
	require.NoError(t, first.UpdateModels(t.Context()))
	require.Empty(t, first.currentAgent.(*sessionAgent).tools.Copy()[0].ProviderOptions())
}

func TestCustomToolsHonorCancellation(t *testing.T) {
	c := newCustomToolCoordinator(testCustomTool("lookup"))
	require.NoError(t, c.UpdateModels(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := c.currentAgent.(*sessionAgent).tools.Copy()[0].Run(ctx, fantasy.ToolCall{
		Name: "lookup", Input: `{"value":"order-42"}`,
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestCustomToolsRunThroughHooks(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("hooks require sh")
	}
	c := newCustomToolCoordinator(testCustomTool("lookup"))
	c.cfg.Config().Hooks = map[string][]config.HookConfig{
		hooks.EventPreToolUse: {{Matcher: "^lookup$", Command: `echo '{"decision":"deny","reason":"blocked lookup"}'`}},
	}
	require.NoError(t, c.UpdateModels(t.Context()))
	response, err := c.currentAgent.(*sessionAgent).tools.Copy()[0].Run(t.Context(), fantasy.ToolCall{
		Name: "lookup", Input: `{"value":"order-42"}`,
	})
	require.NoError(t, err)
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "blocked lookup")
}

func toolNames(toolList []fantasy.AgentTool) []string {
	names := make([]string, len(toolList))
	for i, tool := range toolList {
		names[i] = tool.Info().Name
	}
	return names
}
