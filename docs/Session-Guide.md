# Session Guide - 会话管理指南

Session 代表一个客户端连接，用于存储玩家状态和管理连接生命周期。

## 获取 Session

在 Handler 中通过 `ctx.Session()` 获取：

```go
func handleEntry(ctx *gomelo.Context) {
    session := ctx.Session()
    // ...
}
```

## 基本操作

### 获取 Session ID

每个 Session 有唯一的 ID：

```go
sessionID := session.ID()
log.Printf("Session ID: %d", sessionID)
```

### 绑定用户 ID

`Bind(uid)` 用于绑定玩家用户 ID：

```go
session.Bind("user-12345")
uid := session.UID() // "user-12345"
```

### 存储数据

使用 `Set(key, value)` 存储任意数据：

```go
session.Set("level", 10)
session.Set("name", "player")
session.Set("vip", true)
session.Set("items", []string{"sword", "shield"})
```

### 获取数据

使用 `Get(key)` 获取数据：

```go
level := session.Get("level")    // 10
name := session.Get("name")       // "player"
vip := session.Get("vip")        // true
items := session.Get("items")   // []string{"sword", "shield"}
```

### 删除数据

```go
session.Remove("vip")
```

## 连接回调

在 connector 上设置连接/断开回调：

```go
conn := connector.NewServer(&connector.ServerOptions{Host: "0.0.0.0", Port: 3010})
conn.OnConnect(func(session *gomelo.Session) {
    log.Printf("Client connected: %d", session.ID())
})
conn.OnClose(func(session *gomelo.Session) {
    log.Printf("Client disconnected: %d", session.ID())
    uid := session.UID()
    if uid != "" {
        playerManager.Remove(uid)
    }
})
app.Register("connector", conn)
```

## Session 生命周期

### 1. 连接建立

客户端连接时创建 Session：

```go
conn.OnConnect(func(session *gomelo.Session) {
    session.Set("connectedAt", time.Now().Unix())
})
```

### 2. 用户认证

通常在 entry handler 中绑定用户 ID：

```go
func handleLogin(ctx *gomelo.Context) {
    var req struct {
        Token string `json:"token"`
    }
    ctx.Bind(&req)

    // 验证 token，获取用户信息
    uid := validateAndGetUID(req.Token)

    session := ctx.Session()
    session.Bind(uid)
    session.Set("token", req.Token)

    ctx.Response(map[string]any{
        "code": 0,
        "msg":  "ok",
        "data": map[string]any{"uid": uid},
    })
}
```

### 3. 玩家离线

Session 关闭时清理数据：

```go
conn.OnClose(func(session *gomelo.Session) {
    uid := session.UID()
    if uid != "" {
        // 保存玩家数据
        savePlayerData(uid, session.KV())
        // 移除在线列表
        onlinePlayers.Remove(uid)
        // 通知其他玩家
        broadcastUserOffline(uid)
    }
})
```

## 踢出玩家

使用 `Close()` 主动关闭 Session：

```go
func handleKick(ctx *gomelo.Context) {
    var req struct {
        TargetUID string `json:"targetUid"`
        Reason    string `json:"reason"`
    }
    ctx.Bind(&req)

    targetSession := sessionManager.FindByUID(req.TargetUID)
    if targetSession != nil {
        targetSession.Close()
    }
}
```

## Session 存储结构

Session 内部使用 `SessionStorage` 存储数据，线程安全：

```go
type SessionStorage struct {
    data map[string]any
    mu   sync.RWMutex
}
```

## 深拷贝

获取 Session 的独立副本：

```go
copy := session.DeepCopy()
// 修改 copy 不影响原始 session
copy.Set("temp", "data")
```

## Session 与连接

Session 提供便捷方法获取底层连接：

```go
conn := session.Connection() // 获取 Connection 接口
remoteAddr := conn.RemoteAddr()
conn.Send(msg) // 直接发送消息
```

## 完整示例

```go
package main

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/chuhongliang/gomelo"
	"github.com/chuhongliang/gomelo/connector"
)

type PlayerManager struct {
	players map[string]*Player
	mu     sync.RWMutex
}

type Player struct {
	UID       string
	Name      string
	Level     int
	LoginTime int64
}

var playerMgr = &PlayerManager{
	players: make(map[string]*Player),
}

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
		session.Set("connectedAt", time.Now().Unix())
	})
	conn.OnClose(func(session *gomelo.Session) {
		uid := session.UID()
		if uid != "" {
			playerMgr.Remove(uid)
			log.Printf("Player left: %s", uid)
		}
	})
	app.Register("connector", conn)

	app.On("connector.login", handleLogin)
	app.On("connector.logout", handleLogout)
	app.On("player.getInfo", handleGetInfo)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
	app.Wait()
}

func handleLogin(ctx *gomelo.Context) {
	var req struct {
		Token string `json:"token"`
		Name  string `json:"name"`
	}
	ctx.Bind(&req)

	uid := "user-" + strconv.FormatUint(ctx.Session().ID(), 10)
	session := ctx.Session()
	session.Bind(uid)
	session.Set("name", req.Name)

	playerMgr.Add(&Player{
		UID:       uid,
		Name:      req.Name,
		Level:     1,
		LoginTime: time.Now().Unix(),
	})

	ctx.Response(map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{
			"uid":   uid,
			"name":  req.Name,
			"level": 1,
		},
	})
}

func handleLogout(ctx *gomelo.Context) {
	session := ctx.Session()
	uid := session.UID()

	if uid != "" {
		playerMgr.Remove(uid)
	}
	session.Close()

	ctx.Response(map[string]any{"code": 0, "msg": "ok"})
}

func handleGetInfo(ctx *gomelo.Context) {
	uid := ctx.Session().UID()
	player := playerMgr.Get(uid)

	if player == nil {
		ctx.Response(map[string]any{"code": 404, "msg": "player not found"})
		return
	}

	ctx.Response(map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{
			"uid":   player.UID,
			"name":  player.Name,
			"level": player.Level,
		},
	})
}

func (pm *PlayerManager) Add(p *Player) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.players[p.UID] = p
}

func (pm *PlayerManager) Get(uid string) *Player {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.players[uid]
}

func (pm *PlayerManager) Remove(uid string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.players, uid)
}
```

## 最佳实践

1. **及时清理** - Session 关闭时清理相关数据
2. **使用 UID 绑定** - 玩家登录后及时 Bind UID
3. **避免存储大对象** - Session 存储应保持轻量
4. **并发安全** - Session 的 Get/Set 是线程安全的
