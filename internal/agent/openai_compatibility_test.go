package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"github.com/xiehqing/hiagent-core/internal/config"
)

type openAIRoundTripFunc func(*http.Request) (*http.Response, error)

func (f openAIRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mockOpenAIHTTP(t *testing.T, handler func(*http.Request, map[string]any) string) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = openAIRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		_ = r.Body.Close()
		require.NoError(t, err)
		content := handler(r, body)
		contentType := "application/json"
		if stream, _ := body["stream"].(bool); stream {
			contentType = "text/event-stream"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}},
			Body: io.NopCloser(strings.NewReader(content)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func TestOpenAIProtocolRequests(t *testing.T) {
	for _, tt := range []struct {
		name, typ, model, path, tokenField string
		format                             config.APIFormat
	}{
		{"compat auto Sol", "openai-compat", "gpt-6.1-sol", "/v1/responses", "max_output_tokens", ""},
		{"compat explicit Chat Sol", "openai-compat", "gpt-6.1-sol", "/v1/chat/completions", "max_completion_tokens", config.APIFormatChatCompletions},
		{"compat explicit Responses", "openai-compat", "gpt-6.1-sol", "/v1/responses", "max_output_tokens", config.APIFormatResponses},
		{"openai auto Sol", "openai", "gpt-6.1-sol", "/v1/responses", "max_output_tokens", ""},
		{"openai explicit Chat", "openai", "gpt-6.1-sol", "/v1/chat/completions", "max_completion_tokens", config.APIFormatChatCompletions},
		{"compat legacy GPT5", "openai-compat", "gpt-5", "/v1/chat/completions", "max_completion_tokens", ""},
		{"compat third party", "openai-compat", "deepseek-chat", "/v1/chat/completions", "max_tokens", ""},
		{"openai legacy Responses", "openai", "gpt-4o", "/v1/responses", "max_output_tokens", ""},
		{"Responses alias", "openai-compat", "my-model-alias", "/v1/responses", "max_output_tokens", config.APIFormatResponses},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			mockOpenAIHTTP(t, func(r *http.Request, body map[string]any) string {
				requests++
				require.Equal(t, tt.path, r.URL.Path)
				require.Equal(t, tt.model, body["model"])
				require.EqualValues(t, 1024, body[tt.tokenField])
				for _, field := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
					if field != tt.tokenField {
						require.NotContains(t, body, field)
					}
				}
				require.Equal(t, "test-header", r.Header.Get("X-Test"))
				if tt.typ == "openai-compat" {
					require.Equal(t, "preserved", body["custom_field"])
				}
				if isNewGPTModel(tt.model) {
					require.NotContains(t, body, "temperature")
					if tt.path == "/v1/responses" {
						require.Equal(t, "max", body["reasoning"].(map[string]any)["effort"])
					} else {
						require.Equal(t, "max", body["reasoning_effort"])
					}
				}
				if tt.path == "/v1/responses" {
					return `{"id":"resp","object":"response","status":"completed","output":[{"type":"message","id":"msg","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}]}`
				}
				return `{"id":"chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
			})
			store, err := config.Load(t.TempDir(), "", nil, false)
			require.NoError(t, err)
			c := &coordinator{cfg: store}
			providerCfg := config.ProviderConfig{ID: "relay", Type: catwalk.Type(tt.typ), APIFormat: tt.format,
				BaseURL: "https://relay.invalid/v1", APIKey: "test",
				ExtraHeaders: map[string]string{"X-Test": "test-header"}, ExtraBody: map[string]any{"custom_field": "preserved"}}
			provider, err := c.buildProvider(providerCfg, config.SelectedModel{Model: tt.model}, false)
			require.NoError(t, err)
			model, err := provider.LanguageModel(t.Context(), tt.model)
			require.NoError(t, err)
			options := getProviderOptions(Model{CatwalkCfg: catwalk.Model{ID: tt.model}, ModelCfg: config.SelectedModel{ReasoningEffort: "max"}}, providerCfg)
			if !isNewGPTModel(tt.model) {
				options = nil
			}
			response, err := model.Generate(t.Context(), fantasy.Call{MaxOutputTokens: new(int64(1024)), Temperature: new(0.7), ProviderOptions: options})
			require.NoError(t, err)
			require.Equal(t, "ok", response.Content.Text())
			require.Equal(t, 1, requests)
			if isNewGPTModel(tt.model) {
				// Compatibility conversion must not consume caller-owned reasoning settings.
				encoded := string(mustJSON(t, options))
				require.Contains(t, encoded, `"reasoning_effort":"max"`)
			}
		})
	}
}

func TestNewGPTTokenOverrides(t *testing.T) {
	for _, format := range []config.APIFormat{config.APIFormatChatCompletions, config.APIFormatResponses} {
		t.Run(string(format), func(t *testing.T) {
			mockOpenAIHTTP(t, func(r *http.Request, body map[string]any) string {
				if format == config.APIFormatResponses {
					require.EqualValues(t, 4096, body["max_output_tokens"])
					require.NotContains(t, body, "max_tokens")
					require.NotContains(t, body, "max_completion_tokens")
					return `{"id":"resp","object":"response","status":"completed","output":[]}`
				}
				require.EqualValues(t, 3072, body["max_completion_tokens"])
				require.NotContains(t, body, "max_tokens")
				require.NotContains(t, body, "max_output_tokens")
				return `{"id":"chat","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
			})
			c := &coordinator{cfg: config.NewTestStore(nil)}
			provider, err := c.buildOpenaiCompatProvider("https://relay.invalid/v1", "test", nil, map[string]any{
				"max_tokens": 2048, "max_completion_tokens": 3072, "max_output_tokens": 4096,
			}, "relay", false, format)
			require.NoError(t, err)
			model, err := provider.LanguageModel(t.Context(), "gpt-6.1-sol")
			require.NoError(t, err)
			_, err = model.Generate(t.Context(), fantasy.Call{MaxOutputTokens: new(int64(1024))})
			require.NoError(t, err)
		})
	}
}

