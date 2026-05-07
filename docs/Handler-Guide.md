# Handler Guide - 处理器指南

Handlers 是处理客户端请求的核心组件。

## 基本概念

Handler 是一个接收 `*Context` 参数的方法，遵循命名规范自动注册：

```go
func (h *EntryHandler) Entry(ctx *gomelo.Context)
```

## 自动注册

使用 codegen 自动扫描和注册 Handler，无需手动调用 `app.On()`：

```bash
go run ./cmd/codegen ./servers
```

生成的路由格式：`服务器类型.处理器名.方法名`

| 处理器 | 方法 | 生成路由 |
|--------|------|----------|
| `EntryHandler` | `Entry` | `connector.entryHandler.entry` |
| `ChatHandler` | `Send` | `chat.chatHandler.send` |
| `GameHandler` | `Battle` | `game.gameHandler.battle` |

## Context 常用方法

### 获取请求数据 - Bind

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

### 发送响应 - Response

```go
ctx.Response(map[string]any{
    "code": 0,
    "msg":  "ok",
    "data": map[string]any{
        "uid": ctx.Session().UID(),
    },
})
```

### 获取 Session

```go
session := ctx.Session()
session.Set("level", 10)
uid := session.UID()
```

### 获取 Message

```go
msg := ctx.Request()
log.Printf("Route: %s, Body: %v", msg.Route, msg.Body)
```

## 结构化 Handler

可以将多个 handler 组织到一个结构体中：

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

注册：

```go
handler := &ConnectorHandler{app: app}
app.On("connector.entry", handler.Entry)
app.On("connector.heartbeat", handler.Heartbeat)
```

## 错误处理

### 使用预定义错误

```go
ctx.ResponseError(401, "unauthorized")
ctx.ResponseError(1001, "invalid route")
ctx.ResponseError(1004, "timeout")
```

### 自定义错误

```go
ctx.ResponseError(500, fmt.Sprintf("custom error: %v", someErr))
```

## 前置处理 - Middleware

使用 `app.Before()` 添加前置 Filter：

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

## 后置处理 - AfterFilter

```go
type LogFilter struct{}

func (LogFilter) Name() string { return "log" }
func (LogFilter) Process(ctx *gomelo.Context) bool { return true }
func (LogFilter) After(ctx *gomelo.Context) {
    log.Printf("Request completed: %s", ctx.Route)
}

app.After(LogFilter{})
```

## 完整示例

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

## 路由约定

自动注册生成的完整路由格式：`serverType.handlerName.methodName`

| 完整路由 | 说明 |
|----------|------|
| `connector.entryHandler.entry` | 连接器入口 |
| `connector.entryHandler.entry` | 连接器入口 |
| `connector.entryHandler.login` | 登录 |
| `chat.chatHandler.send` | 聊天发送 |
| `chat.chatHandler.join` | 加入房间 |
| `game.gameHandler.start` | 游戏开始 |
| `game.gameHandler.move` | 游戏移动 |

可配合 `app.Route()` 自定义路由别名。

## 自动注册（推荐）

使用 codegen 自动扫描和注册 Handler，无需手动调用 `app.On()`。

### 目录结构

```
servers/
  {serverType}/
    handler/      # 处理器目录
    remote/       # RPC 目录
    filter/       # 过滤器目录
    cron/         # 定时任务目录
```

### Handler 命名规范

- 类型名以 `Handler` 或 `handler` 结尾
- 方法接收 `*lib.Context` 参数

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

### 运行代码生成

```bash
go run ./cmd/codegen ./servers

# 仅列出路由
go run ./cmd/codegen -list ./servers
```

生成 `servers_gen.go`，包含 Handler、Remote、Filter、Cron 的注册回调。生成文件需要纳入编译；`loader.Load()` 扫描 `servers/` 目录时会按文件 key 触发这些回调。

### 输出所有路由（供客户端使用）

```go
// 获取所有已注册的 Handler 路由
l := loader.GlobalLoader()
handlerRoutes := l.GetAllHandlerRoutes()
fmt.Println("Handler Routes:", handlerRoutes)

// 获取所有已注册的 Remote 路由
remoteRoutes := l.GetAllRemoteRoutes()
fmt.Println("Remote Routes:", remoteRoutes)
```

输出示例：
```
Handler Routes: [connector.entryHandler.entry connector.entryHandler.login chat.chatHandler.send]
Remote Routes: [game.gameHandler.addGame game.gameHandler.start]
```

### 生成的文件示例

```go
// servers_gen.go (自动生成)
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

### 路由规则

| Handler 方法 | 自动生成路由 |
|-------------|-------------|
| `EntryHandler.Entry` | `connector.entryHandler.entry` |
| `ChatHandler.Send` | `connector.chatHandler.send` |

可以用 `app.On(route, handler)` 额外注册手写路由别名。

### 初始化回调

生成代码会直接构造 Handler 实例。若 Handler 需要 `*lib.App`，请在手写注册路径中注入，或按项目需要扩展生成回调：

```go
func (h *EntryHandler) Init(app *lib.App) {
    h.app = app
}
```

| 路由 | 说明 |
|------|------|
| `connector.entry` | 连接器入口 |
| `connector.heartbeat` | 心跳检测 |
| `chat.send` | 聊天发送 |
| `chat.join` | 加入房间 |
| `game.start` | 游戏开始 |
| `game.move` | 游戏移动 |
