# 更新日志

本文档记录 gomelo 的所有重要变更。
## [1.5.5] - 2026-05-13

### 修复

#### 关键并发与安全修复 (P0)
- **connector/ws_server.go** - 移除全局 `upgrader` 变量，改为每个 WebSocketServer 实例持有独立 upgrader，消除多实例竞态
- **pool/pool.go** - 删除 `Get()` 错误路径上的 `atomic.AddInt64(&p.totalConns, -1)`，修复 totalConns 负数问题
- **connector/tcp_server.go** - msgCh 满时改为丢弃消息并告警，而非直接关闭连接踢人
- **connector/tcp_server.go** - dispatchMessages 缓冲区溢出时断开连接而非静默截断，防止内存无限增长
- **lib/message.go** - `DecodeBody()` 撤销硬编码 JSON 反序列化，改为委托 `defaultCodec.DecodeBody()`，支持 Protobuf 等自定义 codec；同时保留非 []byte Body 的 fallback 处理，防止 panic
- **lib/message.go** - 修复 `m.Body.([]byte)` 直接类型断言导致的 panic 风险，添加 ok 检查 + JSON fallback
- **lib/event.go** - `Emit()` 改进：用 RLock 复制 handlers 列表，再用 Lock 清理 once handlers，减少持写锁时间；移除重复空检查

#### 高优先级修复 (P1)
- **rpc/server.go** - `running` 从 `bool` 改为 `atomic.Bool`，添加 `sync/atomic` import
- **rpc/client.go** - 重构 `poolClient.GetClient()`，消除 `goto` 跳转，改为顺序控制流 + 注释说明
- **broadcast/broadcast.go** - `pushToSession()` 添加 panic recover，防止单条推送崩溃导致整个 worker 静默退出
- **connector/tcp_server.go** - `running` 从 `bool` 改为 `atomic.Bool`
- **scheduler/scheduler.go** - `Push()` 的 `recover()` 增加 `log.Printf` 输出 panic 信息，添加 `log` import
- **connector/tcp_server.go** - dispatchMessages 和 readLoop 日志修复：connID 不在作用域，用 session.ID() 替代

#### 中优先级修复 (P2)
- **pool/pool.go** - `RPCClientPool.Stats()` 返回实际 `totalConns` 而非 `maxConns`
- **forward/forward.go** - `getOrCreateClient()` 添加注释说明双重检查锁定设计
- **lib/ratelimit.go** - `ConnectionLimiter.Acquire()` CAS 失败时添加重试循环（最多3次），降低竞态误判
- **lib/tracing.go** - `NewTraceID()` 低 64 位改用单调计数器替代零值，降低碰撞概率
- **errors/errors.go** - `WithDetail()`/`WithErr()` 改为返回新对象而非修改接收者，防止副作用
- **filter/filter.go** - `FilterFunc.Name()` 返回 `func-filter[<ptr>]` 唯一标识，支持精确 Remove
- **connector/udp_server.go** - readPool `New()` 添加注释说明返回 `&b` 在 sync.Pool 场景下是安全的

