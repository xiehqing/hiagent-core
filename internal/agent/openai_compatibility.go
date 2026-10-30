package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"

	"github.com/xiehqing/hiagent-core/internal/config"
)

var gptGenerationPattern = regexp.MustCompile(`(?i)(?:^|/)gpt-(\d+)(?:[.-]|$)`)

func isNewGPTModel(modelID string) bool {
	match := gptGenerationPattern.FindStringSubmatch(modelID)
	if len(match) != 2 {
		return false
	}
	generation, _ := strconv.Atoi(match[1])
	return generation >= 6
}

func usesResponsesAPI(provider config.ProviderConfig, modelID string) bool {
	switch provider.APIFormat {
	case config.APIFormatResponses:
		return true
	case config.APIFormatChatCompletions:
		return false
	}
	if provider.Type == "azure" {
		return openai.IsResponsesModel(modelID)
	}
	if isNewGPTModel(modelID) {
		return true
	}
	return (provider.Type == openai.Name || provider.ID == "copilot") && openai.IsResponsesModel(modelID)
}

type openAIProvider struct{ fantasy.Provider }

func (p openAIProvider) LanguageModel(ctx context.Context, id string) (fantasy.LanguageModel, error) {
	model, err := p.Provider.LanguageModel(ctx, id)
	if err != nil || !isNewGPTModel(id) {
		return model, err
	}
	return openAIModel{LanguageModel: model}, nil
}

// Fantasy v0.24 supports explicit endpoint selection, but its reasoning model
// classification still stops at GPT-5. Carry per-call settings to the request
// transport without changing model IDs or modifying caller-owned options.
type openAIModel struct{ fantasy.LanguageModel }
type openAIRequestKey struct{}
type openAIRequestSettings struct {
	effort  string
	summary string
}

func (m openAIModel) prepare(ctx context.Context, call fantasy.Call) (context.Context, fantasy.Call) {
	settings := openAIRequestSettings{}
	call.ProviderOptions = maps.Clone(call.ProviderOptions)
	for key, options := range call.ProviderOptions {
		switch options := options.(type) {
		case *openai.ResponsesProviderOptions:
			copy := *options
			if copy.ReasoningEffort != nil {
				settings.effort = string(*copy.ReasoningEffort)
			}
			if copy.ReasoningSummary != nil {
				settings.summary = *copy.ReasoningSummary
			}
			copy.ReasoningEffort, copy.ReasoningSummary = nil, nil
			call.ProviderOptions[key] = &copy
		case *openai.ProviderOptions:
			copy := *options
			if copy.ReasoningEffort != nil {
				settings.effort = string(*copy.ReasoningEffort)
			}
			copy.ReasoningEffort = nil
			call.ProviderOptions[key] = &copy
		case *openaicompat.ProviderOptions:
			copy := *options
			if copy.ReasoningEffort != nil {
				settings.effort = string(*copy.ReasoningEffort)
			}
			copy.ReasoningEffort = nil
			call.ProviderOptions[key] = &copy
		}
	}
	// GPT-6 reasoning requests cannot use these sampling/penalty parameters.
	call.Temperature, call.TopP, call.FrequencyPenalty, call.PresencePenalty = nil, nil, nil, nil
	return context.WithValue(ctx, openAIRequestKey{}, settings), call
}

func (m openAIModel) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	ctx, call = m.prepare(ctx, call)
	return m.LanguageModel.Generate(ctx, call)
}

func (m openAIModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	ctx, call = m.prepare(ctx, call)
	return m.LanguageModel.Stream(ctx, call)
}

func (m openAIModel) prepareObject(ctx context.Context, call fantasy.ObjectCall) (context.Context, fantasy.ObjectCall) {
	ctx, prepared := m.prepare(ctx, fantasy.Call{ProviderOptions: call.ProviderOptions})
	call.ProviderOptions = prepared.ProviderOptions
	call.Temperature, call.TopP, call.FrequencyPenalty, call.PresencePenalty = nil, nil, nil, nil
	return ctx, call
}

func (m openAIModel) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	ctx, call = m.prepareObject(ctx, call)
	return m.LanguageModel.GenerateObject(ctx, call)
}

func (m openAIModel) StreamObject(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	ctx, call = m.prepareObject(ctx, call)
	return m.LanguageModel.StreamObject(ctx, call)
}

func withOpenAICompatibility(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	transport := copy.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	copy.Transport = openAICompatibilityTransport{base: transport}
	return &copy
}

type openAICompatibilityTransport struct{ base http.RoundTripper }

func (t openAICompatibilityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	settings, modern := request.Context().Value(openAIRequestKey{}).(openAIRequestSettings)
	if !modern || request.Body == nil {
		return t.base.RoundTrip(request)
	}
	data, err := io.ReadAll(request.Body)
	_ = request.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read OpenAI request: %w", err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("decode OpenAI request: %w", err)
	}
	if strings.HasSuffix(request.URL.Path, "/chat/completions") {
		normalizeTokenLimit(body, "max_completion_tokens", "max_tokens", "max_output_tokens")
		if settings.effort != "" {
			if _, exists := body["reasoning_effort"]; !exists {
				body["reasoning_effort"], _ = json.Marshal(settings.effort)
			}
		}
	} else if strings.HasSuffix(request.URL.Path, "/responses") {
		normalizeTokenLimit(body, "max_output_tokens", "max_completion_tokens", "max_tokens")
		reasoning := map[string]json.RawMessage{}
		if existing := body["reasoning"]; len(existing) > 0 && string(existing) != "null" {
			if err := json.Unmarshal(existing, &reasoning); err != nil {
				return nil, fmt.Errorf("decode OpenAI reasoning options: %w", err)
			}
		}
		if _, exists := reasoning["effort"]; !exists {
			if legacy := body["reasoning_effort"]; len(legacy) > 0 {
				reasoning["effort"] = legacy
			} else if settings.effort != "" {
				reasoning["effort"], _ = json.Marshal(settings.effort)
			}
		}
		delete(body, "reasoning_effort")
		if _, exists := reasoning["summary"]; !exists && settings.summary != "" {
			reasoning["summary"], _ = json.Marshal(settings.summary)
		}
		if len(reasoning) > 0 {
			body["reasoning"], _ = json.Marshal(reasoning)
		}
	}
	// Apply these after SDK extra_body overrides as well.
	for _, field := range []string{"temperature", "top_p", "frequency_penalty", "presence_penalty"} {
		delete(body, field)
	}
	data, err = json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode OpenAI request: %w", err)
	}
	copy := request.Clone(request.Context())
	copy.Body = io.NopCloser(bytes.NewReader(data))
	copy.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
	copy.ContentLength = int64(len(data))
	copy.Header.Del("Content-Length")
	return t.base.RoundTrip(copy)
}

func normalizeTokenLimit(body map[string]json.RawMessage, native string, alternatives ...string) {
	for _, alternative := range alternatives {
		if current := body[native]; len(current) == 0 || string(current) == "null" {
			if value := body[alternative]; len(value) > 0 && string(value) != "null" {
				body[native] = value
			}
		}
		delete(body, alternative)
	}
}
