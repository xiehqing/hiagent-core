package config

import "fmt"

const (
	globalConfigKey       = "global"
	workspaceConfigPrefix = "workspace:"
)

// Scope determines which config record is targeted for read/write operations.
type Scope int

const (
	// ScopeGlobal targets the global config record.
	ScopeGlobal Scope = iota
	// ScopeWorkspace targets the workspace config record.
	ScopeWorkspace
)

// String returns a human-readable label for the scope.
func (s Scope) String() string {
	switch s {
	case ScopeGlobal:
		return "global"
	case ScopeWorkspace:
		return "workspace"
	default:
		return fmt.Sprintf("Scope(%d)", int(s))
	}
}

// ErrNoWorkspaceConfig is returned when a workspace-scoped operation is
// attempted without a workspace config key.
var ErrNoWorkspaceConfig = fmt.Errorf("no workspace config key configured")