func TestNewGPTPerCallTokenLimits(t *testing.T) {
	limits := []int64{40, 2048}
	requests := 0
	mockOpenAIHTTP(t, func(r *http.Request, body map[string]any) string {
		require.Less(t, requests, len(limits))
		require.EqualValues(t, limits[requests], body["max_completion_tokens"])
		requests++
		return `{"id":"chat","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
	})
	c := &coordinator{cfg: config.NewTestStore(nil)}
	provider, err := c.buildOpenaiCompatProvider("https://relay.invalid/v1", "test", nil, nil, "relay", false, config.APIFormatChatCompletions)
	require.NoError(t, err)
	model, err := provider.LanguageModel(t.Context(), "gpt-6.1-sol")
	require.NoError(t, err)
	for _, limit := range limits {
		_, err := model.Generate(t.Context(), fantasy.Call{MaxOutputTokens: new(limit)})
		require.NoError(t, err)
	}
	require.Equal(t, len(limits), requests)
}

func TestOpenAIProtocolFailureDoesNotFallBack(t *testing.T) {
	requests := 0
	previous := http.DefaultTransport
	http.DefaultTransport = openAIRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		require.Equal(t, "/v1/responses", r.URL.Path)
		_ = r.Body.Close()
		return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Responses unsupported","type":"invalid_request_error"}}`)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	c := &coordinator{cfg: config.NewTestStore(nil)}
	provider, err := c.buildOpenaiCompatProvider("https://relay.invalid/v1", "test", nil, nil, "relay", false, config.APIFormatAuto)
	require.NoError(t, err)
	model, err := provider.LanguageModel(t.Context(), "gpt-6.1-sol")
	require.NoError(t, err)
	_, err = model.Generate(t.Context(), fantasy.Call{})
	require.ErrorContains(t, err, "Responses unsupported")
	require.Equal(t, 1, requests)
}

func TestOpenAIStreamingToolRoundTrip(t *testing.T) {
	for _, format := range []config.APIFormat{config.APIFormatChatCompletions, config.APIFormatResponses} {
		t.Run(string(format), func(t *testing.T) {
			requests := 0
			mockOpenAIHTTP(t, func(r *http.Request, body map[string]any) string {
				requests++
				require.True(t, body["stream"].(bool))
				if format == config.APIFormatResponses {
					require.Equal(t, "/v1/responses", r.URL.Path)
					if requests == 1 {
						return responseToolStream
					}
					require.Contains(t, string(mustJSON(t, body["input"])), "function_call_output")
					return responseTextStream
				}
				require.Equal(t, "/v1/chat/completions", r.URL.Path)
				require.NotContains(t, body, "max_tokens")
				if requests == 1 {
					return "data: {\"id\":\"c\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"value\\\":\\\"42\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n"
				}
				require.Contains(t, string(mustJSON(t, body["messages"])), `"role":"tool"`)
				return "data: {\"id\":\"c\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
			})
			c := &coordinator{cfg: config.NewTestStore(nil)}
			provider, err := c.buildOpenaiCompatProvider("https://relay.invalid/v1", "test", nil, nil, "relay", false, format)
			require.NoError(t, err)
			model, err := provider.LanguageModel(t.Context(), "gpt-6.1-sol")
			require.NoError(t, err)
			runner := fantasy.NewAgent(model, fantasy.WithTools(testCustomTool("lookup")))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result, err := runner.Stream(ctx, fantasy.AgentStreamCall{Prompt: "look up 42", MaxOutputTokens: new(int64(1024))})
			require.NoError(t, err)
			require.Equal(t, "done", result.Response.Content.Text())
			require.Equal(t, 2, requests)
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

const responseToolStream = `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc1","call_id":"call1","name":"lookup","arguments":""}}

data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc1","delta":"{\"value\":\"42\"}"}

data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc1","call_id":"call1","name":"lookup","arguments":"{\"value\":\"42\"}"}}

data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[]}}

`

const responseTextStream = `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"m1","role":"assistant","content":[]}}

data: {"type":"response.output_text.delta","item_id":"m1","output_index":0,"content_index":0,"delta":"done"}

data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"m1","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]}}

data: {"type":"response.completed","response":{"id":"r2","status":"completed","output":[]}}

`