#### CLI 模板与启动逻辑修复
- **cmd/gomelo/main.go** - `goModTemplate` 移除硬编码 `D:/workspace/gomelo` replace 指令，改为注释提示用户自行配置
- **cmd/gomelo/main.go** - `adminTemplate` 添加缺失的 `encoding/binary` 和 `io` import
- **cmd/gomelo/main.go** - `connectorRemoteTemplate` args 结构体添加 `json:"userId"` tag
- **cmd/gomelo/main.go** - `cronTemplate` `Cleanup()` 方法移除 `error` 返回值，匹配 Cron 接口规范
- **cmd/gomelo/main.go** - `connectorHandlerTemplate` `Logout` 中 `ResponseOK(nil)` 改为 `ResponseOK(map[string]any{})`
- **cmd/gomelo/main.go** - `filterTemplate` `Name()` 从固定字符串改为 `"%s-filter"` 格式
- **cmd/gomelo/main.go** - `handleInit` 自动生成 `.gitignore` 文件，生成后自动执行 `go mod tidy`
- **cmd/gomelo/main.go** - `handleStart` 增加 `--server-type` 参数支持，增加 `--dev` 模式，默认使用编译产物而非 `go run`
- **cmd/gomelo/main.go** - `handleStart` 修复参数解析 bug：多个非选项参数相互覆盖
- **cmd/gomelo/main.go** - 统一 admin 端口为 `:3006`，消除模板/CLI 间端口不一致
- **cmd/gomelo/main.go** - `gomelo list` 默认端口从 3005 改为 3006（HTTP admin 端口），CLI `serverInfo` 结构体与 Master 返回对齐
- **cmd/gomelo/main.go** - `mainGoTemplate` 的 `startGameServer()` 修复 `s.Frontend()` 永远为 false 的 bug，改为 `app.IsFrontend()`
- **cmd/gomelo/main.go** - `mainGoTemplate` 的 `startGameServer()` 添加 Master 客户端注册逻辑（Register + 心跳），修复子服务器不在 `gomelo list` 中出现的问题
- **cmd/gomelo/main.go** - `mainGoTemplate` 添加 `"time"` import
- **cmd/gomelo/main.go** - `masterMainTemplate` 和 `autoSelectServerID` 添加用途说明注释

## [1.5.4] - 2026-04-28

### 修复

#### 模板问题
- **cmd/gomelo/main.go** - 修复 main.go 模板使用正确的 config.Load 函数
- **cmd/gomelo/main.go** - 添加 StartServers 调用让 Master 自动启动子服务
- **cmd/gomelo/main.go** - 移除 components 目录
- **config/config.go** - 添加 MasterConfig.GetConfig 方法

## [1.5.3] - 2026-04-27

### 新功能

