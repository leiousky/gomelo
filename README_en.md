**[English](README_en.md)** | [简体中文](README.md)

---

# Gomelo

A high-performance distributed game server framework written in Go, inspired by Node.js Pomelo architecture.

## Features

- **Multi-Protocol Support** - TCP, WebSocket, and UDP network protocols
- **Distributed Architecture** - Multi-node deployment with frontend/backend separation
- **High-Performance RPC** - Connection pool reuse, async message forwarding
- **Type Safe** - Strongly typed Filter interfaces and Handler signatures
- **Service Discovery** - Master coordination + Registry dual mode with auto-reconnect
- **Load Balancing** - Round-robin, consistent hash, weighted random strategies
- **Batch Broadcast** - Async batch push, supports UID/ID grouping
- **Production Ready** - Circuit breaker, rate limiting, metrics, health checks
- **Graceful Shutdown** - Timeout control ensuring task completion
- **Hot Config Reload** - File watching + signal triggering (SIGHUP/SIGUSR1)
- **Multi-language Clients** - JavaScript, GDScript, C#, TypeScript, Go, Java with full binary protocol support
- **Unified Error Codes** - Standardized error code system for easier client handling
- **Prometheus Metrics** - Out-of-the-box performance metrics monitoring
- **Performance Benchmarks** - Built-in Benchmark test suite
- **Cron Scheduling** - crontab-style task scheduling

## Requirements

- Go 1.21+

## Quick Start

### 1. Install CLI

```bash
# Method 1: go install (recommended, Go 1.16+)
go install github.com/chuhongliang/gomelo/cmd/gomelo@latest

# Method 2: Manual build
git clone https://github.com/chuhongliang/gomelo.git
cd gomelo
go build -o bin/gomelo ./cmd/gomelo
```

### 2. Initialize Project

```bash
gomelo init mygame
cd mygame
go mod tidy
```

### 3. Start Project

```bash
go run .
```

## Multi-Protocol Configuration

gomelo supports TCP, WebSocket, and UDP network protocols. Choose the appropriate protocol based on your game type.

### TCP (Default, Recommended for Real-time Action Games)

```go
import "github.com/chuhongliang/gomelo/connector"

tcp := connector.NewServer(&connector.ServerOptions{
    Type:              "tcp",
    Host:              "0.0.0.0",
    Port:              3010,
    MaxConns:          10000,
    HeartbeatInterval: 30 * time.Second,
    HeartbeatTimeout:  90 * time.Second,
})
app.Register("connector-tcp", tcp)
```

### WebSocket (Suitable for HTML5 Games, Mobile)

```go
import "github.com/chuhongliang/gomelo/connector"

ws := connector.NewWebSocketServer(&connector.WebSocketOptions{
    Type:              "ws",
    Host:              "0.0.0.0",
    Port:              3011,
    MaxConns:          5000,
    HeartbeatInterval: 30 * time.Second,
    HeartbeatTimeout:  90 * time.Second,
})
app.Register("connector-ws", ws)
```

### UDP (For Latency-Sensitive Games: MOBA, FPS, Rhythm)

```go
import "github.com/chuhongliang/gomelo/connector"

udp := connector.NewUDPServer(&connector.UDPServerOptions{
    Type:              "udp",
    Host:              "0.0.0.0",
    Port:              3012,
    MaxConns:          5000,
    HeartbeatInterval: 10 * time.Second,
    HeartbeatTimeout:  30 * time.Second,
})
udp.SetApp(app)
go udp.Start()
defer udp.Stop()
```

### Protocol Comparison

| Protocol | Latency | Reliability | Use Case | Max Connections |
|----------|---------|-------------|----------|-----------------|
| TCP | Medium | Reliable | General games | 10000+ |
| WebSocket | Medium | Reliable | H5/Mobile | 5000+ |
| UDP | Low | Unreliable | MOBA/FPS/Rhythm | 5000+ |

### Mixed Deployment

The same server can listen on multiple protocols:

```go
// TCP + WebSocket
tcp := connector.NewServer(&connector.ServerOptions{Host: "0.0.0.0", Port: 3010})
app.Register("connector-tcp", tcp)

ws := connector.NewWebSocketServer(&connector.WebSocketOptions{Host: "0.0.0.0", Port: 3011})
app.Register("connector-ws", ws)
```

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
└──
```

## Example Code

### Minimal Entry (main.go)

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

	log.Println("Starting server...")
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
	log.Println("Server started!")

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

	ctx.Session().Set("uid", "user-"+strconv.FormatUint(ctx.Session().ID(), 10))

	ctx.Response(map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{
			"uid": ctx.Session().Get("uid"),
		},
	})
}
```

