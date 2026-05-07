# Handler Guide

Handlers are the core components for processing client requests.

## Basic Concept

A Handler is a method that takes `*Context`, auto-registered by naming convention:

```go
func (h *EntryHandler) Entry(ctx *gomelo.Context)
```

## Auto Registration

Use codegen to automatically scan and register Handlers:

```bash
go run ./cmd/codegen ./servers
```

Generated route format: `serverType.handlerName.methodName`

| Handler | Method | Generated Route |
|---------|--------|----------------|
| `EntryHandler` | `Entry` | `connector.entryHandler.entry` |
| `ChatHandler` | `Send` | `chat.chatHandler.send` |
| `GameHandler` | `Battle` | `game.gameHandler.battle` |

## Context Common Methods

### Get Request Data - Bind

```go
func handleEntry(ctx *gomelo.Context) {
    var req struct {
        Token string `json:"token"`
        Name  string `json:"name"`
    }
    if err := ctx.Bind(&req); err != nil {
        ctx.ResponseError(1001, "invalid request")
        return
    }

    log.Printf("Received: token=%s, name=%s", req.Token, req.Name)
}
```

### Send Response - Response

```go
ctx.Response(map[string]any{
    "code": 0,
    "msg":  "ok",
    "data": map[string]any{
        "uid": ctx.Session().UID(),
    },
})
```

### Get Session

```go
session := ctx.Session()
session.Set("level", 10)
uid := session.UID()
```

### Get Message

```go
msg := ctx.Request()
log.Printf("Route: %s, Body: %v", msg.Route, msg.Body)
```

## Structured Handler

Organize multiple handlers into a struct:

```go
type ConnectorHandler struct {
    app *gomelo.App
}

func (h *ConnectorHandler) Entry(ctx *gomelo.Context) {
    var req struct {
        Token string `json:"token"`
    }
    ctx.Bind(&req)

    uid := fmt.Sprintf("user-%d", ctx.Session().ID())
    ctx.Session().Bind(uid)

    ctx.Response(map[string]any{
        "code": 0,
        "msg":  "ok",
        "data": map[string]any{"uid": uid},
    })
}

func (h *ConnectorHandler) Heartbeat(ctx *gomelo.Context) {
    ctx.Response(map[string]any{"code": 0, "msg": "pong"})
}
```

Register:

```go
handler := &ConnectorHandler{app: app}
app.On("connector.entry", handler.Entry)
app.On("connector.heartbeat", handler.Heartbeat)
```

## Error Handling

### Use Predefined Errors

```go
ctx.ResponseError(401, "unauthorized")
ctx.ResponseError(1001, "invalid route")
ctx.ResponseError(1004, "timeout")
```

### Custom Error

```go
ctx.ResponseError(500, fmt.Sprintf("custom error: %v", someErr))
```

## Pre-processing - Middleware

Use `app.Before()` to add pre-processing filters:

```go
type AuthFilter struct{}

func (AuthFilter) Name() string { return "auth" }
func (AuthFilter) Process(ctx *gomelo.Context) bool {
    token := ctx.Session().Get("token")
    if token == nil {
        ctx.Response(map[string]any{"code": 401, "msg": "unauthorized"})
        return false
    }
    return true
}
func (AuthFilter) After(ctx *gomelo.Context) {}

app.Before(AuthFilter{})
```

## Post-processing - AfterFilter

```go
type LogFilter struct{}

func (LogFilter) Name() string { return "log" }
func (LogFilter) Process(ctx *gomelo.Context) bool { return true }
func (LogFilter) After(ctx *gomelo.Context) {
    log.Printf("Request completed: %s", ctx.Route)
}

app.After(LogFilter{})
```

## Complete Example

