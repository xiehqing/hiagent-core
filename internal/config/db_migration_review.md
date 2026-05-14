# Config DB Migration Review

目标：梳理 `internal/config` 目录下，从“文件配置/文件缓存”切换到“DB 配置/DB 存储”时仍未完成或存在不一致的点。

范围：仅记录问题、影响、建议方案；本文件不包含真实代码修改。

## 一、必须修改的问题

### 1. `load.go` 的加载入口仍然混用文件模型和 DB 模型

位置：

- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:40)
- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:62)
- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:207)
- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:229)

现状：

- `Load()` 里已经开始调用 `loadFromDB(db)`。
- 但它仍然先计算 `lookupConfigs(workingDir)`。
- 仍然保留 `workspacePath`，并尝试 `os.ReadFile(store.workspacePath)` 做 workspace 配置覆盖。
- `loadFromConfigPaths()`、`lookupConfigs()`、`GlobalConfig()`、`GlobalConfigData()` 仍然是文件模型思路。

问题：

- 当前启动流程并没有完全切到 DB。
- 同一次 `Load()` 里，global 配置来自 DB，但 workspace 配置仍来自文件。
- 数据源优先级已经不清晰，后续 reload 和写回会出现不一致。

建议：

- 明确新的配置分层模型，例如：
  - `global`
  - `workspace:<abs-working-dir>`
- `Load()` 只从 DB 读取，不再从配置文件读 workspace override。
- 若仍要保留文件兼容，需要显式定义“兼容导入阶段”和“正式 DB 模式”两套路径，而不是混在同一个 `Load()` 里。

### 2. `store.go` 读写 key 不一致，`workingDir` 命名空间没有统一

位置：

- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:259)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:170)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:216)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:120)

现状：

- `loadFromDB()` 读取的是 `GetConfigByWorkingDir(conn, "global")`。
- `SetConfigField()` / `RemoveConfigField()` 写入的是 `configPath(scope)` 返回值。
- `configPath(scope)` 现在返回的仍是文件路径：
  - global 返回 `globalDataPath`
  - workspace 返回 `workspacePath`

问题：

- 读取 global 用的是 `"global"`，写入 global 用的是类似 `~/.local/share/.../hiagent.json` 的路径字符串。
- 读取 workspace 未来如果走 DB，也应该是 workspace key，但当前写入是 `workspacePath` 文件路径。
- 这会导致“写到一条记录，读另一条记录”，逻辑上已经分叉。

建议：

- 把 `configPath(scope)` 重命名或替换为“config storage key”语义，例如 `configKey(scope)`。
- 统一 key 规则，例如：
  - `global`
  - `workspace:<normalized-working-dir>`
- `HasConfigField / SetConfigField / RemoveConfigField / Load / ReloadFromDisk` 全部使用同一规则。

### 3. `ReloadFromDisk()` 仍完全走文件重载

位置：

- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:267)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:280)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:291)

现状：

- `ReloadFromDisk()` 仍然：
  - `lookupConfigs(s.workingDir)`
  - `loadFromConfigPaths(configPaths)`
  - `os.ReadFile(workspacePath)`

问题：

- 配置已经开始写入 `data_config`，但 reload 仍从文件系统取数据。
- 一旦用户通过 `SetConfigField()` 改了 DB 里的配置，reload 并不会可靠反映 DB 最新状态。
- `autoReload()` 的语义和实际数据源已经错位。

建议：

- 将 `ReloadFromDisk()` 改成“ReloadFromStore()”或保留名字但内部改成 DB reload。
- 若需要兼容文件导入，应该拆出单独的 import/merge 逻辑，而不是继续复用 file reload。

### 4. staleness 检测仍基于文件快照，不适配 DB 配置

位置：

- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:374)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:416)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:891)

现状：

- `CaptureStalenessSnapshot()` 仍把 tracked paths 规范化成文件绝对路径。
- `RefreshStalenessSnapshot()` 用 `os.Stat()` 建快照。
- `ConfigStaleness()` 用文件大小和 `ModTime` 判断 dirty。

问题：

- DB 模型下不存在可靠的“配置文件变更”这一事实源。
- 现在即便 DB 记录已经变化，staleness 也可能完全感知不到。
- 如果记录的是 `"global"` 这种 key，`filepath.Abs("global")` 还会生成误导性的伪路径。

建议：

- 将 staleness 模型切到 DB 记录级别：
  - key
  - `updated_at`
  - 可选 `config hash`
- `fileSnapshot` 建议升级为更中性的 `configSnapshot`。
- 如果仍要支持文件兼容模式，需要区分 `source=db|file`。

### 5. `scope.go` 的注释和错误语义仍是“配置文件”

位置：

- [scope.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/scope.go:3)

现状：

- `Scope` 注释写的是 “which config file is targeted”。
- `ErrNoWorkspaceConfig` 表示 “no workspace config path configured”。

问题：

- 对外语义仍然绑定文件路径，和新的 DB 配置模型不一致。

建议：

- 改成 storage scope / config scope 语义。
- `ErrNoWorkspaceConfig` 改成更接近 “workspace config key unavailable” 或 “workspace context unavailable”。

### 6. `ProjectNeedsInitialization()` / `MarkProjectInitialized()` 仍使用文件 flag

位置：

- [init.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/init.go:29)
- [init.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/init.go:103)

现状：

- 通过 `DataDirectory/init` 文件判断是否初始化。

问题：

- 这部分状态仍然保存在文件系统，不在 DB。
- 如果你们的目标是“config 目录下配置和数据存储都切到 DB 模型”，这个 flag 也属于需要统一的状态。

