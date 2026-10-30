# HiAgent Core

HiAgent Core 是一个基于 Go 构建的终端优先 AI 编码助手与运行时核心，主要提供以下能力：

- `hiagent` 命令行工具，支持交互式和非交互式 AI 工作流
- 本地模式与服务端模式 API，可管理 workspace、session、config、LSP、MCP 与权限
- 可嵌入的 `appsdk`，方便在你自己的 Go 服务中复用运行时能力
- 基于 SQLite 或 MySQL 的持久化存储

整个项目采用 workspace 作为核心上下文，配置、模型提供商、会话、消息、文件历史以及 agent 执行都围绕 workspace 展开。

## 功能概览

- 交互式 TUI 编码助手
- 非交互式 `hiagent run` 执行模式
- Workspace 与 Session 生命周期管理
- 基于数据库的 provider 与 model 配置
- MCP 与 LSP 集成
- 具备权限控制的工具执行机制
- 可供本地或远程调用的 HTTP API
- 可嵌入其他应用的 Go SDK

## 目录结构

- [main.go](/C:/zorktech/projects/backen/hiagentgo/main.go)：CLI 入口
- [internal/cmd](/C:/zorktech/projects/backen/hiagentgo/internal/cmd)：命令实现，例如 `run`、`server`、`login`、`session`、`stats`
- [internal/app](/C:/zorktech/projects/backen/hiagentgo/internal/app)：核心应用运行时
- [internal/config](/C:/zorktech/projects/backen/hiagentgo/internal/config)：配置加载、持久化、provider/model 解析
- [internal/db](/C:/zorktech/projects/backen/hiagentgo/internal/db)：数据库连接、迁移、查询层
- [internal/server](/C:/zorktech/projects/backen/hiagentgo/internal/server)：HTTP 服务与控制器
- [internal/workspace](/C:/zorktech/projects/backen/hiagentgo/internal/workspace)：workspace 抽象
- [pkg/appsdk](/C:/zorktech/projects/backen/hiagentgo/pkg/appsdk)：可嵌入式 Go SDK

## 环境要求

- Go `1.26.3`
- SQLite 或 MySQL
- 至少一个已配置好的模型提供商

## 编译

```bash
go build -o hiagent .
```

Windows PowerShell:

```powershell
go build -o hiagent.exe .
```

## 运行方式

启动交互式 TUI：

```bash
go run .
```

非交互式执行单条提示：

```bash
go run . run "解释一下这个仓库的结构"
```

指定工作目录和数据目录：

```bash
go run . --cwd /path/to/project --data-dir /path/to/.hiagent
```

使用 MySQL：

```bash
go run . --driver mysql --dsn "user:password@tcp(localhost:3306)/hiagent"
```

## 服务端模式

启动服务：

```bash
go run . server
```

默认监听地址：

- Unix: `unix:///tmp/hiagent-<uid>.sock`
- Windows: `npipe:////./pipe/hiagent-<uid>.sock`

也可以显式指定：

```bash
go run . server --host tcp://127.0.0.1:8080
```

服务端 API 主要覆盖：

- health 与 version
- workspace 创建、查询、删除
- session 与 message 历史
- provider 列表
- LSP 与 MCP 操作
- 权限控制
- 配置读写

可以参考 [internal/server/server.go](/C:/zorktech/projects/backen/hiagentgo/internal/server/server.go) 以及 [internal/swagger](/C:/zorktech/projects/backen/hiagentgo/internal/swagger) 下的 swagger 产物。

## 配置与存储

项目同时支持 SQLite 和 MySQL，统一通过 [internal/db](/C:/zorktech/projects/backen/hiagentgo/internal/db) 管理数据库连接与迁移。

当前持久化数据主要包括：

- session
- message
- 文件历史
- provider catalog
- model catalog
- config 记录

仓库中的 `internal/config` 也在逐步收敛到 DB 驱动的配置存储模型。

## 常用命令

交互式运行：

```bash
hiagent
```

非交互式执行：

```bash
hiagent run "总结一下最近的改动"
```

继续某个会话：

```bash
hiagent --session <session-id>
hiagent run --session <session-id> "继续上一次的话题"
```

启动服务：

```bash
hiagent server
```

生成配置 JSON Schema：

```bash
hiagent schema
```

## 使用 Go SDK

SDK 位于 [pkg/appsdk](/C:/zorktech/projects/backen/hiagentgo/pkg/appsdk)。

一个最小示例：

