# Distributed Guide - 分布式部署指南

gomelo 支持前端/后端分离。前端 connector 负责客户端连接，后端服务通过 RPC 暴露业务能力，Master 负责服务协调和管理查询。

## 拓扑结构

```
clients
  |
  v
connector tcp/ws/udp  --forward/RPC-->  chat/game/auth backends
  |
  +-- register/query --> master
```

## 配置文件

`config/master.json`：

```json
{
  "development": {
    "id": "master-1",
    "host": "127.0.0.1",
    "port": 3005
  }
}
```

`config/servers.json`：

```json
{
  "development": [
    {
      "id": "connector-1",
      "serverType": "connector",
      "host": "127.0.0.1",
      "port": 3010,
      "frontend": true
    },
    {
      "id": "chat-1",
      "serverType": "chat",
      "host": "127.0.0.1",
      "port": 3020,
      "frontend": false
    }
  ]
}
```

`lib.LoadServersConfig` 要求每个环境下是服务器对象数组，不是按类型分组的 map。

## Master Server

```go
data, err := os.ReadFile("config/master.json")
if err != nil {
    log.Fatal(err)
}

m := master.New()
m.EnableAdmin(":3006")
if err := m.Start(data); err != nil {
    log.Fatal(err)
}
m.Wait()
```

Master query 返回结构为：

```json
{
  "servers": {
    "connector": [
      {"id":"connector-1","serverType":"connector","host":"127.0.0.1","port":3010,"frontend":true}
    ],
    "chat": [
      {"id":"chat-1","serverType":"chat","host":"127.0.0.1","port":3020,"frontend":false}
    ]
  },
  "count": 2
}
```

`master.Client.QueryServers()` 返回 `map[string][]*master.ServerInfo`，使用同样的 `serverType -> servers` 分组。

## 前端 Connector

TCP 和 WebSocket connector 可以作为 App 组件注册：

```go
app := gomelo.NewApp(
    gomelo.WithServerID("connector-1"),
    gomelo.WithHost("0.0.0.0"),
    gomelo.WithPort(3010),
)

conn := connector.NewServer(&connector.ServerOptions{
    Type:     "tcp",
    Host:     "0.0.0.0",
    Port:     3010,
    MaxConns: 10000,
})
conn.OnConnect(func(s *gomelo.Session) {
    log.Printf("connected: %d", s.ID())
})
app.Register("connector", conn)

app.On("connector.entryHandler.entry", entry)

if err := app.Start(); err != nil {
    log.Fatal(err)
}
app.Wait()
```

UDP connector 直接启动：

```go
udp := connector.NewUDPServer(&connector.UDPServerOptions{
    Type:     "udp",
    Host:     "0.0.0.0",
    Port:     3012,
    MaxConns: 5000,
})
udp.SetApp(app)
udp.Handle("connector.move", func(s *gomelo.Session, msg *gomelo.Message) (any, error) {
    return map[string]any{"code": 0}, nil
})
go udp.Start()
defer udp.Stop()
```

TCP、WebSocket、UDP 都支持 Pipeline 和 route handler。WebSocket 读路径兼容纯 JSON frame 和带 4 字节长度头的 JSON frame。

## RPC Server

Remote 方法签名为：

```go
func (r *ChatRemote) Send(ctx context.Context, args struct {
    RoomID string `json:"roomId"`
    Text   string `json:"text"`
}) (any, error) {
    return map[string]any{"code": 0}, nil
}
```

RPC 服务端会把 JSON args 转换为声明的参数类型，并把业务 error 返回给调用方。

```go
srv := rpc.NewServer("127.0.0.1:3020")
if err := srv.Register("chat", &ChatRemote{}); err != nil {
    log.Fatal(err)
}
if err := srv.Start(); err != nil {
    log.Fatal(err)
}
defer srv.Stop()
```

## RPC Client 和 Selector

```go
reg := server_registry.New()
_ = reg.Register(server_registry.ServerInfo{
    ID: "chat-1", ServerType: "chat", Host: "127.0.0.1", Port: 3020,
})

sel := selector.New(reg)
mgr, err := gomelo.NewRPCClientManager(reg, sel, nil)
if err != nil {
    log.Fatal(err)
}
app.SetRPCClientManager(mgr)

var reply map[string]any
err = app.RPCTo(context.Background(), "chat", "Send", map[string]any{
    "roomId": "lobby",
    "text":   "hello",
}, &reply)
```

默认 selector 是轮询，并会在多次调用之间保留轮询状态。可以使用 `sel.Register(serverType, handler)` 为指定 server type 注册自定义选择策略。

## Forwarding

前端 connector 可以按 route 第一段转发消息。例如 `chat.send` 会选择一个 `chat` 服务，并调用 RPC service `chat`、method `send`。

```go
fwd := forward.NewForwarder(app, sel)
_ = fwd.Start()
defer fwd.Stop()

tcp.SetForwarder(fwd)
tcp.SetForwardSelector(sel)
```

## 代码生成

```bash
go run ./cmd/codegen ./servers
go run ./cmd/codegen -list ./servers
```

生成的 `servers_gen.go` 必须纳入编译。它会为 Handler、Remote、Filter、Cron 文件注册 loader 回调；`loader.Load()` 扫描 `servers/` 目录时触发这些回调。

## 运维建议

- 保持 `serverType` 稳定，转发和 RPC 选择都依赖它。
- 为 connector 设置 `MaxConns`，避免无限接入。
- 创建 selector 前先把后端服务注册到 registry。
- 代码生成后、部署前运行 `go test ./...`。