建议：

- 迁移到单独表，或并入 `data_config` 的某个 workspace 记录字段。
- 至少要明确：init flag 是否属于“仍保留文件”的例外。

## 二、强烈建议优化的点

### 7. `loadFromDB()` 只读 `global`，没有 workspace 合并能力

位置：

- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:256)

现状：

- 当前只读取 `GetConfigByWorkingDir(conn, "global")`。

问题：

- 无法表达 workspace 级配置覆盖。
- 也无法承接 `ScopeWorkspace` 相关写入。

建议：

- 定义一个 DB merge 流程，例如：
  1. 读取 `global`
  2. 读取 `workspace:<normalized-working-dir>`
  3. 按既有 merge 规则合并

### 8. `GetConfigByWorkingDir()` 对“记录不存在”和“配置为空字符串”区分不清

位置：

- [db_config.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/db_config.go:52)

现状：

- 找不到记录时返回 `("", nil)`。
- 记录存在但 `config=""` 时也返回 `("", nil)`。

问题：

- 调用方没法区分“没有记录”还是“有记录但为空配置”。
- `SetConfigField()` 里使用 `os.IsNotExist(err)` 判断也已经不成立。

建议：

- 增加显式语义：
  - `GetDataConfigByWorkingDir()` 返回 `(*DataConfig, nil)` / `(nil, nil)`
  - 调用方自行判断不存在
- 或者新增 `ErrDataConfigNotFound`。

### 9. `SetConfigField()` / `RemoveConfigField()` 里的错误处理仍带文件语义

位置：

- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:156)
- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:208)

现状：

- 读取失败时还在判断 `os.IsNotExist(err)`。
- 错误文本还是 `failed to read config file` / `failed to write data config` 混杂。

问题：

- 错误分层不清晰，调用方很难判断真正失败原因。

建议：

- 全部切成 DB 术语：
  - read config record
  - config row not found
  - persist config row

### 10. `loadedPaths` / `globalDataPath` / `workspacePath` 字段命名不再准确

位置：

- [store.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/store.go:47)

现状：

- `ConfigStore` 里很多字段仍然是 path 语义。

问题：

- 在 DB 模式下，这些字段会误导维护者。
- 部分字段现在既承担“文件路径”意义，又被借用成“DB 记录 key”来源。

建议：

- 梳理命名：
  - `globalConfigKey`
  - `workspaceConfigKey`
  - `loadedKeys`
  - 或保留 path 字段仅用于兼容导入，不参与主逻辑

### 11. `load_test.go` 测试目标已经落后于实现

位置：

- [load_test.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load_test.go:5)

现状：

- 只测 `lookupConfigs("./")`。

问题：

- 这个测试只覆盖旧的文件发现逻辑。
- 对 DB 模型下最关键的 global/workspace merge、scope 写入、reload、staleness 都没有保护。

建议：

- 增加以 DB 为中心的测试：
  - global load
  - workspace override
  - `SetConfigField()` 持久化
  - `ReloadFromDisk()`（或替代实现）读取 DB
  - staleness 基于 DB `updated_at`

## 三、需要确认边界的点

### 12. provider catalog 是否也要进入 DB

位置：

- [provider.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/provider.go:60)
- [custom_provider.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/custom_provider.go:49)

现状：

- `provider.go` 仍然有 provider cache 文件读写。
- `custom_provider.go` / `open_provider.go` 风格逻辑仍从 JSON 文件读 provider。
- 但 `db_provider.go` 已经开始从 DB 的 `providers` / `big_models` 表加载。

问题：

- provider 体系现在有三套来源：
  - DB provider catalog
  - provider cache 文件
  - custom/open provider JSON 文件
- 如果目标是“config 和数据存储都切 DB”，需要明确这几类是否也一起迁移。

建议：

- 如果 provider catalog 已经由 DB 承担主存储：
  - `provider.go` 应退化成导入工具，或只保留远程同步能力但落 DB。
- `custom_provider.go` 需要决定：
  - 保留为外部导入源
  - 还是也落到 `providers` / `big_models` 相关表

### 13. `GlobalConfig()` / `GlobalConfigData()` 是否还保留兼容职责

位置：

- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:288)
- [load.go](/C:/zorktech/projects/backen/hiagentgo/internal/config/load.go:297)

现状：

- 这些函数仍生成文件路径。

问题：

- 如果主逻辑完全 DB 化，它们最多只该用于：
  - 兼容导入
  - 一次性迁移
  - 非配置类磁盘文件定位

建议：

- 明确保留目的，避免继续被主配置读写链路引用。

## 四、推荐的落地顺序

1. 先统一 key 模型。
   先定义 `global` / `workspace:<workingDir>` 的 DB key 规则。

2. 再统一加载链路。
   让 `Load()` 和 `ReloadFromDisk()` 都只走 DB。

3. 接着替换写入链路。
   让 `configPath(scope)` 退出主流程，改为 DB key 访问。

4. 然后重做 staleness。
   把文件快照换成 DB record snapshot。

5. 最后处理外围兼容项。
   包括 init flag、provider cache、legacy file import、旧测试。

## 五、当前结论

`internal/config` 目前处于“半文件、半 DB”的过渡状态，最核心的问题不是单个函数实现，而是“配置主键模型”和“加载/重载的数据源”尚未统一。

在真正开始修改前，建议先做一个很小的设计确认：

- DB 中 global 和 workspace 配置分别用什么 key
- 是否保留文件兼容导入
- provider catalog / custom provider 是否也统一进 DB

只要这三个点定下来，后续改造路径会清楚很多。