Auto-generated route: `connector.entryHandler.entry`

### Unified Error Codes

```go
import "github.com/chuhongliang/gomelo/errors"

func (h *EntryHandler) Entry(ctx *gomelo.Context) {
	var req struct {
		Name string `json:"name"`
	}
	ctx.Bind(&req)

	if req.Name == "" {
		ctx.ResponseError(int(errors.BadRequest), "name is required")
		return
	}

	// Use error codes
	ctx.Response(map[string]any{
		"code": errors.OK,
		"msg":  "ok",
	})
}
```

Error code ranges:
| Range | Purpose |
|-------|---------|
| 0 | OK |
| 400-499 | HTTP client errors |
| 1001-1009 | Route/message errors |
| 2001-2006 | RPC errors |
| 3001-3003 | Registry errors |
| 4001-4003 | Pool errors |
| 5001-5006 | Network errors |
| 6001-6004 | Auth errors |
| 7001-7006 | Game business errors |

### Prometheus Metrics

```go
import "github.com/chuhongliang/gomelo/metrics"

// Initialize global metrics
m := metrics.Global()

// Use in Handler
m.ObserveHandlerDuration("connector.entry", "success", time.Since(start).Seconds())

// Expose /metrics endpoint
http.Handle("/metrics", m.Handler())
```

### Hot Config Reload

```go
import "github.com/chuhongliang/gomelo/reload"

// Create config reloader
reloader, _ := reload.NewConfigReloader("config.json", func(cfg *config.Config) error {
	app.Set("config", cfg)
	return nil
})

// Start watching
reloader.Start()

// Also supports signal triggering (SIGHUP/SIGUSR1)
```

### Performance Benchmark

```bash
# Run benchmarks
go test -bench=. ./benchmark/...

# Run specific test
go test -bench=MessageEncodeDecode -benchtime=1s ./benchmark/...
```

## Distributed Architecture

```
                         ┌─────────────┐
                         │   Master    │  ← Service Coordination
                         └─────────────┘
                                │
     ┌──────────────────────────┼──────────────────────────┐
     │                          │                          │
 ┌───▼────┐              ┌──────▼──────┐              ┌──────▼──────┐
 │connector│              │  connector  │              │  connector  │  ← Frontend Layer
 │(Frontend)│             │  (Frontend) │              │  (Frontend) │
 └────┬────┘              └──────┬──────┘              └──────┬──────┘
      │                          │                              │
      └──────────────────────────┼──────────────────────────────┘
                                 │ RPC
                     ┌───────────┼───────────┐
                     │           │           │
               ┌─────▼─────┐┌───▼────┐┌─────▼─────┐
               │    chat    ││   game  ││   auth   │  ← Backend Layer
               └───────────┘└─────────┘└───────────┘
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `gomelo init <name>` | Initialize new project |
| `gomelo add <type>` | Add server type (connector/chat/gate/auth/game/match) |
| `gomelo start` | Start application |
| `gomelo build` | Build application |
| `gomelo clean` | Clean build artifacts |
| `gomelo routes` | List all registered routes |
| `gomelo list` | Show running servers |
| `gomelo -v` | Show version |
| `gomelo -h` | Show help |

## Auto Route Registration

Use codegen to automatically scan server code and generate registration:

```bash
# Generate registration code
go run ./cmd/codegen ./servers

