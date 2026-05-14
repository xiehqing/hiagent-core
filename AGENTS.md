# Crush (hiagent) Codebase Guide

This document captures non-obvious patterns, architecture decisions, and gotchas for AI agents working in this codebase.

## Overview

Crush (`/hiagent`) is a terminal-first AI coding assistant. It can run interactively (Bubble Tea TUI) or non-interactively (`crush run`), and supports a client/server split (`CRUSH_CLIENT_SERVER=1`). The module path is `github.com/xiehqing/hiagent-core`.

The project uses [Bubble Tea v2](https://charm.land/bubbletea/v2), [Lip Gloss v2](https://charm.land/lipgloss/v2), [Fantasy](https://charm.land/fantasy) (AI agent framework), [Catwalk](https://charm.land/catwalk/pkg/catwalk) (provider catalog), [Ultraviolet](https://github.com/charmbracelet/ultraviolet) (terminal screen buffer), and [Cobra](https://github.com/spf13/cobra) for CLI.

## Essential Commands

| Action | Command |
|--------|---------|
| Build | `go build .` |
| Test package | `go test ./internal/<pkg>/...` |
| Test all | `go test ./...` (slow; many tests) |
| Update providers | `go run . update-providers` |
| Run interactive | `go run .` |
| Run non-interactive | `go run . run "prompt"` |
| Start server | `go run . server` |
| Check log capitalization | `./scripts/check_log_capitalization.sh` |

Test files are numerous — 60+ scattered across packages. The DB package (`internal/db/`) uses sqlc-generated code (`*sql.go` files). Some tests use test stores (`internal/config/test_store.go`).

## Project Structure

```
main.go                  — Entry point (pprof, cmd.Execute())
internal/
  app/                   — Wires services; main App struct (lifecycle coordinator)
  agent/                 — AI agent orchestration (Coordinator, SessionAgent)
    tools/               — Individual LLM tool implementations + MCP integration
    templates/           — Title/summary prompt templates (embedded via go:embed)
  cmd/                   — Cobra commands (root, run, server, dirs, session, etc.)
  config/                — Config loading/merging (files + DB; hybrid state)
  server/                — HTTP server (Unix socket / named pipe) + v1 API handlers
  client/                — HTTP client SDK for server mode
  workspace/             — Workspace interface (local AppWorkspace vs remote ClientWorkspace)
  ui/                    — Bubble Tea TUI (see internal/ui/AGENTS.md)
  db/                    — SQL queries (sqlc-generated), DB connection
  session/               — Session service and model
  message/               — Message service and content parts
  provider/              — Provider service
  permission/            — Permission request service
  lsp/                   — LSP manager and client
  hooks/                 — Hook runner (pre/post tool use hooks)
  skills/                — Agent skill loading
  mcp/                   — MCP tool integration (via go-sdk)
  pubsub/                — Generic pub/sub broker
  csync/                 — Concurrent-safe wrappers (Value, Map, Slice)
  event/                 — Telemetry/metrics events
  shell/                 — Shell command execution (background jobs)
  fsext/                 — File system extensions (lookup, ls, paste, ignore)
  filepathext/           — File path pattern matching
  filetracker/           — Tracks which files an agent has read
  history/               — File history service (for file operations)
  format/                — Spinner animation
  diff/                  — Diff utilities
  diffdetect/            — Language detection for diffs
  stringext/             — String utilities (capitalization, base64, etc.)
  ansiext/               — ANSI handling extensions
  home/                  — Home directory utilities
  env/                   — Environment variable resolution
  oauth/                 — OAuth token management + Copilot integration
  projects/              — Project registration
  version/               — Version info
  update/                — Update checking
  log/                   — Log setup (lumberjack rotation)
proto/                   — Protocol-level types (shared between internal/client and internal/server)
pkg/
  appsdk/                — App SDK (external-facing programmatic API)
scripts/
  check_log_capitalization.sh  — Linter: verifies slog log messages start with capital letters
  run-labeler.sh               — Label runner script
.hi_agent/               — Default data directory (created at runtime)
```

## Architecture & Data Flow

### Two Deployment Modes

1. **Local (default)**: Everything in-process. `main.go` → `cmd.setupLocalWorkspace()` → creates `app.App` → wraps in `workspace.AppWorkspace`.
2. **Client/Server** (`CRUSH_CLIENT_SERVER=1`): CLI connects to a detached server via Unix socket or Windows named pipe. `cmd.setupClientServerWorkspace()` → `client.Client` + `workspace.ClientWorkspace` → HTTP API to server.

### Workspace Interface

The `workspace.Workspace` interface (`internal/workspace/workspace.go`) is the central abstraction consumed by the TUI and CLI. It groups every operation a frontend needs, regardless of deployment mode:
- Session CRUD
- Message listing
- Agent run/cancel/queue/summarize
- Permission grant/deny
- File tracker read tracking
- History listing
- LSP start/stop/state
- Config mutations (model, compact mode, API keys, etc.)
- MCP operations (states, refresh prompts/resources/tools, read resource)
- Project lifecycle (needs init, mark init, init prompt)

### App Lifecycle (`internal/app/app.go`)

`app.New()` wires all services and starts them:
1. Creates DB connection via `db.ConnectWithOption()`
2. Loads config via `config.Init()`
3. Creates `app.App` with all services (Sessions, Messages, Providers, History, Permissions, FileTracker, LSPManager)
4. Sets up event bus: subscribes all services to `pubsub.Broker[tea.Msg]`
5. Starts MCP initialization and LSP tracking in background goroutines
6. Initializes the coder agent coordinator

`app.Shutdown()` gracefully cancels all agents first, then runs cleanup in parallel (background shells, LSP clients, DB connection, MCP).

### Agent System

- `Coordinator` (`internal/agent/coordinator.go`) — top-level orchestrator; creates the coder `SessionAgent`, bridges config/fantasy models, manages tool registration (built-in tools + MCP tools + skills)
- `SessionAgent` (`internal/agent/agent.go`) — per-session agent; handles queuing, auto-summarization, tool call/result lifecycle, streaming callbacks
- Uses `charm.land/fantasy` under the hood with provider implementations for OpenAI, Anthropic, Google, Bedrock, OpenRouter, Vercel, Azure, and custom openai-compatible
- Tool execution is handled via `fantasy.AgentTool` interface

### Configuration System (`internal/config/`)

**⚠️ Important**: The config system is in a **hybrid file/DB migration** state. See `internal/config/db_migration_review.md` for the full audit.

- Config is loaded from multiple sources: embedded defaults, provider cache files, custom provider JSON files, DB records (`data_config`, `providers` tables)
- `ConfigStore` wraps `*config.Config` with overrides, staleness detection, and scope-aware write operations
- `Scope` enum distinguishes `ScopeGlobal` vs `ScopeWorkspace`
- Key paths:
  - Global config: OS-specific config dir (e.g., `~/.config/hi_agent/`)
  - Data directory: defaults to `.hi_agent/` in CWD
  - Server log: `<dataDir>/logs/hi_agent.log`
- Providers come from: embedded catalog, DB `providers`/`big_models` tables, provider cache files, custom provider JSON files

### Server API

The server (`internal/server/`) exposes a REST API over Unix socket or Windows named pipe:
- Path-based routing via Go 1.22+ `http.ServeMux` patterns (e.g., `"POST /v1/workspaces/{id}/sessions"`)
- Swagger docs at `/v1/docs/`
- Controller: `controllerV1` in `proto.go`
- Events: SSE-based event streaming for TUI updates

### Event System

The `pubsub.Broker` (`internal/pubsub/broker.go`) is the backbone for event propagation:
- Generic typed broker (`pubsub.Broker[T]`)
- Services expose a `Subscribe(ctx) <-chan Event[T]` method
- `app.setupEvents()` bridges all service subscriptions into a single `pubsub.Broker[tea.Msg]`
- The TUI subscribes via `app.Subscribe(program)` which pipes events into Bubble Tea's message loop

## Key Conventions

### Code Style

- **Log messages must start with a capital letter**. Enforced by `scripts/check_log_capitalization.sh` which greps for `slog.(Error|Info|Warn|Debug).*("[a-z]'`. This is a hard CI-style lint rule.
- **Error variables**: follow Go convention: `errXxx` for package-level sentinel errors (see `internal/agent/agent.go` line ~54: `ErrEmptyPrompt`, `ErrSessionMissing`; `internal/agent/coordinator.go` line ~54: `errCoderAgentNotConfigured`)
- **Comments**: package-level doc comments describe the package's purpose and non-obvious semantics (see `internal/agent/agent.go` line 1, `internal/app/app.go` line 1)
- **Import grouping**: standard library first, then charm/land, then external, then internal — with blank line separators between groups
- **Platform-specific files**: use `_windows.go`, `_other.go`, `_darwin.go` suffixes (see `internal/cmd/root_windows.go`, `internal/server/net_windows.go`)
- **Embedded files**: templates and config via `//go:embed` (see `internal/agent/templates/`, `internal/cmd/gitignore/`)

### UI Conventions

The TUI has a comprehensive set of development instructions in `internal/ui/AGENTS.md`. Key highlights:
- **Hybrid rendering**: Ultraviolet screen buffer + string-based sub-components
- **No IO in Update**: always use `tea.Cmd` for side effects
- **Centralized message handling**: the main `UI` model is the sole Bubble Tea model; sub-components use imperative methods, not standard Elm Update
- **Focus state routing**: `uiFocusEditor` vs `uiFocusMain`
- **Lazy rendering**: `list.List` only renders visible items
- Use `github.com/charmbracelet/x/ansi` for ANSI-safe string manipulation — never manipulate at byte level

### Testing Patterns

- Table-driven tests with `stretchr/testify` assertions (assert, require)
- Test files co-located with source (`*_test.go` in same package)
- DB tests use `db.ConnectWithOption` with sqlite or test helpers
- Config tests use `config/test_store.go` for isolated config state
- Some tests use `go.uber.org/goleak` for goroutine leak detection
- UI tests use catwalk for golden file testing (see `internal/ui/model/ui_test.go`)

## Non-Obvious Gotchas

1. **Early slog discarding**: `cmd.Execute()` sets slog to a discard handler at startup because `config.Load` calls slog before the file-based logger is set up. The log file path depends on the data directory, which isn't known until after config loading. (See comment in `internal/cmd/root.go:167-173`. Proper fix would remove slog calls from config.Load entirely.)

2. **Heartbit version trick**: The version template prepends a colored heartbit ASCII art via a hack: a `colorprofile.Writer` writes to a buffer, then the buffer content is prepended to cobra's version template. Cobra doesn't expose a PreRun for version, so this is done at init time.

3. **Progress bar hack**: Non-interactive mode reinitializes the terminal progress bar (`ansi.SetIndeterminateProgressBar`) on every loop iteration to prevent the terminal from hiding it due to inactivity.

4. **Anthropic caching**: Caching is enabled by default for Anthropic/Bedrock models but can be disabled via `CRUSH_DISABLE_ANTHROPIC_CACHE=1`.

5. **Provider media limitation workaround**: OpenAI/Google providers don't support images in tool result messages. The agent converts media in tool results to a text placeholder + a subsequent user message with the image as a file attachment. Anthropic and Bedrock support images natively, so the workaround is skipped for them.

6. **Orphaned tool call/result handling**: The agent filters orphaned tool results (results without matching calls) and injects synthetic error tool results for orphaned tool calls. Without this, interrupted sessions permanently lock the conversation.

7. **Context file detection**: The app searches for context files (`.cursorrules`, `CLAUDE.md`, `AGENTS.md`, etc.) automatically. See `defaultContextPaths` in `internal/config/config.go:28-49` for the full list. More than 20 variants are checked.

8. **Session hash prefix resolution**: Sessions can be referenced by UUID, full XXH3 hash, or hash prefix (short enough to be unique). `session.HashID()` uses XXH3. This works across both local and client/server modes.

9. **Client/server socket race**: When creating a client workspace in server mode, the socket may exist before the HTTP handler is ready. The client retries `CreateWorkspace` 5 times with 200ms backoff.

10. **Version mismatch handling**: `ensureServer()` checks if the running server version matches the client; on mismatch, it shuts down the old server and starts a fresh one. The old socket is force-removed after 2 seconds of waiting.

11. **Non-interactive session auto-approval**: When running via `crush run`, all permission requests for that session are auto-approved (`app.Permissions.AutoApproveSession(sess.ID)`).

12. **Small model fallback for title generation**: Title generation tries the small model first; if that fails, it falls back to the large model. If both fail, it uses "Untitled Session".

13. **Auto-summarize thresholds**: Contexts >200k tokens use a 20k token buffer; smaller contexts use 20% of the window as threshold. Controlled by `largeContextWindowThreshold` and `smallContextWindowRatio` in `internal/agent/agent.go:53-55`.

14. **Config staleness**: The `ConfigStore` tracks file staleness via file size and ModTime snapshots. This doesn't work with DB-backed config (part of the ongoing DB migration issue).

15. **Workspace config file override**: Despite the DB migration, workspace config overrides are still read from a separate JSON file (`crush.json` or `hiagent.json`) in the working directory. Global config comes from DB. This dual-source approach leads to priority ambiguity.

16. **MCP instruction injection**: MCP server initialization results include instructions that get injected into the system prompt between `<mcp-instructions>` tags.

17. **spinner uses stderr**: The non-interactive spinner writes to stderr, not stdout. This ensures piped output (`crush run ... > file`) doesn't capture spinner animations.

18. **windows named pipes**: On Windows, the server uses `npipe:////./pipe/crush-{uid}.sock`. On Unix, it uses `unix:///tmp/crush-{uid}.sock`.

## Library Notice

All internal Charm libraries use the `charm.land` vanity domain (not `github.com/charmbracelet`), e.g., `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/fantasy`, `charm.land/catwalk/pkg/catwalk`. The `github.com/charmbracelet/x/*` packages provide ANSI, term, and exp utilities.
