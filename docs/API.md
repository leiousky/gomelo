# gomelo-go API Reference

## 目录
1. [核心类型 (Core Types)](#1-核心类型-core-types)
2. [应用 (Application)](#2-应用-application)
3. [服务器 (Server)](#3-服务器-server)
4. [会话 (Session)](#4-会话-session)
5. [消息 (Message)](#5-消息-message)
6. [管道 (Pipeline)](#6-管道-pipeline)
7. [路由 (Router)](#7-路由-router)
8. [RPC分布式](#8-rpc分布式)
9. [组与频道](#9-组与频道)
10. [过滤器与插件](#10-过滤器与插件)
11. [日志与配置](#11-日志与配置)
12. [编解码器](#12-编解码器)
13. [工具](#13-工具)

---

## 1. 核心类型 (Core Types)

### App State
应用生命周期状态
```go
const (
    StateInited  int = 1
    StateStart   int = 2
    StateStarted int = 3
    StateStopped int = 4
)
```

### MessageType
消息类型
```go
type MessageType int
const (
    Request MessageType = iota    // 请求消息
    Response                   // 响应消息
    Notify                    // 通知消息(无响应)
    Broadcast                 // 广播消息
)
```

### Level
日志级别
```go
type Level int
const (
    DebugLevel Level = iota
    InfoLevel
    WarnLevel
    ErrorLevel
    FatalLevel
)
```

---

## 2. 应用 (Application)

### 函数

#### NewApp
创建新的应用实例
```go
func NewApp(opts ...lib.AppOption) *lib.App
```
**示例**:
```go
app := gomelo.NewApp(
    gomelo.WithHost("0.0.0.0"),
    gomelo.WithPort(3010),
)
```

#### WithHost
设置监听主机
```go
func WithHost(host string) lib.AppOption
```

#### WithPort
设置监听端口
```go
func WithPort(port int) lib.AppOption
```

#### WithServerID
设置服务器ID
```go
func WithServerID(id string) lib.AppOption
```

#### WithMasterAddr
设置Master服务器地址
```go
func WithMasterAddr(addr string) lib.AppOption
```

### 方法

#### Register
注册组件
```go
func (a *App) Register(name string, comp Component)
```
**参数**:
- `name` - 组件名
- `comp` - 组件实例

#### SetRoute
设置路由处理器
```go
func (a *App) SetRoute(serverType string, handler RouteHandler)
```
**参数**:
- `serverType` - 服务器类型
- `handler` - 路由处理函数

#### Use
添加中间件
```go
func (a *App) Use(m Middleware)
```
**参数**:
- `m` - 中间件函数

#### On
注册请求处理器
```go
func (a *App) On(route string, handler HandlerFunc)
```
**参数**:
- `route` - 路由字符串
- `handler` - 处理函数

**示例**:
```go
app.On("connector.entry", func(ctx *gomelo.Context) {
    var req struct {
        Token string `json:"token"`
    }
    ctx.Bind(&req)
    ctx.Response(map[string]any{
        "code": 0,
        "msg": "ok",
    })
})
```

#### Configure
配置服务器
```go
func (a *App) Configure(fn func(*Server))
func (a *App) ConfigureWithEnv(env string, serverType ...string) func(fn func(*Server))
```
**参数**:
- `fn` - 配置函数

#### Start
启动应用
```go
func (a *App) Start() error
```

#### Stop
停止应用
```go
func (a *App) Stop(force bool) error
```

#### Wait
等待信号并优雅关闭
```go
func (a *App) Wait()
```

### 接口

#### Component
组件接口
```go
type Component interface {
    Name() string
    Start(app *App) error
    Stop()
}
```
所有自定义组件必须实现此接口

---

## 3. 服务器 (Server)

### 函数

#### newServer
创建服务器实例(内部使用)
```go
func newServer(serverType string, app *App) *Server
```

### 方法

#### Type
获取服务器类型
```go
func (s *Server) Type() string
```

#### SetHost
设置监听主机
```go
func (s *Server) SetHost(host string)
```

#### SetPort
设置监听端口
```go
func (s *Server) SetPort(port int)
```

#### SetFrontend
设置为前端服务器
```go
func (s *Server) SetFrontend(frontend bool)
```

#### OnConnection
设置连接回调
```go
func (s *Server) OnConnection(fn ConnectHandler)
```
**参数**:
- `fn` - 连接回调函数,接收 `*Session`

#### OnMessage
设置消息回调
```go
func (s *Server) OnMessage(fn MessageHandler)
```
**参数**:
- `fn` - 消息回调函数

#### OnClose
设置关闭回调
```go
func (s *Server) OnClose(fn CloseHandler)
```

#### Start
启动服务器
```go
func (s *Server) Start() error
```

#### Stop
停止服务器
```go
func (s *Server) Stop()
```

---

## 4. 会话 (Session)

### 函数

#### NewSession
创建新会话
```go
func NewSession() *Session
```

### 方法

#### Get
获取会话数据
```go
func (s *Session) Get(key string) any
```
**参数**:
- `key` - 键名

#### Set
设置会话数据
```go
func (s *Session) Set(key string, value any)
```
**参数**:
- `key` - 键名
- `value` - 值

#### Remove
删除会话数据
```go
func (s *Session) Remove(key string)
```

#### Bind
绑定用户ID
```go
func (s *Session) Bind(uid string)
```
**参数**:
- `uid` - 用户ID

#### Close
关闭会话
```go
func (s *Session) Close()
```

#### IsClosed
检查会话是否已关闭
```go
func (s *Session) IsClosed() bool
```

#### KV
获取所有会话数据
```go
func (s *Session) KV() map[string]any
```

### 字段

#### ID
会话唯一ID
```go
s.ID uint64
```

#### UID
用户ID
```go
s.UID string
```

#### ServerID
服务器ID
```go
s.ServerID string
```

#### ServerType
服务器类型
```go
s.ServerType string
```

---

## 5. 消息 (Message)

### 类型定义
```go
type Message struct {
    Type  MessageType
    Route string
    Seq   uint64
    Body  any
}
```

### 方法

#### Encode
编码消息
```go
func (m *Message) Encode() ([]byte, error)
```

#### Decode
解码消息
```go
func (m *Message) Decode(data []byte) error
```

---

## 6. 管道 (Pipeline)

### 函数

#### NewPipeline
创建新管道
```go
func NewPipeline() *Pipeline
```

### 方法

#### Use
添加中间件
```go
func (p *Pipeline) Use(m Middleware)
```

#### On
注册路由处理器
```go
func (p *Pipeline) On(route string, handler HandlerFunc)
```

#### GetHandlers
获取处理器链
```go
func (p *Pipeline) GetHandlers(route string) []HandlerFunc
```

#### Invoke
执行处理器
```go
func (p *Pipeline) Invoke(ctx *Context)
```

### 类型

#### HandlerFunc
请求处理函数
```go
type HandlerFunc func(*Context)
```

#### Middleware
中间件函数
```go
type Middleware func(HandlerFunc) HandlerFunc
```

---

## 7. 路由 (Router)

### 函数

#### NewRouter
创建新路由器
```go
func NewRouter() *Router
```

### 方法

#### SetRoute
设置路由
```go
func (r *Router) SetRoute(serverType string, handler RouteHandler)
```

#### GetRoute
获取路由
```go
func (r *Router) GetRoute(serverType string) (RouteHandler, bool)
```

### 类型

#### RouteHandler
路由处理函数
```go
type RouteHandler func(*Context, string)
```

---

## 8. RPC分布式

### 8.1 客户端

#### RPCClient
RPC客户端接口
```go
type RPCClient interface {
    Close()
    Invoke(service, method string, args, reply any) error
    InvokeCtx(ctx context.Context, service, method string, args, reply any) error
    Notify(service, method string, args any) error
}
```

#### ClientOptions
RPC客户端选项
```go
type ClientOptions struct {
    Host            string
    Port            int
    MaxConns        int
    MinConns        int
    KeepAlive       time.Duration
    IdleTime        time.Duration
    Timeout         time.Duration
    MaxResponseSize int
}
```

### 8.2 服务器

#### RPCServer
RPC服务器接口
```go
type RPCServer interface {
    Register(service string, impl any) error
    Start() error
    Stop()
    Addrs() map[string]string
}
```

Remote 方法签名：

```go
func (r *ChatRemote) Send(ctx context.Context, args struct {
    RoomID string `json:"roomId"`
    Text   string `json:"text"`
}) (any, error)
```

RPC 服务端会把 JSON 参数转换为声明的 args 类型，并把返回的 error 传回调用方。

### 8.3 注册中心

#### Registry
服务注册中心接口
```go
type ServerRegistry interface {
    Register(server ServerInfo) error
    Unregister(serverID string) error
    GetServer(serverID string) (ServerInfo, bool)
    GetServersByType(serverType string) []ServerInfo
    GetAllServers() []ServerInfo
    GetServerTypes() []string
    Watch(ch chan<- []ServerInfo)
    SetEventHandler(handler RegistryEventHandler)
    Close()
}
```

#### ServerInfo
服务器信息
```go
type ServerInfo struct {
    ID         string
    ServerType string
    Host       string
    Port       int
    Frontend   bool
    State      int
    Count      int
    RegisterAt int64
    LastUpdate int64
    Metadata   map[string]any
}
```
#### New
创建本地注册中心
```go
func server_registry.New() ServerRegistry
```

### 8.4 选择器

#### Selector
服务器选择器接口
```go
type Selector interface {
    Select(serverType string) server_registry.ServerInfo
    SelectMulti(serverType string, n int) []server_registry.ServerInfo
    Register(serverType string, handler SelectorHandler)
    GetStats() (total, fail int64)
}
```

#### SelectorHandler
选择器处理函数
```go
type SelectorHandler func([]server_registry.ServerInfo) server_registry.ServerInfo
```

#### NewSelector
创建选择器
```go
func selector.New(reg server_registry.ServerRegistry) Selector
```

#### LoadBalancer
轮询负载均衡器
```go
type LoadBalancer struct {
    curIndex map[string]int
}
```

#### NewLoadBalancer
创建负载均衡器
```go
func NewLoadBalancer() *LoadBalancer
```

---

## 9. 组与频道

### 9.1 组

#### Group
玩家分组接口
```go
type Group interface {
    Add(s *Session)
    Remove(s *Session)
    Contains(sid uint64) bool
    Members() []*Session
    Size() int
    Clear()
}
```

#### GroupManager
组管理器接口
```go
type GroupManager interface {
    CreateGroup(gid string) (Group, bool)
    GetGroup(gid string) (Group, bool)
}
```

### 9.2 频道

#### Channel
频道接口
```go
type Channel interface {
    Add(s *Session)
    Remove(s *Session)
    Members() []*Session
    Size() int
    Clear()
    GetName() string
}
```

#### ChannelManager
频道管理器接口
```go
type ChannelManager interface {
    Create(name string) Channel
    Get(name string) (Channel, bool)
    Remove(name string)
}
```

### 9.3 广播

#### BroadcastService
广播服务接口
```go
type BroadcastService interface {
    Broadcast(route string, msg any)
    BroadcastTo(route string, msg any, uids ...string)
    BroadcastByIDs(route string, msg any, ids ...uint64)
    Add(ids ...uint64) error
    Remove(ids ...uint64) error
    Size() int
    Clear()
}
```

#### NewBroadcast
创建广播服务
```go
func NewBroadcast(route string) BroadcastService
```

### 9.4 推送

#### PushService
推送服务接口
```go
type PushService interface {
    Push(uid string, route string, msg any)
    PushBatch(uids []string, route string, msg any)
}
```

---

## 10. 过滤器与插件

### 10.1 过滤器

#### Filter
过滤器接口
```go
type Filter interface {
    Process(ctx *Context) bool
    After(ctx *Context)
}
```

#### FilterFunc
过滤器函数类型
```go
type FilterFunc func(*Context) bool
```

#### FilterChain
过滤器链
```go
type FilterChain struct {
    filters []Filter
}
```

#### newFilterChain
创建过滤器链
```go
func newFilterChain() *FilterChain
```

#### FilterChain.Add
添加过滤器
```go
func (c *FilterChain) Add(f Filter)
```

#### FilterChain.Process
执行过滤器链
```go
func (c *FilterChain) Process(ctx *Context) bool
```

### 10.2 插件

#### Plugin
插件接口
```go
type Plugin interface {
    Name() string
    Initialize() error
    AfterInitialize() error
    BeforeStart() error
    AfterStart() error
    BeforeStop() error
    AfterStop() error
}
```

#### PluginManager
插件管理器接口
```go
type PluginManager interface {
    Get(name string) (Plugin, bool)
    GetAll() []Plugin
    Install(p Plugin) error
    Uninstall(name string) error
}
```

---

## 11. 日志与配置

### 11.1 日志

#### Logger
日志器
```go
type Logger struct {
    mu     sync.Mutex
    level  Level
    output io.Writer
    prefix string
}
```

#### NewLogger
创建日志器
```go
func NewLogger(output io.Writer, prefix string) *Logger
```

#### Logger.SetLevel
设置日志级别
```go
func (l *Logger) SetLevel(level Level)
```

#### Logger.Debug/Info/Warn/Error/Fatal
输出日志
```go
func (l *Logger) Debug(msg string)
func (l *Logger) Info(msg string)
func (l *Logger) Warn(msg string)
func (l *Logger) Error(msg string)
func (l *Logger) Fatal(msg string)
```

#### 包级别日志函数
```go
func Debug(msg string)
func Info(msg string)
func Warn(msg string)
func Error(msg string)
func Fatal(msg string)
func SetLevel(level Level)
```

### 11.2 配置

#### Config
应用配置
```go
type Config struct {
    Server ServerConfig   `json:"server"`
    RPC   RPCConfig     `json:"rpc"`
    Servers ServersConfig `json:"servers"`
    Log   LogConfig    `json:"log"`
}
```

#### ServerConfig
服务器配置
```go
type ServerConfig struct {
    Host string `json:"host"`
    Port int    `json:"port"`
}
```

#### LoadConfig
加载配置文件
```go
func LoadConfig(path string) (*Config, error)
```

---

## 12. 编解码器

### 12.1 Codec 接口

```go
type Codec interface {
    Encode(msg *lib.Message) ([]byte, error)
    Decode(data []byte) (*lib.Message, error)
}
```

### 12.2 JSONCodec

```go
func NewJSONCodec() *JSONCodec
func (c *JSONCodec) Encode(msg *lib.Message) ([]byte, error)
func (c *JSONCodec) Decode(data []byte) (*lib.Message, error)
```

### 12.3 ProtobufCodec

真正的 Protocol Buffers 编解码器，支持类型自动注册和反序列化。

```go
type ProtobufCodec struct {
    routes map[string]uint16
    ids    map[uint16]string
    types  map[string]reflect.Type
    nextID uint16
    mu     sync.RWMutex
}
```

#### 创建和注册

```go
func NewProtobufCodec() *ProtobufCodec

// 注册路由（用于 route ID 压缩）
func (c *ProtobufCodec) RegisterRoute(route string) uint16

// 注册类型（用于自动反序列化）
func (c *ProtobufCodec) RegisterType(route string, msg proto.Message)
```

#### 编码/解码

```go
func (c *ProtobufCodec) Encode(msg *lib.Message) ([]byte, error)
func (c *ProtobufCodec) Decode(data []byte) (*lib.Message, error)
```

#### 协议格式

```
[1 byte]  type      - 消息类型 (1=Request, 2=Response, 3=Notify, 4=Error)
[1 byte]  flag      - 0x01=route ID, 0x00=route string
[2/N bytes] route  - route ID (2字节) 或 route string (N字节+0结束)
[8 bytes] seq       - sequence number (big-endian)
[N bytes] body      - protobuf 编码的数据
```

#### 使用示例

```go
codec := codec.NewProtobufCodec()
codec.RegisterRoute("player.entry", 1)
codec.RegisterRoute("player.move", 2)

// 注册类型后，Decode 自动反序列化
codec.RegisterType("player.entry", &proto.EntryRequest{})

// 编码（body 必须是 proto.Message）
msg := &lib.Message{
    Type:  lib.Request,
    Route: "player.entry",
    Seq:   1,
    Body:  &proto.EntryRequest{Name: "Alice"},
}
data, _ := codec.Encode(msg)

// 解码（自动反序列化到注册的类型）
decoded, _ := codec.Decode(data)
// decoded.Body 类型为 *proto.EntryRequest
```

### 12.4 路由压缩

```go
type RouteCompressor struct {
    routes map[string]uint16
    ids    map[uint16]string
    nextID uint16
}

func NewRouteCompressor() *RouteCompressor
func (c *RouteCompressor) Register(route string) uint16
func (c *RouteCompressor) Compress(route string) uint16
func (c *RouteCompressor) Decompress(id uint16) string
```

---

## 13. 工具

### 13.1 CLI命令行

```bash
# 初始化项目
gomelo init <name>

# 启动
gomelo start

# 构建
gomelo build

# 清理
gomelo clean
```

### 13.2 Benchmark

```bash
# 运行性能测试
go run cmd/benchmark/main.go -host localhost:3010 -users 100 -duration 10
```

### 13.3 Robot

```bash
# 运行压力测试机器人
go run cmd/robot/main.go -host localhost:3010 -users 100 -duration 60
```

### 13.4 Admin

```bash
# 启动管理后台
go run cmd/admin/main.go -http :3005
```

---

## 上下文 (Context)

### Context
请求上下文

#### Context.App
获取应用实例
```go
func (c *Context) App() *App
```

#### Context.Session
获取会话
```go
func (c *Context) Session() *Session
```

#### Context.Request
获取请求消息
```go
func (c *Context) Request() *Message
```

#### Context.Bind
绑定请求数据
```go
func (c *Context) Bind(v any) error
```

#### Context.Response
发送响应
```go
func (c *Context) Response(body any)
```

#### Context.Next
调用下一个处理器
```go
func (c *Context) Next()
```

---

## 使用示例

### 完整示例
```go
package main

import (
    "log"

    "github.com/chuhongliang/gomelo"
    "github.com/chuhongliang/gomelo/connector"
)

func main() {
    // 创建应用
    app := gomelo.NewApp(
        gomelo.WithHost("0.0.0.0"),
        gomelo.WithPort(3010),
    )
    
    conn := connector.NewServer(&connector.ServerOptions{
        Type: "tcp",
        Host: "0.0.0.0",
        Port: 3010,
    })
    conn.OnConnect(func(session *gomelo.Session) {
        log.Printf("New connection: %d", session.ID())
    })
    app.Register("connector", conn)
    
    // 添加中间件
    app.Use(func(next gomelo.HandlerFunc) gomelo.HandlerFunc {
        return func(ctx *gomelo.Context) {
            log.Println("Before request")
            next(ctx)
            log.Println("After request")
        }
    })
    
    // 注册处理器
    app.On("connector.entry", func(ctx *gomelo.Context) {
        var req struct {
            Token string `json:"token"`
        }
        ctx.Bind(&req)
        
        ctx.Session().Set("user", "test")
        
        ctx.Response(map[string]any{
            "code": 0,
            "msg": "ok",
        })
    })
    
    // 启动
    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
    
    // 等待关闭信号
    app.Wait()
}
```