# List routes without generating code
go run ./cmd/codegen -list ./servers
```

Scans `servers/{serverType}/handler/`, `remote/`, `filter/`, and `cron/` directories and generates `servers_gen.go`. Include the generated file in your build; it registers loader callbacks during package init, and `loader.Load()` activates them at runtime.

The generated file supports multiple Handler/Remote/Filter/Cron types in the same source file.

See [Handler-Guide.md](docs/Handler-Guide.md) for details.

## Core API

### App

| Method | Description |
|--------|-------------|
| `NewApp(opts...)` | Create app instance |
| `WithHost(host)` | Set listen address |
| `WithPort(port)` | Set listen port |
| `WithServerID(id)` | Set server ID |
| `WithMasterAddr(addr)` | Set Master address |
| `Configure(fn)` | Run a simple server configuration callback |
| `ConfigureWithEnv(env, serverType...)` | Run a configuration callback for an environment/server type |
| `On(route, handler)` | Register route handler |
| `Before(filter)` | Register pre-filter |
| `After(filter)` | Register post-filter |
| `Start()` | Start app |
| `Stop(force)` | Stop app |
| `Wait()` | Block waiting for signals |

### Context

| Method | Description |
|--------|-------------|
| `Session()` | Get current Session |
| `Request()` | Get current Message |
| `Bind(v)` | Parse request data |
| `Response(v)` | Send response |
| `ResponseError(code, msg)` | Send error response |
| `Next()` | Call next handler |

### Session

| Method | Description |
|--------|-------------|
| `ID()` | Get session ID |
| `UID()` | Get bound user ID |
| `Bind(uid)` | Bind user ID |
| `Set(key, val)` | Store data |
| `Get(key)` | Get data |
| `Remove(key)` | Delete data |
| `Close()` | Close session |

### Server

| Method | Description |
|--------|-------------|
| `SetFrontend(v)` | Set as frontend server |
| `SetPort(port)` | Set port |
| `SetHost(host)` | Set address |
| `SetServerType(t)` | Set server type |
| `OnConnection(fn)` | Connection callback |
| `OnMessage(fn)` | Message callback |
| `OnClose(fn)` | Close callback |

Connector servers also expose protocol-specific callbacks such as `OnConnect`, `OnMessage`, and `OnClose`.

### RPC

| Method | Description |
|--------|-------------|
| `NewRPCClientManager(reg, selector, opts)` | Create an RPC client manager |
| `app.SetRPCClientManager(mgr)` | Attach RPC manager to app |
| `app.RPCTo(ctx, serverType, method, args, reply)` | Load-balanced RPC call by server type |

Example:
```go
type ChatRemote struct{}

func (r *ChatRemote) Send(ctx context.Context, args struct {
    RoomID string `json:"roomId"`
    Text   string `json:"text"`
}) (any, error) {
    return map[string]any{"code": 0}, nil
}

if err := app.RPCTo(context.Background(), "chat", "Send", req, &resp); err != nil {
    log.Printf("rpc error: %v", err)
}
```

Remote methods use the contract `func(context.Context, Args) (any, error)`. JSON request arguments are converted into the declared `Args` type before invocation, and returned errors are sent back to the caller.

## Directory Structure

```
gomelo/
├── gomelo.go           # Entry, exports all public APIs
├── lib/                 # Core library
│   ├── app.go          # Application
│   ├── session.go      # Session management
│   ├── context.go      # Request context
│   ├── router.go       # Router
│   ├── event.go        # Event emitter
│   ├── metrics.go      # Metrics
│   ├── health.go       # Health check
│   └── shutdown.go     # Graceful shutdown
├── rpc/                 # RPC system
│   ├── client.go       # RPC client + connection pool
│   └── server.go       # RPC server
├── connector/           # Network connector
│   ├── tcp_server.go   # TCP Server
│   ├── udp_server.go   # UDP Server
│   └── ws_server.go    # WebSocket Server
├── master/             # Master server
├── registry/           # Service registry
├── selector/           # Load balancer
├── forward/            # Message forwarder
├── broadcast/           # Broadcast service
├── scheduler/          # Task scheduling (including Cron)
├── pool/               # Connection pool + WorkerPool
├── loader/             # Handler/Remote loader
├── codec/              # Message codec (JSON/Protobuf)
├── proto/              # Protocol buffer definitions
├── errors/             # Unified error codes
├── reload/             # Hot reload support
├── metrics/            # Prometheus metrics
├── benchmark/          # Performance benchmarks
├── client/             # Client SDKs
│   ├── js/            # JavaScript client
│   ├── godot/         # Godot GDScript client
│   ├── unity/         # Unity C# client
│   ├── cocos/         # Cocos Creator TypeScript client
│   ├── go/            # Go client
│   └── java/          # Java/Android client
└── cmd/               # CLI tools
    ├── gomelo/        # gomelo CLI
    ├── demo/          # Demo
    └── codegen/       # Code generator
```

## Client SDK

### JavaScript Client

```javascript
import { GomeloClient, MessageType } from './client/js/client.js';

const client = new GomeloClient({ host: 'localhost', port: 3010 });
await client.connect();

// Register route (optional)
client.registerRoute('connector.entry', 1);

// request-response
const res = await client.request('connector.entry', { name: 'Alice' });

