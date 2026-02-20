# MaiBot Bootstrap（单实例 CLI + TUI）

本项目提供 MaiBot 的一键安装入口与跨平台命令行管理能力，专注单实例工作区（默认 `main`）。
支持 curl + bash 首装，之后通过 `maibot` 命令完成初始化、启动停止、更新、服务管理与开发工具操作。

## 快速安装

从发布页获取 install.sh 并执行：

```bash
curl -fsSL https://raw.githubusercontent.com/Mai-with-u/maibot-bootstrap/main/scripts/install.sh | bash
```

安装后可用：

```bash
maibot version
```

## 常用命令

工作区管理：

```bash
maibot install
maibot start
maibot status
maibot logs --tail 100
maibot update
maibot stop
```

服务管理：

```bash
maibot service install
maibot service start
maibot service status
maibot service stop
maibot service uninstall
```

自更新与清理：

```bash
maibot self-update
maibot cleanup --test-artifacts
maibot run echo devtool
```

## 配置

配置文件位于 `~/.maibot/maibot.conf`，为 JSON 格式。首次运行自动生成，支持环境变量覆盖（`MAIBOT_` 前缀）。

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

本项目遵循 Go 推荐目录结构，核心逻辑位于 `internal/`，入口在 `cmd/maibot`。工作区配置与运行数据默认存放于 `~/.maibot`。

`cleanup --test-artifacts` 默认仅清理 `~/.maibot/workspace` 与锁文件。
若要额外清理当前仓库下的 `./maibot`、`./dist`，请显式设置环境变量：`MAIBOT_ALLOW_DEV_CLEANUP=1`。
