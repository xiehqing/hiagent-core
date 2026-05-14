# HiAgent Core

HiAgent Core is a terminal-first AI coding assistant and application runtime built in Go. It provides:

- A `hiagent` CLI for interactive and non-interactive AI workflows
- A local/server mode API for workspace, session, config, LSP, MCP, and permission management
- A reusable `appsdk` package for embedding the runtime into your own Go services
- Persistent storage backed by SQLite or MySQL

The codebase is based around a workspace-centric model: configuration, providers, sessions, messages, file history, and agent execution all hang off a workspace context.

## Features

- Interactive TUI for coding tasks and project conversations
- Non-interactive `hiagent run` mode for scripting and automation
- Workspace/session lifecycle management
- Provider and model configuration backed by database tables
- MCP and LSP integration
- Permission-gated tool execution
- Built-in HTTP API for local or remote control
- Go SDK for embedding the runtime in other applications

## Project Layout

- [main.go](/C:/zorktech/projects/backen/hiagentgo/main.go): CLI entrypoint
- [internal/cmd](/C:/zorktech/projects/backen/hiagentgo/internal/cmd): Cobra commands such as `run`, `server`, `login`, `session`, `stats`
- [internal/app](/C:/zorktech/projects/backen/hiagentgo/internal/app): core application runtime
- [internal/config](/C:/zorktech/projects/backen/hiagentgo/internal/config): configuration loading, persistence, provider/model resolution
- [internal/db](/C:/zorktech/projects/backen/hiagentgo/internal/db): database connections, migrations, generated query layer
- [internal/server](/C:/zorktech/projects/backen/hiagentgo/internal/server): HTTP server and controller endpoints
- [internal/workspace](/C:/zorktech/projects/backen/hiagentgo/internal/workspace): workspace abstraction for local/app/client usage
- [pkg/appsdk](/C:/zorktech/projects/backen/hiagentgo/pkg/appsdk): embeddable Go SDK

## Requirements

- Go `1.26.3`
- SQLite or MySQL
- A configured model provider in the database/config store

## Build

```bash
go build -o hiagent .
```

On Windows PowerShell:

```powershell
go build -o hiagent.exe .
```

## Run

Start the interactive TUI:

```bash
go run . 
```

Run a single prompt non-interactively:

```bash
go run . run "Explain this repository structure"
```

Use a custom working directory and data directory:

```bash
go run . --cwd /path/to/project --data-dir /path/to/.hiagent
```

Use MySQL instead of SQLite:

```bash
go run . --driver mysql --dsn "user:password@tcp(localhost:3306)/hiagent"
```

## Server Mode

Start the API server:

```bash
go run . server
```

By default the server binds to:

- Unix: `unix:///tmp/hiagent-<uid>.sock`
- Windows: `npipe:////./pipe/hiagent-<uid>.sock`

You can override the host with:

```bash
go run . server --host tcp://127.0.0.1:8080
```

The API exposes endpoints for:

- health and version
- workspace creation and inspection
- session history and messages
- provider listing
- LSP and MCP operations
- permission controls
- config mutation

See [internal/server/server.go](/C:/zorktech/projects/backen/hiagentgo/internal/server/server.go) and the generated swagger artifacts under [internal/swagger](/C:/zorktech/projects/backen/hiagentgo/internal/swagger).

## Configuration and Storage

The runtime supports both SQLite and MySQL through the shared DB layer in [internal/db](/C:/zorktech/projects/backen/hiagentgo/internal/db).

Important persisted data includes:

- sessions
- messages
- file history
- provider catalog
- model catalog
- config records

Recent changes in this repository also move more config behavior toward DB-backed storage under `internal/config`.

## Common Commands

Interactive mode:

```bash
hiagent
```

Non-interactive prompt:

```bash
hiagent run "Summarize the latest changes"
```

Continue a session:

```bash
hiagent --session <session-id>
hiagent run --session <session-id> "Follow up on the previous answer"
```

Start the server:

```bash
hiagent server
```

Generate the config JSON schema:

```bash
hiagent schema
```

## Using the Go SDK

The SDK lives in [pkg/appsdk](/C:/zorktech/projects/backen/hiagentgo/pkg/appsdk).

Minimal example:

```go
package main

import (
	"context"
	"log"

	"github.com/xiehqing/hiagent-core/internal/config"
	"github.com/xiehqing/hiagent-core/pkg/appsdk"
)

func main() {
	app, err := appsdk.New(context.Background(),
		appsdk.WithWorkDir(`C:\project`),
		appsdk.WithDataDir(`C:\project\.hiagent`),
		appsdk.WithSkipPermissionRequests(true),
		appsdk.WithConfigScope(config.ScopeWorkspace),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Shutdown()

	_, err = app.SubmitMessage(context.Background(), "Help me review this project", "", false)
	if err != nil {
		log.Fatal(err)
	}
}
```

Useful SDK options include:

- `WithWorkDir(...)`
- `WithDataDir(...)`
- `WithDatabaseDriver(...)`
- `WithDatabaseDSN(...)`
- `WithSkipPermissionRequests(...)`
- `WithSelectedProvider(...)`
- `WithSelectedModel(...)`
- `WithConfigScope(...)`
- `WithAdditionalSystemPrompt(...)`

## Development

Run focused tests:

```bash
go test ./internal/config
go test ./pkg/appsdk -run '^$'
```

Run the full suite:

```bash
go test ./...
```

Note: some tests in this repository are environment-dependent and may expect external services such as MySQL to be available.

## Notes

- Provider availability at runtime depends on both the provider catalog and effective configuration.
- If a provider exists in the database but is missing required runtime settings such as `api_key` or endpoint, it may be skipped during provider configuration.
- In server/client mode, model overrides and config writes flow through the workspace APIs rather than directly mutating local state.

## License

Please refer to the repository license file if present in your distribution or upstream source.
