# Retro BBS

Go 实现的复古风格论坛，包含 WebSocket 服务端、Web 客户端、Bubble Tea 终端客户端和可选的 MCP agent 接口。

## 运行

```powershell
go run ./cmd/retro-bbs
go run ./cmd/tui
```

`go run .` 仍可用于启动服务端。配置使用根目录的 `config.toml`；数据默认保存在 `data/`。可通过 `BBS_ROOT` 指定运行根目录。

## 目录

```text
cmd/retro-bbs/  服务端启动入口
cmd/tui/        终端客户端
internal/bbs/   服务端配置、协议、存储与业务逻辑
web/            Web 客户端静态资源
docs/           设计及运维文档
```

## 管理命令

```powershell
go run ./cmd/retro-bbs backup
go run ./cmd/retro-bbs restore <zip>
```

更多说明见 [docs/backup.md](docs/backup.md) 和 [docs/mcp.md](docs/mcp.md)。
