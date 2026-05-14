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

	_, err = app.SubmitMessage(context.Background(), "帮我审查一下这个项目", "", false)
	if err != nil {
		log.Fatal(err)
	}
}
```

常用 SDK 选项包括：

- `WithWorkDir(...)`
- `WithDataDir(...)`
- `WithDatabaseDriver(...)`
- `WithDatabaseDSN(...)`
- `WithSkipPermissionRequests(...)`
- `WithSelectedProvider(...)`
- `WithSelectedModel(...)`
- `WithConfigScope(...)`
- `WithAdditionalSystemPrompt(...)`

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
