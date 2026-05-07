# Gomelo Wiki

Welcome to the Gomelo game server framework!

## Table of Contents

### Getting Started
- [Getting Started](Getting-Started.md) - 5-minute quick start
- [Handler Guide](Handler-Guide.md) - Handle client requests
- [Session Guide](Session-Guide.md) - Manage player sessions
- [Distributed Guide](Distributed-Guide.md) - Multi-node deployment

### API Reference
- [API Documentation](../docs/API.md) - Complete API reference

### Core Concepts

#### Application (App)
- Create app: `gomelo.NewApp()`
- Register connector components: `app.Register(name, connector)`
- Register route: `app.On(route, handler)`
- Start/Stop: `app.Start()` / `app.Stop(false)`

#### Session
- Get current: `ctx.Session()`
- Bind user: `session.Bind(uid)`
- Store data: `session.Set(key, val)`
- Close session: `session.Close()`

#### Handler
- Signature: `func(ctx *gomelo.Context)`
- Get request: `ctx.Bind(&req)`
- Send response: `ctx.Response(data)`
- Get Session: `ctx.Session()`

#### Filter/Middleware
```go
type AuthFilter struct{}

func (AuthFilter) Name() string { return "auth" }
func (AuthFilter) Process(ctx *gomelo.Context) bool {
    if ctx.Session().Get("token") == nil {
        ctx.Response(map[string]any{"code": 401})
        return false
    }
    return true
}
func (AuthFilter) After(ctx *gomelo.Context) {}

app.Before(AuthFilter{})
```

### Distributed

#### Master Server
```go
data, _ := os.ReadFile("config/master.json")
m := master.New()
_ = m.Start(data)
```

#### RPC Call
```go
client, _ := rpc.NewClient(&rpc.ClientOptions{Host: "127.0.0.1", Port: 3020})
_ = client.Invoke("service", "method", args, &reply)
```

#### Service Discovery
```go
reg := server_registry.New()
ch := make(chan []server_registry.ServerInfo, 16)
reg.Watch(ch)
```

### Production Features

#### Circuit Breaker
```go
cb := circuitbreaker.NewCircuitBreaker("myService", nil)
cb.Call(func() error {
    return doSomething()
})
```

#### Rate Limiting
```go
limiter := ratelimit.NewConnectionLimiter(10000)
if limiter.Allow() {
    // Handle request
}
```

#### Metrics
```go
registry := metrics.NewMetricsRegistry()
counter := registry.Counter("requests_total")
counter.Inc()
```

#### Health Check
```go
hs := health.NewHealthServer(":8080")
hs.RegisterChecker("db", func() error { return db.Ping() })
hs.Start()
```

### FAQ

**Q: How to handle cross-server calls?**
A: Use RPC Client or Forwarder component.

**Q: How to implement player online status?**
A: Use Session's Bind/UID feature combined with online player map management.

**Q: How to broadcast messages?**
A: Use Broadcast service:
```go
broadcast := broadcast.NewBroadcast("route")
broadcast.BroadcastTo([]string{"uid1", "uid2"}, "msg.route", data)
```

### Project Structure

```
gomelo/
├── gomelo.go      # Entry, exports all APIs
├── lib/           # Core library
├── rpc/           # RPC system
├── connector/     # Network connector
├── master/        # Master server
├── registry/      # Service registry
├── selector/      # Load balancer
├── broadcast/     # Broadcast service
├── forward/       # Message forwarder
├── pool/          # Connection pool
├── loader/       # Handler/Remote loader
├── codec/         # Message codec
├── proto/         # Protocol buffer definitions
├── errors/        # Unified error codes
├── reload/        # Hot reload support
├── metrics/       # Prometheus metrics
├── benchmark/     # Performance benchmarks
└── cmd/           # CLI tools
```

### License

MIT
