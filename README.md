# MaiBot Bootstrap（单实例 CLI + TUI）

本项目提供 MaiBot 的一键安装入口与跨平台命令行管理能力。
采用 Git 风格 workspace：在项目目录执行 `maibot init` 后，目录约定如下：
- `.maibot/`：仅存放 maibot 命令行工具的配置/状态/日志数据
- `modules/`：存放附属模块（如 napcat、适配器）
- `MaiBot/`：存放本体文件

## 快速安装

从发布页获取 install.sh 并执行：

```bash
curl -fsSL https://raw.githubusercontent.com/Mai-with-u/maibot-bootstrap/main/scripts/install.sh | bash
```

安装脚本会预检 `git` 与 `uv`；若缺失会先征求确认，确认后尝试自动安装。

安装后可用：

```bash
maibot version
```

## 常用命令

工作区管理（Git 风格）：

```bash
maibot init
maibot start
maibot status
maibot logs --tail 100
maibot update
maibot stop
maibot workspace ls .
maibot -C ../other-workspace status
maibot modules list
maibot modules install napcat
```

内置 `napcat` 模块会执行系统依赖安装、LinuxQQ 安装、launcher 编译等步骤。
这些步骤可能触发 sudo 认证，建议在 TTY 环境执行（例如直接在终端或 TUI 中运行）。

服务管理：

```bash
maibot service install
maibot service start
maibot service status
maibot service stop
maibot service uninstall
```

说明：service 按 workspace 路径生成唯一服务名；可用 `maibot -C <path> service ...` 管理其他 workspace 的服务。

自更新与清理：

```bash
maibot upgrade
maibot cleanup --test-artifacts
maibot run echo devtool
```

## 配置

全局配置文件默认位于 `~/.maibot/maibot.conf`，为 JSON 格式。
workspace 运行数据位于工作区目录下的 `.maibot/`（通过 `maibot init` 创建）。
附属模块安装到工作区根目录的 `modules/`，本体目录使用 `MaiBot/`。
支持环境变量覆盖（`MAIBOT_` 前缀）。

`modules` 支持两种来源：
- 内置模块列表（写死在代码中）
- 远程 `catalog_urls`（HTTP JSON）

git 与 modules 共用顶层 `mirrors` 镜像池配置。

远程 catalog JSON 可为以下两种格式之一：

```json
{"modules":[{"name":"napcat","description":"...","install":[{"name":"step","command":"bash","args":["-lc","..."]}]}]}
```

或

```json
[{"name":"napcat","description":"...","install":[{"name":"step","command":"bash","args":["-lc","..."]}]}]
```

核心字段示例：

```json
{
  "installer": {
    "repo": "Mai-with-u/maibot-bootstrap",
    "release_channel": "stable",
    "language": "auto",
    "data_home": "~/.maibot",
    "instance_tick_interval": "15s",
    "lock_timeout_seconds": 8
  },
  "logging": {
    "file_path": "~/.maibot/logs/installer.log",
    "max_size_mb": 10,
    "retention_days": 7,
    "max_backup_files": 20
  },
  "updater": {
    "require_signature": false,
    "minisign_public_key": ""
  },
  "mirrors": {
    "urls": [
      "https://ghfast.top",
      "https://gh.wuliya.xin",
      "https://gh-proxy.com",
      "https://github.moeyy.xyz"
    ],
    "probe_url": "https://raw.githubusercontent.com/Mai-with-u/plugin-repo/refs/heads/main/plugins.json",
    "probe_seconds": 8
  },
  "git": {
    "mirrors": [
      {
        "name": "fastgit",
        "base_url": "https://hub.fastgit.org",
        "enabled": false
      }
    ],
    "mirror_first": true,
    "retry_per_source": 2,
    "retry_backoff_seconds": 1,
    "command_timeout_seconds": 120
  },
  "modules": {
    "catalog_urls": [],
    "catalog_timeout_seconds": 5,
    "install_retries": 2,
    "install_backoff_seconds": 1,
    "prefer_catalog_source": false
  }
}
```

## TUI

直接运行 `maibot` 会进入交互式 TUI，支持中英文显示与功能面板导航。终端非 TTY 时会自动降级为帮助输出。

## 构建与测试

```bash
go test ./...
go build ./cmd/maibot
```

## 说明

本项目遵循 Go 推荐目录结构，核心逻辑位于 `internal/`，入口在 `cmd/maibot`。
每个 workspace 的运行数据默认存放在该 workspace 根目录的 `.maibot/`。

`cleanup --test-artifacts` 默认仅清理当前 workspace 的 `.maibot/` 与全局锁文件。
若要额外清理当前仓库下的 `./maibot`、`./dist`，请显式设置环境变量：`MAIBOT_ALLOW_DEV_CLEANUP=1`。
