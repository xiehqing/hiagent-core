package appsdk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"github.com/xiehqing/hiagent-core/internal/config"
	"github.com/xiehqing/hiagent-core/internal/db"
)

func sdkTestTool(name string) fantasy.AgentTool {
	return fantasy.NewAgentTool(name, "Test tool", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse("ok"), nil
	})
}

func TestWithToolsCopiesInputAndAppends(t *testing.T) {
	input := []fantasy.AgentTool{sdkTestTool("first")}
	option := WithTools(input...)
	input[0] = sdkTestTool("changed")

	first, second := &Options{}, &Options{}
	option(first)
	WithTools(sdkTestTool("second"))(first)
	option(second)
	require.Len(t, first.tools, 2)
	require.Equal(t, "first", first.tools[0].Info().Name)
	require.Equal(t, "second", first.tools[1].Info().Name)
	require.Len(t, second.tools, 1)
	first.tools[0] = sdkTestTool("replaced")
	require.Equal(t, "first", second.tools[0].Info().Name)
}

func TestNewRejectsInvalidCustomTools(t *testing.T) {
	// Validation must happen before config/DB access, even without a model.
	for _, tt := range []struct {
		name  string
		tools []fantasy.AgentTool
		err   string
	}{
		{name: "nil", tools: []fantasy.AgentTool{nil}, err: "custom tool at index 0 is nil"},
		{name: "duplicate", tools: []fantasy.AgentTool{sdkTestTool("lookup"), sdkTestTool("lookup")}, err: "duplicate custom tool"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(t.Context(), &sql.DB{}, WithWorkDir(t.TempDir()), WithTools(tt.tools...))
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func TestCustomToolsSDKExecutionAfterReinitialization(t *testing.T) {
	var calls atomic.Int32
	lookup := fantasy.NewAgentTool("lookup_order", "Look up an order", func(ctx context.Context, input struct {
		OrderID string `json:"order_id"`
	}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if input.OrderID != "42" {
			return fantasy.ToolResponse{}, fmt.Errorf("unexpected order ID %q", input.OrderID)
		}
		calls.Add(1)
		return fantasy.NewTextResponse("order found"), nil
	})
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !request.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"title","object":"chat.completion","model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"Order lookup"},"finish_reason":"stop"}]}`)
			return
		}
		count := 0
		for _, tool := range request.Tools {
			if tool.Function.Name == "lookup_order" {
				count++
			}
		}
		if len(request.Tools) > 0 && count != 1 {
			t.Errorf("expected exactly one custom tool, got %d", count)
			http.Error(w, "missing or duplicate tool", http.StatusBadRequest)
			return
		}
		delta := map[string]any{"role": "assistant", "content": "done"}
		finishReason := "stop"
		if count == 1 && len(request.Messages) > 0 && request.Messages[len(request.Messages)-1].Role != "tool" {
			delta = map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"index": 0, "id": fmt.Sprintf("lookup-%d", calls.Load()), "type": "function",
					"function": map[string]any{"name": "lookup_order", "arguments": `{"order_id":"42"}`},
				}},
			}
			finishReason = "tool_calls"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, choice := range []any{
			map[string]any{"index": 0, "delta": delta},
			map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finishReason},
		} {
			chunk, err := json.Marshal(map[string]any{
				"id": "completion", "object": "chat.completion.chunk", "model": "test", "choices": []any{choice},
			})
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer modelServer.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	workDir := t.TempDir()
	conn, err := db.Connect(ctx, workDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	// The catalog loader already reads model options, but the existing SQLite
	// migrations do not create that column. Supply the current expected schema.
	_, err = conn.Exec(`ALTER TABLE big_models ADD COLUMN options TEXT`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO providers (id, name, type, api_endpoint, api_key, default_large_model_id, default_small_model_id)
		VALUES ('test', 'Test', ?, ?, 'test', 'test', 'test')`, string(catwalk.TypeOpenAICompat), modelServer.URL+"/v1")
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO big_models (provider_id, id, name, context_window, default_max_tokens)
		VALUES ('test', 'test', 'Test', 32768, 1024)`)
	require.NoError(t, err)
	_, err = config.AddDataConfig(conn, config.DataConfig{
		WorkingDir: "global", Config: `{"options":{"auto_lsp":false,"disable_auto_summarize":true}}`,
	})
	require.NoError(t, err)
	_, err = New(ctx, conn, WithWorkDir(workDir), WithTools(sdkTestTool("view")))
	require.ErrorContains(t, err, `"view" conflicts with an enabled tool`)
	sdk, err := New(ctx, conn, WithWorkDir(workDir), WithTools(lookup))
	require.NoError(t, err)
	defer sdk.Shutdown()
	for i := range 2 {
		if i > 0 {
			require.NoError(t, sdk.AppInstance.InitCoderAgent(ctx))
		}
		result, err := sdk.SubmitMessage(ctx, "Look up order 42", "custom-tools-session", false)
		require.NoError(t, err)
		require.Equal(t, "done", result.Response.Content.Text())
	}
	require.EqualValues(t, 2, calls.Load())
}