// notify (no response)
client.notify('player.move', { position: { x: 1, y: 2, z: 3 } });

// event listener
client.on('onChat', (msg) => console.log('Chat:', msg));
```

### Go Client

```go
import "github.com/chuhongliang/gomelo/client/go"

client := go.NewClient(go.ClientOptions{
    Host:                 "localhost",
    Port:                 3010,
    HeartbeatInterval:    30 * time.Second,
    ReconnectInterval:    3 * time.Second,
    MaxReconnectAttempts: 5,
})

client.OnConnected(func() { fmt.Println("Connected") })
client.OnDisconnected(func() { fmt.Println("Disconnected") })
client.OnError(func(err error) { fmt.Printf("Error: %v\n", err) })

if err := client.Connect(); err != nil {
    log.Fatal(err)
}
defer client.Disconnect()

resp, err := client.Request("connector.entry", map[string]interface{}{"name": "Alice"})
```

### Java Client

```java
import com.gomelo.GomeloClient;

GomeloClient client = new GomeloClient();
client.setHost("localhost");
client.setPort(3010);

client.onConnected(() -> System.out.println("Connected"));
client.onDisconnected(() -> System.out.println("Disconnected"));
client.onError(e -> System.err.println("Error: " + e));

client.connect("localhost", 3010);

Object resp = client.requestSync("connector.entry", new Object[]{"Alice"});
```

### Unity C# Client

```csharp
using Gomelo;

public class GameManager : MonoBehaviour
{
    private GomeloClient _client;

    void Start()
    {
        _client = gameObject.AddComponent<GomeloClient>();
        _client.OnConnected += OnConnected;
        _client.OnError += (msg) => Debug.LogError("Error: " + msg);
        _client.Connect("localhost", 3010);

        // Register route
        _client.RegisterRoute("player.entry", 1);

        // Event listener
        _client.On("onChat", (body) => Debug.Log("Chat: " + body));
    }

    void OnConnected()
    {
        _client.Request("player.entry", new { name = "Player1" },
            (body) => Debug.Log("Success: " + body),
            (err) => Debug.LogError("Error: " + err));

        _client.Notify("player.move", new { x = 100, y = 200 });
    }
}
```

### Godot GDScript Client

```gdscript
var client: GomeloClient

func _ready():
    client = GomeloClient.new()
    add_child(client)
    client.connect_to_server("localhost", 3010)
    client.connected.connect(_on_connected)

func _on_connected():
    var seq = client.request("player.entry", {"name": "Player1"})
    client.on("onChat", func(body): print("Chat: ", body))
    client.notify("player.move", {"position": {"x": 1, "y": 2, "z": 3}})
```

### Cocos Creator TypeScript Client

```typescript
import { GomeloClient } from './GomeloClient';

export class GameManager extends cc.Component {
    private client!: GomeloClient;

    start() {
        this.client = this.addComponent(GomeloClient);
        this.client.connect('localhost', 3010);

        this.client.onConnected = () => {
            console.log('Connected!');
            this.client.request('connector.entry', { name: 'Player1' });
        };

        this.client.on('onChat', (data) => {
            console.log('Chat:', data);
        });
    }
}
```

## Comparison with Node.js Pomelo

| Feature | Node.js Pomelo | gomelo |
|---------|---------------|--------|
| Install | `npm install -g pomelo` | `go build ./cmd/gomelo` |
| Init | `pomelo init mygame` | `gomelo init mygame` |
| Start | `node start.js` | `go run .` |
| Entry file | `start.js` | `main.go` |
| Handler signature | `function(session, msg, next)` | `func(ctx *Context)` |
| Filter interface | `before/after filter` | `Before/After filter` |
| RPC | `pomelo.rpc.invoke` | `client.Invoke(service, method, args, reply)` |

## Performance

- RPC connection pool reuse: >90%
- Message forwarding latency: <1ms
- Single node connections: 10000+
- Goroutine pooling to avoid unlimited creation

## Documentation

- [Handler Guide](docs/Handler-Guide.md)
- [Getting Started](docs/Getting-Started.md)
- [Session Guide](docs/Session-Guide.md)
- [Distributed Guide](docs/Distributed-Guide.md)
- [API Reference](docs/API.md)

## Client Documentation

- [JavaScript Client](../client/js)
- [Go Client](../client/go)
- [Java Client](../client/java)
- [Unity Client](../client/unity)
- [Godot Client](../client/godot)
- [Cocos Client](../client/cocos)

## License

MIT
