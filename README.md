# Codex Token Float

[![Native build](https://github.com/javatoai/codex-token-float/actions/workflows/build.yml/badge.svg?branch=master)](https://github.com/javatoai/codex-token-float/actions/workflows/build.yml?query=branch%3Amaster)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

一个用 Go 和 MyGO 编写的本地 Codex Token 悬浮计数器，支持 Windows x64 和 macOS 13 Ventura 及以上版本（Apple Silicon 和 Intel）。

- 按北京时间显示当天的原始 Token 总量，从右向左每四位用逗号分组，例如 `21,2002,8236`。
- 透明、无边框、始终置顶。拖动可移动，重启恢复上次位置。悬停可查看精确总数、日期、日志数和更新时间。
- 托盘菜单可显示、隐藏、立即刷新和退出。
- 每两秒检查日志变化，文件追加时从上次读到的位置继续。跨日自动刷新。

## 下载

打开 [`master` 分支的构建记录](https://github.com/javatoai/codex-token-float/actions/workflows/build.yml?query=branch%3Amaster)，进入最近一次成功运行。在 Artifacts 下载对应平台的包并解压。每份产物包含便携 ZIP 和 `SHA256SUMS.txt`。

- Windows x64 解压便携 ZIP 后运行 `codex-token-float.exe`。
- macOS 解压便携 ZIP 后将 `Codex Token Float.app` 放入“应用程序”。构建使用临时签名，首次打开可能需要通过系统设置允许启动。

## 统计口径

应用在本机读取 `~/.codex/sessions` 下的 JSONL。设置 `CODEX_HOME` 后，读取该目录下的 `sessions`。应用只提取 Token 事件的时间和用量字段，统计过程完全离线。

每个会话按相邻 `total_token_usage.total_tokens` 的差值计数，累计值相同的快照贡献为零。跨午夜的会话使用前一条累计值作基准，累计值回落时按计数器重启计算。旧日志缺少累计字段时使用 `last_token_usage.total_tokens`。

总量单位为原始 Token，涵盖输入和输出。缓存输入和推理 Token 已计入各自的输入或输出总量。

## 开发与构建

开发环境使用 Go 1.27.1 和 MyGO v0.5.5。打包脚本另外需要 Python 3.11 或更新版本。

```sh
go test ./...
go vet ./...
go run ./cmd/codex-token-float
```

直接输出当天统计的 JSON。

```sh
go run ./cmd/codex-token-float --print-today
```

生成便携 ZIP。macOS 构建可用 `darwin/arm64` 或 `darwin/amd64` 作为目标。

```sh
python scripts/package.py --target windows/amd64
```

输出在 `dist/` 的对应平台子目录中，ZIP 内附应用和依赖许可证。

## 许可证

[MIT](LICENSE)