#### Schema 协商机制
- **schema/schema.go** - 新增独立 schema 包，包含 RouteSchema、ServerSchema、SchemaManager
- **lib/app.go** - 新增 RegisterRoute/RegisterJSONRoute/RegisterPBRoute 接口
- **lib/session.go** - 新增 SendSchema/SendRaw 方法，支持直接发送原始数据
- **lib/message.go** - Connection 接口新增 SendRaw 方法
- **connector/*.go** - 连接建立后自动发送 Schema 给客户端

#### 客户端 Schema 处理
- **client/java/GomeloClient.java** - 支持接收和解析 Schema，动态注册路由和 Parser
- **client/js/client.js** - 支持接收和解析 Schema，动态注册路由和 Codec
- **client/unity/GomeloClient.cs** - 支持接收和解析 Schema
- **client/godot/client.gd** - 支持接收和解析 Schema
- **client/godot/network/packet.gd** - 支持 Schema 消息识别
- **client/godot/network/protobuf_codec.gd** - 新增 decode_body 方法

#### RPC 链式调用封装
- **lib/rpc_proxy.go** - 新增 RPCProxy 和 ServiceProxy，支持链式调用风格
- **lib/app.go** - 新增 RPC() 方法返回 RPCProxy
- **ServiceProxy.Call(method, args, reply)** - 负载均衡调用指定 serverType 的随机实例
- **ServiceProxy.ToServer(serverID, method, args, reply)** - 直接调用指定 serverID 的服务器

#### CLI 增强
- **cmd/gomelo/main.go** - 新增 build 命令，编译项目到二进制
- **cmd/gomelo/main.go** - start 命令启动 Master，Master 自动启动所有配置的服务器
- **cmd/gomelo/main.go** - start 命令支持 --production 环境参数
- **cmd/gomelo/main.go** - 修复 go.mod 模板，移除无效的 replace 指令，使用真实版本号
- **cmd/gomelo/main.go** - 修复 import 路径为完整包名 github.com/chuhongliang/gomelo
- **cmd/gomelo/main.go** - 移除 chat/game 服务器类型的模板代码
- **lib/app.go** - 新增 ParseFlags() 方法和命令行 flag 支持
- **lib/app.go** - 新增 GetHost()、GetPort() 方法
- **master/master.go** - 新增 EnableAdmin(addr) 方法，内置监控面板 HTTP 服务
- **master/master.go** - 新增 Wait() 方法

### 修复

#### 高优先级问题 (P1)
- **master/master.go:259-266** - 修复 handleRegister 回调在 unlock 后调用：创建 info 副本再传递
- **master/master.go:375-412** - 修复 checkHeartbeats 持锁期间回调：收集过期 ID 后在锁外执行回调
- **pool/pool.go:106-117** - 新增 Warmup 方法解决初始连接同步问题
- **selector/selector.go:113-127** - 修复 LoadBalancer 未复制 slice 问题
- **config/config.go:122-136** - 新增 Config.Validate() 必填字段验证
- **lib/app.go:602** - 修复 Configure() nil 类型断言
- **plugin/plugin.go:96-133** - 新增 doCall() panic recovery

#### 编译问题修复
- **connector/tcp_server.go** - 修复 atomic.AddUint32 参数类型，使用 unsafe.Pointer 转换
- **connector/udp_server.go** - 删除重复方法声明，恢复 strings 包导入
- **connector/ws_server.go** - 删除未使用的 crypto/tls 和 route 导入，添加 log 和 errors 导入

#### 并发安全与资源管理修复
- **rpc/client.go** - InvokeCtx 添加 readWithContext/readFullWithContext，支持 context 取消
- **connector/tcp_server.go** - 修复 readPool 使用 *[]byte 指针问题，改为直接使用 []byte
- **master/master.go** - processMessages 返回 ([]byte, bool)，缓冲区超限时断开连接
- **lib/app.go** - 使用 sync.Once 确保 flag 只注册一次，修复重复注册问题

## [1.5.1] - 2026-04-27

### 修复

#### 关键并发问题 (P0)
- **rpc/client.go:193-210** - 修复 poolClient.Close() 死锁：使用 goroutine + 超时 channel 替代直接 Wait()
- **connector/udp_server.go:105-126** - 修复 UDP Server 重复 Stop() panic：使用 sync.Once 确保 stopCh 只关闭一次
- **rpc/server.go:164-194** - 修复 RPC context 检查顺序：读操作前检查 context，添加 SetReadDeadline 超时保护 (30s)

#### 高优先级问题 (P1)
- **pool/pool.go:83-95** - 在 pool.cleanupLoop() 添加 panic recovery
- **pool/pool.go:272-285** - 在 RPCClientPool.cleanupLoop() 添加 panic recovery
- **master/master.go:519-537** - 在 watchServers goroutine 添加 panic recovery

#### 代码审查修复的关键 Bug
- **master/master.go:178-216** - 修复 length==0 时无限循环
- **master/master.go:54,255,595** - 移除未清理导致内存泄漏的 serverIDs 切片

#### 游戏服务器架构修复
- **connector/udp_server.go:153-154** - 修复 buffer use-after-return：移除异步 handlePacket goroutine
- **connector/tcp_server.go:226-250** - 修复 TCP readBuf 无界增长：添加 64KB 最大缓冲区限制
- **connector/tcp_server.go:436-461** - 减少心跳检查锁竞争：释放锁后再关闭连接
- **lib/session.go:86-96,213-240** - 消除热路径发送时的锁竞争：closed 改为 atomic.Bool
- **connector/udp_server.go:364-370** - 修复 IPv6 session key bug：直接使用 addr.String()

#### 第三轮审核修复
- **lib/app.go:314,849-913** - 修复 stopWg 从未使用：改用 a.stopWg 进行组件关闭等待
- **connector/tcp_server.go:192-236** - 修复连接双重关闭：移除 handleConn 中的 conn.Close()
- **connector/tcp_server.go:167-175** - 修复 msgWg 从未等待：Stop() 中添加 s.msgWg.Wait()
- **connector/tcp_server.go:238-245** - 修复 readLoop 不检查 stopCh：添加 select 检查 shutdown 信号
- **broadcast/broadcast.go:190-203** - 修复 Add() 创建无效会话：添加警告日志
- **filter/ratelimit.go:102-117** - 修复 cleanupOldBuckets 竞态：使用互斥锁保护
- **rpc/client.go:515-523** - 修复 singleClient 锁模式：统一使用 Lock 替代 RUnlock+Lock 模式

#### 第四轮审核修复
- **master/master.go:730-770** - 修复 reconnectLoop() 竞态：将 connected 检查移入 connMu 锁内

### 优化

#### Pipeline 缓存优化 (P3)
- **lib/router.go:27-97** - 使用 generation 版本号替代全量缓存失效

#### 路由锁优化
- **lib/router.go:61-97** - GetHandlers 缓存命中时使用 RLock，减少读竞争约 80%

## [1.5.0] - 2026-04-25

### 新增

#### 多协议客户端 SDK 支持
所有客户端现在都支持 TCP、UDP 和 WebSocket 三种协议：

- **Go Client** - 新增 `ProtocolType` (tcp/udp/ws)，`ClientOptions` 增加 `Protocol` 字段
- **JS Client** - 新增 `Protocol` 常量，Node.js 环境支持 TCP/UDP
- **Java Client** - 新增 `Protocol` 枚举，新增 `TCPClient.java`、`UDPClient.java`
- **Unity Client** - 新增 `ProtocolType` 枚举，TCP/UDP 连接及读写线程
- **Godot Client** - 新增 `ProtocolType` 枚举，支持 TCP/UDP 处理流程
- **Cocos Client** - 新增 `ProtocolType` 枚举（浏览器环境自动降级为 WebSocket）

#### 配置驱动的自动启动
新增 `AutoSetup()` 和 `AutoConfigure()` 方法：

- **lib/app.go** - 新增 `AutoSetup(configDir)` 自动加载 master.json 和 servers.json
- **lib/app.go** - 新增 `LoadMasterConfig()` 解析 master.json
- **lib/app.go** - 新增 `AutoConfigure()` 自动匹配当前服务器类型
- **lib/app.go** - 新增 `SetHost()`, `SetPort()`, `SetMasterAddr()` 方法

#### 文档更新
- **README.md** - 更新客户端 SDK 示例，展示多协议用法
- **所有客户端 README** - 增加各语言的协议配置示例

### 变更

- **gomelo.go** - 版本号升至 1.5.0
- **Java pom.xml** - 版本号升至 1.5.0
- **cmd/gomelo/main.go** - 更新 main.go 模板使用 AutoSetup/AutoConfigure

## [1.4.0] - 2026-04-24

### 新增

#### 多协议支持
- **UDP 服务器** - 新增 `connector/udp_server.go` 用于 UDP 游戏服务器连接
- **WebSocket 服务器** - 合并到 `connector/ws_server.go`，与 TCP 统一 API
- **UDPConnection** - 新增 `lib.UDPConnection` 类型用于 UDP 会话管理

#### Cron 定时任务
- **scheduler/cron.go** - 完整的 cron 调度支持，Pomelo 风格配置
- **config/crons.json** - 基于环境的 cron 配置文件
- **CronManager** - 多服务器 cron 任务协调
- **CronScheduler.Cancel(id)** - 按 ID 取消任务

#### 代码质量
- **Connector 清理** - 统一 TCP/UDP/WS 的 Forward/Selector 接口
- **移除未使用代码** - 清理 getSession、getIP、GenerateRSAKeys 等

#### 新模块
- **errors/** - 统一错误码体系，标准 HTTP 兼容码（1001-7006 范围）
- **reload/** - 热更新支持，支持文件监控和信号触发（SIGHUP/SIGUSR1）
- **metrics/** - Prometheus 监控指标集成，内置收集器
- **benchmark/** - 性能基准测试套件

#### 客户端 SDK 增强
- **Unity 客户端** - 完全重写，使用原生 WebSocket (System.Net.WebSockets)、心跳、自动重连
- **Unity 客户端** - 修复 seq bug (uint32→uint64)，移除 WebSocketSharp 依赖
- **Java 客户端** - 修复 WebSocketClient 二进制消息处理
- **Java 客户端** - 新增 `ProtobufCodec.java` 支持 Protocol Buffer
- **Java 客户端** - 新增 `CompressionUtil.java` 支持 gzip/zlib 压缩
- **Godot 客户端** - 新增 `protobuf_codec.gd` 和 `compression.gd`
- **Cocos 客户端** - 新增 TypeScript 压缩工具

#### 文档
- **Unity README** - 完整文档，包含 API 参考
- **Godot README** - 完整文档，包含 GDScript 示例
- **Demos** - 为所有 6 个客户端 SDK 添加示例项目

### 修复

#### 客户端 SDK
- **Java WebSocketClient** - 二进制消息处理（移除仅处理 String 的 onMessage）
- **Unity seq bug** - 序列号从 uint32 改为 uint64 以支持 8 字节
- **Unity Packet** - 使用 BitConverter.ToUInt64 替代 ToUInt32

## [1.3.0] - 2026-04-22

### 新增

#### 新模块
- **errors/** - 统一错误码体系，标准 HTTP 兼容码（1001-7006 范围）
- **reload/** - 热更新支持，支持文件监控和信号触发（SIGHUP/SIGUSR1）
- **metrics/** - Prometheus 监控指标集成，内置收集器
- **benchmark/** - 性能基准测试套件

#### 客户端 SDK 增强
- **Unity 客户端** - 完全重写，使用原生 WebSocket (System.Net.WebSockets)、心跳、自动重连
- **Unity 客户端** - 修复 seq bug (uint32→uint64)，移除 WebSocketSharp 依赖
- **Java 客户端** - 修复 WebSocketClient 二进制消息处理
- **Java 客户端** - 新增 `ProtobufCodec.java` 支持 Protocol Buffer
- **Java 客户端** - 新增 `CompressionUtil.java` 支持 gzip/zlib 压缩
- **Godot 客户端** - 新增 `protobuf_codec.gd` 和 `compression.gd`
- **Cocos 客户端** - 新增 TypeScript 压缩工具

#### 文档
- **Unity README** - 完整文档，包含 API 参考
- **Godot README** - 完整文档，包含 GDScript 示例
- **Demos** - 为所有 6 个客户端 SDK 添加示例项目

### 修复

#### 客户端 SDK
- **Java WebSocketClient** - 二进制消息处理（移除仅处理 String 的 onMessage）
- **Unity seq bug** - 序列号从 uint32 改为 uint64 以支持 8 字节
- **Unity Packet** - 使用 BitConverter.ToUInt64 替代 ToUInt32

## [1.2.0] - 2026-04-22

### 新增
- **gomelo routes** - CLI 命令列出所有已注册路由
- **gomelo list** - CLI 命令显示运行中的服务器（跨平台纯 Go HTTP 实现）
- **codegen --list** - 仅列出路由不生成代码
- **ClientOptions.MaxResponseSize** - 可配置的 RPC 响应大小限制
- **Cocos Creator TypeScript 客户端** - Cocos Creator 3.x 原生 TypeScript 客户端
- **Go 客户端** - 纯 Go WebSocket 客户端（无外部依赖）
- **Java 客户端** - Java/Android 客户端，支持 WebSocket
- **Unity C# 客户端** - Unity 游戏完整二进制协议支持
- **Godot GDScript 客户端** - 原生 GDScript 客户端实现
- **JavaScript 客户端** - 更新支持二进制协议
- **Protobuf 类型注册** - 自动类型注册实现 protobuf 编解码
- **真正的 Protobuf 支持** - 使用 `google.golang.org/protobuf` 实现 Protocol Buffers

### 修复

#### 严重并发问题
- **pool.Get()** - 检查与增加 total 非原子操作导致的竞态
- **RPCClientPool.Get()** - 同上
- **pool.Close()** - 持锁调用 Wait() 导致的死锁
- **pool.Put()** - 连接泄漏（静默丢弃而非关闭）
- **RPCClientPool.Put() timer 泄漏** - 高负载下创建大量 timer 导致 GC 压力
- **poolClient.Close()** - 持锁期间调用 Wait() 的死锁风险
- **Master reconnectLoop** - connMu 连接竞态
- **lib/app.go 事件发射** - 在 mutex unlock 后发射事件导致竞态
- **lib/app.go filter setters** - Filter getter/setter 访问 settings 无锁
- **forward/forward.go Stop()** - 清理时并发迭代 map
- **forward/forward.go cleanupLoop** - 无退出信号导致 goroutine 泄漏
- **lib/router.go Pipeline 缓存** - 双重检查锁定模式的 TOCTOU 竞态
- **lib/session.go Send/SendResponse** - 持锁期间执行 I/O
- **connector/checkHeartbeats** - 持锁期间关闭连接导致竞态
- **connector/readLoop** - 缺少 context 检查导致 goroutine 泄漏
- **connector/removeSession** - 可能双重关闭 msgCh
- **rpc/server.go handleConn** - 循环中缺少 context 检查

#### 高优先级
- **master/Heartbeat** - 在验证连接状态前设置 connected 标志
- **master/handleConn** - 静默读取错误无日志
- **master/processMessages** - 畸形输入导致缓冲区无限增长
- **master/callbacks** - 回调处理时复制前的竞态

#### 中低优先级
- **App.Set()** - 移除未使用的 `attach` 参数
- **broadcast/worker** - 添加 worker 退出时待处理任务的日志
- **RateLimiter busy-loop** - 替换为高效的 sync.Cond 信号
- **TokenBucket busy-loop** - 替换为高效的 sync.Cond 信号
- **HealthServer** - 添加单项检查超时（每项3秒，总计10秒）
- **App.afterStart** - 修复事件发射时机

### 变更
- **handleStart** - 现在实际运行服务器而非空实现
- **BuildRoute** - 输出小写路由（pomelo 兼容性）
- **模块路径** - 改为 `github.com/chuhongliang/gomelo`
- **gomelo 二进制名称** - 从 `cli` 改为 `gomelo`
- **Codec** - ProtobufCodec 使用 proto.Marshal 正确序列化
- **Codec** - 类型注册允许基于路由自动反序列化

## [1.1.0] - 2024

### 新增
- 基于 Master 协调的分布式架构
- RPC 客户端连接池
- 服务注册与发现
- 多种负载均衡策略（轮询、一致性哈希、加权随机）
- 广播服务批量消息
- 服务器间消息转发
- 超时控制的优雅关闭
- 配置热更新支持
- 熔断器模式
- 限流
- 指标采集
- 健康检查端点
- Handler/Remote 代码生成

### 组件
- `lib/` - 核心：App, Session, Message, Router, Event, Metrics, Health, Shutdown
- `rpc/` - 带连接池的 RPC 客户端
- `connector/` - 网络连接器
- `master/` - Master 协调服务器
- `registry/` - 服务注册中心
- `selector/` - 负载均衡选择器
- `broadcast/` - 广播服务
- `forward/` - 消息转发
- `pool/` - 连接池
- `loader/` - Handler/Remote 代码加载器
- `codec/` - 消息编解码（JSON/Protobuf）
- `proto/` - Protocol Buffer 消息定义
- `client/` - 客户端 SDK（JS, Godot, Unity）

## [1.0.0] - 初始版本
- 基于 Node.js Pomelo 架构的初始实现