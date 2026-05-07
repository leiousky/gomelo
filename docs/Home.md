# Gomelo Wiki

欢迎使用 Gomelo 游戏服务器框架！

## 目录

### 开始使用
- [快速开始](Getting-Started.md) - 5分钟快速上手
- [Handler 指南](Handler-Guide.md) - 处理客户端请求
- [Session 管理](Session-Guide.md) - 管理玩家会话
- [分布式部署](Distributed-Guide.md) - 多节点部署架构

### API 参考
- [API 文档](../docs/API.md) - 完整的 API 接口文档

### 核心概念

#### 应用 (App)
- 创建应用：`gomelo.NewApp()`
- 注册 connector 组件：`app.Register(name, connector)`
- 注册路由：`app.On(route, handler)`
- 启动/停止：`app.Start()` / `app.Stop(false)`

#### 会话 (Session)
- 获取当前会话：`ctx.Session()`
- 绑定用户：`session.Bind(uid)`
- 存储数据：`session.Set(key, val)`
- 关闭会话：`session.Close()`

#### 处理器 (Handler)
- 签名：`func(ctx *gomelo.Context)`
- 获取请求：`ctx.Bind(&req)`
- 发送响应：`ctx.Response(data)`
- 获取 Session：`ctx.Session()`

#### 过滤器 (Filter/Middleware)
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

### 分布式

#### Master Server
```go
data, _ := os.ReadFile("config/master.json")
m := master.New()
_ = m.Start(data)
```

#### RPC 调用
```go
client, _ := rpc.NewClient(&rpc.ClientOptions{Host: "127.0.0.1", Port: 3020})
_ = client.Invoke("service", "method", args, &reply)
```

#### 服务发现
```go
reg := server_registry.New()
ch := make(chan []server_registry.ServerInfo, 16)
reg.Watch(ch)
```

### 生产级功能

#### 熔断器
```go
cb := circuitbreaker.NewCircuitBreaker("myService", nil)
cb.Call(func() error {
    return doSomething()
})
```

#### 限流
```go
limiter := ratelimit.NewConnectionLimiter(10000)
if limiter.Allow() {
    // 处理请求
}
```

#### 指标
```go
m := metrics.Global()
m.ObserveHandlerDuration("connector.entry", "success", time.Since(start).Seconds())
http.Handle("/metrics", m.Handler())
```

#### 统一错误码
```go
import "github.com/chuhongliang/gomelo/errors"

ctx.ResponseError(int(errors.BadRequest), "name is required")
```

#### 热更新
```go
import "github.com/chuhongliang/gomelo/reload"

reloader, _ := reload.NewConfigReloader("config.json", func(cfg *config.Config) error {
    app.Set("config", cfg)
    return nil
})
reloader.Start()
```

#### 健康检查
```go
hs := health.NewHealthServer(":8080")
hs.RegisterChecker("db", func() error { return db.Ping() })
hs.Start()
```

### 常见问题

**Q: 如何处理跨服务器调用？**
A: 使用 RPC Client 或 Forwarder 组件。

**Q: 如何实现玩家在线状态？**
A: 使用 Session 的 Bind/UID 功能结合在线玩家 map 管理。

**Q: 如何广播消息？**
A: 使用 Broadcast 服务：
```go
broadcast := broadcast.NewBroadcast("route")
broadcast.BroadcastTo([]string{"uid1", "uid2"}, "msg.route", data)
```

### 项目结构

```
gomelo/
├── gomelo.go      # 入口，导出所有 API
├── lib/           # 核心库
├── rpc/           # RPC 系统
├── connector/     # 网络连接器
├── master/       # Master 服务器
├── registry/     # 服务注册
├── selector/     # 负载均衡
├── broadcast/    # 广播服务
├── forward/     # 消息转发
├── pool/        # 连接池
├── loader/      # Handler/Remote 加载器
├── codec/       # 消息编解码
├── proto/       # protobuf 消息定义
├── errors/      # 统一错误码
├── reload/      # 热更新支持
├── metrics/     # Prometheus 监控
├── benchmark/   # 性能基准测试
└── cmd/        # CLI 工具
```
gomelo/
├── gomelo.go      # 入口，导出所有 API
├── lib/           # 核心库
├── rpc/           # RPC 系统
├── connector/     # 网络连接器
├── master/        # Master 服务器
├── registry/      # 服务注册
├── selector/      # 负载均衡
├── broadcast/     # 广播服务
├── pool/          # 连接池
├── codec/         # 消息编解码
├── proto/         # protobuf 消息定义
└── cmd/           # CLI 工具
```

### 客户端 SDK

#### JavaScript
```javascript
const client = new GomeloClient({ host: 'localhost', port: 3010 });
await client.connect();
const res = await client.request('player.entry', { name: 'Alice' });
```

#### Godot GDScript
```gdscript
var client: GomeloClient
client = GomeloClient.new()
client.connect_to_server("localhost", 3010)
var seq = client.request("player.entry", {"name": "Player1"})
```

### 许可证

MIT
