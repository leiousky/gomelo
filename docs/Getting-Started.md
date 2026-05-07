# Getting Started - 快速开始

本指南将帮助你在 5 分钟内启动一个 gomelo 游戏服务器。

## 环境要求

- Go 1.21 或更高版本

## 安装

### 方式一：go install（推荐）

```bash
go install github.com/chuhongliang/gomelo/cmd/gomelo@latest
```

### 方式二：手动编译

```bash
git clone https://github.com/chuhongliang/gomelo.git
cd gomelo
go build -o bin/gomelo ./cmd/gomelo
```

## 初始化项目

```bash
gomelo init mygame
cd mygame
go mod tidy
```

### 4. 启动服务器

```bash
go run .
```

服务器将在 `http://localhost:3010` 启动。

## 项目结构

```
game-project/
├── game-server/           # 游戏服务器
│   ├── main.go
│   ├── go.mod
│   ├── config/
│   │   ├── servers.json
│   │   ├── log.json
│   │   └── master.json
│   ├── servers/          # 服务器定义
│   │   ├── connector/
│   │   ├── gate/
│   │   ├── chat/
│   │   └── game/
│   ├── components/      # 共享组件
│   ├── cmd/admin/        # 监控管理后台
│   └── logs/            # 日志目录
├── web-server/           # 前端静态资源
│   └── public/
│       ├── index.html
│       └── js/client.js
```

## 最小示例

### main.go

```go
package main

import (
	"log"

	"github.com/chuhongliang/gomelo"
	"github.com/chuhongliang/gomelo/connector"
)

func main() {
	app := gomelo.NewApp(
		gomelo.WithHost("0.0.0.0"),
		gomelo.WithPort(3010),
		gomelo.WithServerID("connector-1"),
	)

	conn := connector.NewServer(&connector.ServerOptions{
		Type: "tcp",
		Host: "0.0.0.0",
		Port: 3010,
	})
	app.Register("connector", conn)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
	log.Println("Server started on :3010")

	app.Wait()
}
```

### Handler 示例

```go
// servers/connector/handler/entry.go
package handler

type EntryHandler struct{}

func (h *EntryHandler) Entry(ctx *gomelo.Context) {
	var req struct {
		Name string `json:"name"`
	}
	ctx.Bind(&req)

	ctx.Session().Set("name", req.Name)

	ctx.Response(map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{
			"welcome": "Hello " + req.Name,
		},
	})
}
```

自动生成的路由：`connector.entryHandler.entry`

### config.json

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 3010,
    "env": "development"
  },
  "rpc": {
    "host": "0.0.0.0",
    "port": 3030,
    "maxConns": 10
  },
  "log": {
    "level": "debug"
  }
}
```

## 测试

这是 TCP/WebSocket/UDP 服务器，不是 HTTP handler。请使用 gomelo 客户端或 demo/robot 工具测试：

```bash
go run ./client/go/demo
```

预期响应：

```json
{"code":0,"msg":"ok","data":{"welcome":"Hello player1"}}
```

## 下一步

- [创建 Handler](Handler-Guide.md) - 学习如何处理客户端请求
- [使用 Session](Session-Guide.md) - 管理玩家会话
- [分布式部署](Distributed-Guide.md) - 部署多节点游戏服务器

## 常见问题

### Q: 端口被占用？

修改 `config.json` 中的端口：

```json
{
  "server": {
    "port": 3011
  }
}
```

### Q: 如何开启调试日志？

设置环境变量或修改配置：

```json
{
  "log": {
    "level": "debug"
  }
}
```
