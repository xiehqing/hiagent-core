package agent

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"charm.land/fantasy"
)

// ValidateCustomTools rejects nil tools, empty names, and duplicate custom names.
// Conflicts with enabled built-in/MCP tools are checked when tools are assembled.
func ValidateCustomTools(customTools []fantasy.AgentTool) error {
	seen := make(map[string]struct{}, len(customTools))
	for i, tool := range customTools {
		if tool == nil {
			return fmt.Errorf("custom tool at index %d is nil", i)
		}
		switch v := reflect.ValueOf(tool); v.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
			if v.IsNil() {
				return fmt.Errorf("custom tool at index %d is nil", i)
			}
		}
		name := tool.Info().Name
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("custom tool at index %d has an empty name", i)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate custom tool name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func mergeCustomTools(existing, customTools []fantasy.AgentTool, disabled []string) ([]fantasy.AgentTool, error) {
	if len(customTools) == 0 {
		return existing, nil
	}
	if err := ValidateCustomTools(customTools); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(existing)+len(customTools))
	for _, tool := range existing {
		seen[tool.Info().Name] = struct{}{}
	}
	merged := slices.Clone(existing)
	for _, tool := range customTools {
		name := tool.Info().Name
		if slices.Contains(disabled, name) {
			continue
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("custom tool name %q conflicts with an enabled tool", name)
		}
		seen[name] = struct{}{}
		merged = append(merged, &customTool{
			AgentTool: tool,
			options:   maps.Clone(tool.ProviderOptions()),
		})
	}
	return merged, nil
}

// customTool keeps provider cache options local to this tool-list build. The
// session agent can set these options without mutating the SDK caller's tool.
type customTool struct {
	fantasy.AgentTool
	mu      sync.RWMutex
	options fantasy.ProviderOptions
}

func (t *customTool) ProviderOptions() fantasy.ProviderOptions {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return maps.Clone(t.options)
}

func (t *customTool) SetProviderOptions(options fantasy.ProviderOptions) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.options = maps.Clone(options)
}
