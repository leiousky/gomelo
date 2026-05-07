# Session Guide

Session represents a client connection, used for storing player state and managing connection lifecycle.

## Get Session

Get via `ctx.Session()` in Handler:

```go
func handleEntry(ctx *gomelo.Context) {
    session := ctx.Session()
    // ...
}
```

## Basic Operations

### Get Session ID

Each Session has a unique ID:

```go
sessionID := session.ID()
log.Printf("Session ID: %d", sessionID)
```

### Bind User ID

`Bind(uid)` binds a player user ID:

```go
session.Bind("user-12345")
uid := session.UID() // "user-12345"
```

### Store Data

Use `Set(key, value)` to store any data:

```go
session.Set("level", 10)
session.Set("name", "player")
session.Set("vip", true)
session.Set("items", []string{"sword", "shield"})
```

### Get Data

Use `Get(key)` to get data:

```go
level := session.Get("level")    // 10
name := session.Get("name")       // "player"
vip := session.Get("vip")        // true
items := session.Get("items")   // []string{"sword", "shield"}
```

### Delete Data

```go
session.Remove("vip")
```

## Connection Callbacks

Set connection/disconnection callbacks on the connector:

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

## Session Lifecycle

### 1. Connection Established

Session created when client connects:

```go
conn.OnConnect(func(session *gomelo.Session) {
    session.Set("connectedAt", time.Now().Unix())
})
```

### 2. User Authentication

Usually bind user ID in entry handler:

```go
func handleLogin(ctx *gomelo.Context) {
    var req struct {
        Token string `json:"token"`
    }
    ctx.Bind(&req)

    // Validate token, get user info
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

### 3. Player Offline

Cleanup data when Session closes:

```go
conn.OnClose(func(session *gomelo.Session) {
    uid := session.UID()
    if uid != "" {
        // Save player data
        savePlayerData(uid, session.KV())
        // Remove from online list
        onlinePlayers.Remove(uid)
        // Notify other players
        broadcastUserOffline(uid)
    }
})
```

## Kick Player

Use `Close()` to actively close Session:

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

## Session Storage Structure

Session uses `SessionStorage` internally, thread-safe:

```go
type SessionStorage struct {
    data map[string]any
    mu   sync.RWMutex
}
```

## Deep Copy

Get an independent copy of Session:

```go
copy := session.DeepCopy()
// Modifying copy doesn't affect original session
copy.Set("temp", "data")
```

## Session and Connection

Session provides convenient methods to access underlying connection:

```go
conn := session.Connection() // Get Connection interface
remoteAddr := conn.RemoteAddr()
conn.Send(msg) // Send message directly
```

## Complete Example

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

## Best Practices

1. **Clean up promptly** - Clean related data when Session closes
2. **Bind UID timely** - Bind UID after player login
3. **Avoid storing large objects** - Session storage should be lightweight
4. **Thread safe** - Session Get/Set is thread-safe
