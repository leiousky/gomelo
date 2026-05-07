# Getting Started

This guide will help you get a gomelo game server running in 5 minutes.

## Requirements

- Go 1.21 or higher

## Installation

### Method 1: go install (recommended)

```bash
go install github.com/chuhongliang/gomelo/cmd/gomelo@latest
```

### Method 2: Manual build

```bash
git clone https://github.com/chuhongliang/gomelo.git
cd gomelo
go build -o bin/gomelo ./cmd/gomelo
```

## Initialize Project

```bash
gomelo init mygame
cd mygame
go mod tidy
```

### 4. Start Server

```bash
go run .
```

Server will start at `http://localhost:3010`.

## Project Structure

```
game-project/
├── game-server/           # Game server
│   ├── main.go
│   ├── go.mod
│   ├── config/
│   │   ├── servers.json
│   │   ├── log.json
│   │   └── master.json
│   ├── servers/          # Server definitions
│   │   ├── connector/
│   │   ├── gate/
│   │   ├── chat/
│   │   └── game/
│   ├── components/      # Shared components
│   ├── cmd/admin/        # Admin monitor
│   └── logs/            # Log directory
├── web-server/           # Frontend static files
│   └── public/
│       ├── index.html
│       └── js/client.js
```
mygame/
├── main.go              # Entry file
├── go.mod               # Go module
├── config.json          # Config file
├── servers.json         # Multi-server config
├── config/
│   ├── prod.json        # Production env
│   └── dev.json         # Development env
└── app/
    └── handlers/        # Business handlers
```

## Minimal Example

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

### Handler Example

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

Auto-generated route: `connector.entryHandler.entry`

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

## Testing

This is a TCP/WebSocket/UDP server, not an HTTP handler. Test it with one of the gomelo clients or the demo/robot tools:

```bash
go run ./client/go/demo
```

Expected response:

```json
{"code":0,"msg":"ok","data":{"welcome":"Hello player1"}}
```

## Next Steps

- [Handler Guide](Handler-Guide.md) - Learn to handle client requests
- [Session Guide](Session-Guide.md) - Manage player sessions
- [Distributed Guide](Distributed-Guide.md) - Deploy multi-node game servers

## FAQ

### Q: Port already in use?

Modify port in `config.json`:

```json
{
  "server": {
    "port": 3011
  }
}
```

### Q: How to enable debug logs?

Set environment or modify config:

```json
{
  "log": {
    "level": "debug"
  }
}
```