```go
package main

import (
	"log"
	"strconv"
	"github.com/chuhongliang/gomelo"
	"github.com/chuhongliang/gomelo/connector"
)

func main() {
	app := gomelo.NewApp(
		gomelo.WithPort(3010),
		gomelo.WithServerID("connector-1"),
	)

	conn := connector.NewServer(&connector.ServerOptions{
		Type: "tcp",
		Host: "0.0.0.0",
		Port: 3010,
	})
	conn.OnConnect(func(session *gomelo.Session) {
		log.Printf("Client connected: %d", session.ID())
	})
	app.Register("connector", conn)

	app.Before(AuthFilter{})

	app.On("connector.entry", handleEntry)
	app.On("connector.heartbeat", handleHeartbeat)
	app.On("chat.send", handleChatSend)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
	app.Wait()
}

type AuthFilter struct{}

func (AuthFilter) Name() string { return "auth" }
func (AuthFilter) Process(ctx *gomelo.Context) bool {
	token := ctx.Session().Get("token")
	if token == nil {
		ctx.Response(map[string]any{"code": 401, "msg": "no token"})
		return false
	}
	return true
}
func (AuthFilter) After(ctx *gomelo.Context) {}

func handleEntry(ctx *gomelo.Context) {
	var req struct {
		Name string `json:"name"`
	}
	ctx.Bind(&req)

	uid := "user-" + strconv.FormatUint(ctx.Session().ID(), 10)
	ctx.Session().Bind(uid)
	ctx.Session().Set("name", req.Name)

	ctx.Response(map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{
			"uid":  uid,
			"name": req.Name,
		},
	})
}

func handleHeartbeat(ctx *gomelo.Context) {
	ctx.Response(map[string]any{"code": 0, "msg": "pong"})
}

func handleChatSend(ctx *gomelo.Context) {
	var req struct {
		Msg string `json:"msg"`
	}
	ctx.Bind(&req)

	uid := ctx.Session().Get("uid")
	log.Printf("%s: %s", uid, req.Msg)

	ctx.Response(map[string]any{"code": 0, "msg": "ok"})
}
```

## Route Convention

Auto-registered route format: `serverType.handlerName.methodName`

| Full Route | Description |
|------------|-------------|
| `connector.entryHandler.entry` | Connector entry |
| `connector.entryHandler.login` | Login |
| `chat.chatHandler.send` | Chat send |
| `chat.chatHandler.join` | Join room |
| `game.gameHandler.start` | Game start |
| `game.gameHandler.move` | Game move |

Use `app.Route()` to customize route aliases.

## Auto-Registration (Recommended)

Use codegen to automatically scan and register Handlers - no manual `app.On()` calls needed.

### Directory Structure

```
servers/
  {serverType}/
    handler/      # Handler directory
    remote/       # RPC directory
    filter/       # Filter directory
    cron/         # Cron directory
```

### Handler Naming Convention

- Type name must end with `Handler` or `handler`
- Methods take `*lib.Context` parameter

```go
// servers/connector/handler/entry.go
package handler

import "github.com/chuhongliang/gomelo/lib"

type EntryHandler struct {
    app *lib.App
}

func (h *EntryHandler) Init(app *lib.App) { h.app = app }

func (h *EntryHandler) Entry(ctx *lib.Context) {
    var req struct {
        Name string `json:"name"`
    }
    ctx.Bind(&req)
    ctx.Response(map[string]any{"msg": "hello " + req.Name})
}
```

### Run Code Generation

```bash
go run ./cmd/codegen ./servers

# list routes only
go run ./cmd/codegen -list ./servers
```

Generates `servers_gen.go` with Handler, Remote, Filter, and Cron registration callbacks. Include this file in the build. `loader.Load()` scans the `servers/` tree and invokes the generated callbacks by file key.

### Output All Routes (for clients)

```go
// Get all registered Handler routes
l := loader.GlobalLoader()
handlerRoutes := l.GetAllHandlerRoutes()
fmt.Println("Handler Routes:", handlerRoutes)

// Get all registered Remote routes
remoteRoutes := l.GetAllRemoteRoutes()
fmt.Println("Remote Routes:", remoteRoutes)
```

Output example:
```
Handler Routes: [connector.entryHandler.entry connector.entryHandler.login chat.chatHandler.send]
Remote Routes: [game.gameHandler.addGame game.gameHandler.start]
```

### Generated File Example

```go
// servers_gen.go (auto-generated)
func init() {
    loader.RegisterHandler("connector/handler/entry", func(l *loader.Loader, serverType string) {
        hEntryHandler := &handler.EntryHandler{}
        vEntryHandler := loader.ReflectValueOf(hEntryHandler)
        tEntryHandler := vEntryHandler.Type()
        for i := 0; i < tEntryHandler.NumMethod(); i++ {
            m := tEntryHandler.Method(i)
            if loader.IsHandlerMethod(m) {
                route := loader.BuildRoute(serverType, tEntryHandler.Elem().Name(), m.Name)
                l.RegisterHandlerMethod(serverType, route, hEntryHandler, m)
            }
        }
    })
}
```

### Route Rules

| Handler Method | Auto-generated Route |
|----------------|----------------------|
| `EntryHandler.Entry` | `connector.entryHandler.entry` |
| `ChatHandler.Send` | `connector.chatHandler.send` |

Manual route aliases can be added with `app.On(route, handler)`.

### Init Callback

Generated registration constructs handler values directly. If a handler needs `*lib.App`, wire it explicitly in your own registration path or extend the generated callback:

```go
func (h *EntryHandler) Init(app *lib.App) {
    h.app = app
}
```