```go
package main

import (
	"context"
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
	"github.com/xiehqing/hiagent-core/pkg/appsdk"
)

func main() {
	conn, err := sql.Open("sqlite", `C:\project\.hiagent\hi_agent.db`)
	if err != nil {
		log.Fatal(err)
	}
	if err := appsdk.AutoMigrations(conn); err != nil {
		log.Fatal(err)
	}
	app, err := appsdk.New(context.Background(), conn,
		appsdk.WithWorkDir(`C:\project`),
		appsdk.WithDataDir(`C:\project\.hiagent`),
		appsdk.WithSkipPermissionRequests(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Shutdown()

	_, err = app.SubmitMessage(context.Background(), "帮我审查一下这个项目", "", false)
	if err != nil {
		log.Fatal(err)
	}
}
```

常用 SDK 选项包括：

- `WithWorkDir(...)`
- `WithDataDir(...)`
- `WithSkipPermissionRequests(...)`
- `WithSelectedProvider(...)`
- `WithSelectedModel(...)`
- `WithAdditionalSystemPrompt(...)`
- `WithTools(...)`

传入的数据库连接需要完成初始化并配置好 provider/model。
`App.Shutdown()` 会关闭该连接。

自定义工具实现 `charm.land/fantasy.AgentTool` 即可。使用
`fantasy.NewAgentTool` 可以根据输入结构体自动生成 JSON Schema：

```go
greet := fantasy.NewAgentTool("greet", "根据姓名打招呼",
	func(ctx context.Context, input struct {
		Name string `json:"name" description:"对方姓名"`
	}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse("你好，" + input.Name), nil
	},
)

app, err := appsdk.New(ctx, conn,
	appsdk.WithWorkDir(workDir),
	appsdk.WithTools(greet),
)
```

该示例需要导入 `charm.land/fantasy`。多次调用 `WithTools` 会追加工具。
自定义工具仅向主 Agent 开放，在模型/工具刷新和 Agent 重新初始化后仍然保留，
作为运行时依赖保存，不写入配置。`options.disabled_tools` 同样适用于自定义工具名。
nil 工具、空名称、自定义工具重名，以及与已启用的内置/MCP 工具重名都会返回错误。
自定义工具接入现有 PreToolUse hooks；处理函数仍需自行处理鉴权、取消和共享业务状态的同步。
provider 缓存参数与调用方的工具对象隔离。初始化后应保持工具名称和 Schema 稳定，
处理函数依赖资源的生命周期由调用方管理。

## OpenAI 协议选择

`openai` 和 `openai-compat` provider 支持 `api_format`：

| 值 | 行为 |
|---|---|
| `auto`（默认） | GPT-6 及后续代际使用 Responses；其他模型保持原有路由规则 |
| `chat-completions` | 使用 `/chat/completions`，供只支持 Chat 的中转使用 |
| `responses` | 使用 `/responses`，也适用于中转自定义的模型别名 |

配置保存在数据库 `data_config` 的 global/workspace JSON 记录中。已有 provider
可以增加以下配置片段，工作区配置优先于全局配置：

```json
{"providers":{"my-relay":{"api_format":"responses"}}}
```

SDK 可按实例覆盖协议，覆盖会保留到配置重载后，但不写入数据库：

```go
app, err := appsdk.New(ctx, conn,
	appsdk.WithWorkDir(workDir),
	appsdk.WithProviderAPIFormat("my-relay", appsdk.APIFormatResponses),
)
```

只支持 Chat 的中转可改用 `appsdk.APIFormatChatCompletions`。GPT-6 系列在 Chat
模式下发送 `max_completion_tokens`，Responses 模式下发送 `max_output_tokens`，
同时处理推理参数；第三方旧模型原有的 `max_tokens` 行为保持不变。

GPT-6.1 Sol 的官方工具调用要求 Responses API。若强制 Chat 模式，需要中转服务
自行支持工具协议转换。不会在请求失败后自动切换协议重试。

## 开发说明

运行局部测试：

```bash
go test ./internal/config
go test ./pkg/appsdk -run '^$'
```

运行全部测试：

```bash
go test ./...
```

注意：仓库里有些测试依赖具体环境，例如外部 MySQL 服务，因此在本地或 CI 中可能需要额外准备。

## 补充说明

- 运行时 provider 是否可用，不仅取决于 provider catalog，也取决于最终配置是否完整。
- 如果某个 provider 虽然存在于数据库中，但缺少 `api_key` 或 endpoint 等关键配置，它可能会在运行时被跳过。
- 在 server/client 模式下，模型覆盖与配置写入是通过 workspace API 完成的，而不是直接修改本地内存状态。

## 许可证

请以仓库中的许可证文件或上游项目许可证说明为准。
