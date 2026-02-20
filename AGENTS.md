# AGENTS.md

本文档给在本仓库工作的 agent 使用，目标是：
- 快速定位代码与关键入口
- 使用仓库真实命令完成构建/测试/校验
- 保持与现有 Go 代码风格一致

## 1. 仓库与技术栈
- 语言：Go（`go.mod`：`go 1.23.4`）
- 产品定位：单实例 CLI + TUI 的 MaiBot 工作区面板与开发工具
- 程序入口：`cmd/maibot/main.go`
- 核心逻辑：`internal/`
- CI：`.github/workflows/ci.yml`
- 发布脚本：`scripts/release-local.sh`
- 安装脚本：`scripts/install.sh`

## 2. 高优先阅读文件
- `README.md`（用户视角命令与构建说明）
- `.github/workflows/ci.yml`（权威质量门槛）
- `internal/app/app.go`（CLI 命令树）
- `internal/config/config.go`（配置结构与默认值）
- `internal/logging/logger.go`（日志封装）

## 3. 构建/测试/Lint 命令（仓库实锤）
以下命令来自 `README.md` 与 CI，优先使用：

```bash
# 全量测试
go test ./...

# 构建主程序
go build ./cmd/maibot

# 格式检查（CI 同款）
test -z "$(gofmt -l .)"

# 静态检查（CI 使用 staticcheck action，本地建议）
staticcheck ./...
```

说明：
- 仓库内未发现 `golangci-lint` 配置文件。
- 未发现 Makefile/Taskfile/npm scripts 包装层，直接使用 `go` 命令。

## 4. 单元测试运行方式（重点）
Go 测试按“包”执行，单文件实践通常是“包路径 + `-run` 精确函数名”。

```bash
# 跑单个包
go test ./internal/app

# 跑单个测试函数
go test ./internal/app -run '^TestValidateConfig$' -v -count=1

# 跑一组测试（正则）
go test ./internal/config -run 'TestLoadOrCreate' -v

# 跑子测试
go test ./path/to/pkg -run '^TestParent/Subcase$' -v
```

## 5. 建议本地校验顺序
每次修改后建议按以下顺序执行：
1. `gofmt -w <changed-files>`
2. `test -z "$(gofmt -l .)"`
3. `go test ./...`
4. `go build ./cmd/maibot`
5. `staticcheck ./...`（若本机已安装）

## 6. 代码风格约定（基于现有代码）

### 6.1 导入与包组织
- 导入分组：标准库 / 第三方 / 本地模块。
- 本地模块路径使用 `maibot/internal/...`。
- 包名小写且语义清晰，避免含糊缩写。
- 平台差异代码使用 build tags（如 `//go:build windows`）。

### 6.2 命名
- 导出标识符：PascalCase（如 `LoadOrCreate`）。
- 非导出标识符：camelCase（如 `resolveInstanceName`）。
- 状态常量集中定义（参考 `internal/instance/state.go`）。

### 6.3 类型与序列化
- 配置/持久化结构体必须写 JSON tag。
- 优先使用强类型结构体而非无约束 map。
- 非必要不引入指针，优先值语义。

### 6.4 错误处理
- 使用就近处理：`if err := ...; err != nil { ... }`。
- 透传底层错误时使用 `%w` 包装。
- 哨兵错误判断使用 `errors.Is`。
- 禁止吞错；错误必须返回或记录。
- 错误信息应包含上下文（动作/对象/原因）。

### 6.5 日志与输出
- 业务日志统一使用 `internal/logging`。
- 使用 `Module("...")` 划分日志域（app/instance/update/cleanup）。
- 入口初始化失败可直接写 `stderr` 并退出。

### 6.6 进程与上下文
- 外部命令执行优先接受 `context.Context`。
- 显式绑定 `stdin/stdout/stderr`。
- 平台相关逻辑分文件维护：`*_unix.go` / `*_windows.go`。

### 6.7 测试
- 测试文件命名：`*_test.go`。
- 测试函数命名：`TestXxx`。
- 使用标准库 `testing`，失败信息应带 got/want 关键上下文。

## 7. 变更实施清单（给 agent）
- 先读同目录既有实现，再改代码。
- 改动保持小而聚焦，避免无关重构。
- 新增配置项时，同步更新：`defaults`、`applyDefaults`、落盘逻辑、测试。
- 新增 CLI 子命令时，在 `newRootCommand()` 注册并补充日志。
- 修改发布链路时，同步检查 manifest 生成与解析逻辑。

## 8. 明确禁止
- 不要引入与仓库无关的主构建体系（npm/pnpm 等）。
- 不要在未验证前修改 CI 关键命令。
- 不要跳过 gofmt、测试和构建直接交付。
- 不要把平台特定逻辑塞进通用文件。

## 9. Cursor / Copilot 规则检查结果
已检查以下位置：
- `.cursor/rules/`
- `.cursorrules`
- `.github/copilot-instructions.md`

当前仓库未发现上述规则文件。
若后续新增，请将关键约束合并到本文件并标注来源路径。

## 10. 维护建议
- CI 命令变化后，优先更新第 3 节与第 4 节。
- 新增工具时，补齐“安装方式 + 全量命令 + 单项运行方式”。
- 规则尽量写成“可执行命令 + 对应路径证据”。

## 11. 常见模块速查
- `internal/app/`：CLI 子命令与主编排流程。
- `internal/config/`：配置加载、迁移、默认值、环境变量覆盖。
- `internal/instance/`：实例状态机与锁文件管理。
- `internal/process/`：跨平台进程存活检测与停止逻辑。
- `internal/execx/`：外部命令执行、TTY 交互与 sudo 处理。
- `internal/release/`：manifest 解析与资产定位。
- `internal/version/`：安装器版本常量。
- `internal/logging/`：控制台 + 文件日志封装。
- `scripts/`：发布与安装脚本实现。
