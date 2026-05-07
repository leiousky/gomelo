# Distributed Guide

gomelo supports frontend/backend separation. Frontend connector servers accept client traffic, and backend servers expose RPC services. Master can coordinate server discovery and provides an admin API.

## Topology

```
clients
  |
  v
connector tcp/ws/udp  --forward/RPC-->  chat/game/auth backends
  |
  +-- register/query --> master
```

## Configuration Files

`config/master.json`:

```json
{
  "development": {
    "id": "master-1",
    "host": "127.0.0.1",
    "port": 3005
  }
}
```

`config/servers.json`:

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

`lib.LoadServersConfig` expects each environment to be an array of server objects, not a map grouped by type.

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

The Master query response shape is:

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

`master.Client.QueryServers()` returns `map[string][]*master.ServerInfo` using the same `serverType -> servers` grouping.

## Frontend Connector

TCP and WebSocket connectors are App components:

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

UDP is started directly:

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

TCP, WebSocket, and UDP all support Pipeline and route handlers. WebSocket accepts both raw JSON frames and length-prefixed JSON frames.

## RPC Server

Remote methods use this contract:

```go
func (r *ChatRemote) Send(ctx context.Context, args struct {
    RoomID string `json:"roomId"`
    Text   string `json:"text"`
}) (any, error) {
    return map[string]any{"code": 0}, nil
}
```

The RPC server converts JSON args into the declared argument type and returns business errors to the caller.

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

## RPC Client and Selector

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

The default selector is round-robin and keeps state across calls. Custom selectors can be registered per server type with `sel.Register(serverType, handler)`.

## Forwarding

Frontend connectors can forward messages whose route starts with the target server type. For route `chat.send`, the forwarder selects a `chat` server and calls RPC service `chat`, method `send`.

```go
fwd := forward.NewForwarder(app, sel)
_ = fwd.Start()
defer fwd.Stop()

tcp.SetForwarder(fwd)
tcp.SetForwardSelector(sel)
```

## Code Generation

```bash
go run ./cmd/codegen ./servers
go run ./cmd/codegen -list ./servers
```

The generated `servers_gen.go` must be included in the build. It registers loader callbacks for Handler, Remote, Filter, and Cron files; `loader.Load()` invokes those callbacks while scanning the `servers/` tree.

## Operational Notes

- Use stable `serverType` names because route forwarding and RPC selection depend on them.
- Set `MaxConns` on connectors to enforce admission control.
- Register backend servers in the registry before creating the selector.
- Run `go test ./...` after code generation and before deployment.
