package config

import "fmt"

// APIFormat selects the wire protocol independently of the provider catalog.
type APIFormat string

const (
	APIFormatAuto            APIFormat = "auto"
	APIFormatChatCompletions APIFormat = "chat-completions"
	APIFormatResponses       APIFormat = "responses"
)

// ValidateAPIFormat rejects typos and protocol overrides on unrelated providers.
func (p ProviderConfig) ValidateAPIFormat() error {
	switch p.APIFormat {
	case "", APIFormatAuto:
		return nil
	case APIFormatChatCompletions, APIFormatResponses:
		if p.Type == "" || p.Type == "openai" || p.Type == "openai-compat" || p.Type == "hyper" {
			return nil
		}
		return fmt.Errorf("provider %q: api_format is not supported for type %q", p.ID, p.Type)
	default:
		return fmt.Errorf("provider %q: invalid api_format %q (use auto, chat-completions, or responses)", p.ID, p.APIFormat)
	}
}
