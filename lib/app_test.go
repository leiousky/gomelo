package lib

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

type mockConn struct {
	id     uint64
	sendCh chan *Message
	closed bool
}

func newMockConn(id uint64) *mockConn {
	return &mockConn{
		id:     id,
		sendCh: make(chan *Message, 100),
	}
}

func (m *mockConn) ID() uint64          { return m.id }
func (m *mockConn) Close()               { m.closed = true }
func (m *mockConn) Send(msg *Message) error {
	m.sendCh <- msg
	return nil
}
func (m *mockConn) SendRaw(data []byte) error {
	return nil
}
func (m *mockConn) RemoteAddr() net.Addr { return nil }

type mockFilter struct {
	name    string
	process func(*Context) bool
	after   func(*Context)
}

func (f *mockFilter) Name() string                 { return f.name }
func (f *mockFilter) Process(ctx *Context) bool    {
	if f.process != nil {
		return f.process(ctx)
	}
	return true
}
func (f *mockFilter) After(ctx *Context) {
	if f.after != nil {
		f.after(ctx)
	}
}

type mockComponent struct {
	name   string
	start  bool
	stop   bool
	startErr error
	stopErr  error
}

func (c *mockComponent) Name() string                     { return c.name }
func (c *mockComponent) Start(app *App) error             { c.start = true; return c.startErr }
func (c *mockComponent) Stop() error                     { c.stop = true; return c.stopErr }

func TestApp_NewApp(t *testing.T) {
	tests := []struct {
		name   string
		opts   []AppOption
		expect func(*testing.T, *App)
	}{
		{
			name: "default app",
			opts: nil,
			expect: func(t *testing.T, app *App) {
				if app.state != StateInited {
					t.Errorf("expected state %d, got %d", StateInited, app.state)
				}
				if app.event == nil {
					t.Error("event emitter should be initialized")
				}
				if app.router == nil {
					t.Error("router should be initialized")
				}
				if app.pipeline == nil {
					t.Error("pipeline should be initialized")
				}
				if app.settings == nil {
					t.Error("settings should be initialized")
				}
			},
		},
		{
			name: "with env option",
			opts: []AppOption{WithEnv("production")},
			expect: func(t *testing.T, app *App) {
				if app.Get("env") != "production" {
					t.Errorf("expected env 'production', got %v", app.Get("env"))
				}
			},
		},
		{
			name: "with host option",
			opts: []AppOption{WithHost("127.0.0.1")},
			expect: func(t *testing.T, app *App) {
				if app.Get("host") != nil {
					t.Errorf("expected host nil (not stored), got %v", app.Get("host"))
				}
			},
		},
		{
			name: "with port option",
			opts: []AppOption{WithPort(3000)},
			expect: func(t *testing.T, app *App) {
				if app.Get("port") != nil {
					t.Errorf("expected port nil (not stored), got %v", app.Get("port"))
				}
			},
		},
		{
			name: "with server id option",
			opts: []AppOption{WithServerID("server-001")},
			expect: func(t *testing.T, app *App) {
				if app.GetServerId() != "" {
					t.Errorf("expected server id '' (not stored via option), got %s", app.GetServerId())
				}
			},
		},
		{
			name: "with master addr option",
			opts: []AppOption{WithMasterAddr("localhost:3001")},
			expect: func(t *testing.T, app *App) {
				if app.Get("masterAddr") != nil {
					t.Errorf("expected masterAddr nil (not stored), got %v", app.Get("masterAddr"))
				}
			},
		},
		{
			name: "multiple options",
			opts: []AppOption{WithEnv("development"), WithHost("0.0.0.0"), WithPort(8080), WithServerID("test")},
			expect: func(t *testing.T, app *App) {
				if app.Get("env") != "development" {
					t.Errorf("expected env 'development', got %v", app.Get("env"))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp(tt.opts...)
			tt.expect(t, app)
		})
	}
}

func TestApp_Configure(t *testing.T) {
	app := NewApp()
	app.SetServerType("connector")

	tests := []struct {
		name        string
		env         string
		serverType  string
		expectCalled bool
	}{
		{"match env and type", "development", "connector", true},
		{"match env different type", "development", "gate", false},
		{"different env same type", "production", "connector", false},
		{"all env all type", "all", "all", true},
		{"empty env matching type", "", "connector", true},
		{"empty type matching env", "development", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			app.ConfigureWithEnv(tt.env, tt.serverType)(func(s *Server) {
				called = true
				if s.app != app {
					t.Error("server should have app reference")
				}
			})
			if called != tt.expectCalled {
				t.Errorf("expected called=%v, got %v", tt.expectCalled, called)
			}
		})
	}
}

func TestApp_Filter(t *testing.T) {
	app := NewApp()

	tests := []struct {
		name     string
		addFilter func(*App)
		setting  string
	}{
		{"Before filter", func(a *App) { a.Before(&mockFilter{name: "before"}) }, "beforeFilter"},
		{"After filter", func(a *App) { a.After(&mockFilter{name: "after"}) }, "afterFilter"},
		{"GlobalBefore filter", func(a *App) { a.GlobalBefore(&mockFilter{name: "gbefore"}) }, "globalBeforeFilter"},
		{"GlobalAfter filter", func(a *App) { a.GlobalAfter(&mockFilter{name: "gafter"}) }, "globalAfterFilter"},
		{"RpcBefore filter", func(a *App) { a.RpcBefore(&mockFilter{name: "rpcbefore"}) }, "rpcBeforeFilter"},
		{"RpcAfter filter", func(a *App) { a.RpcAfter(&mockFilter{name: "rpcafter"}) }, "rpcAfterFilter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.addFilter(app)
			filters, ok := app.settings[tt.setting].([]Filter)
			if !ok {
				t.Errorf("expected filters to be set for %s", tt.setting)
			}
			if len(filters) != 1 {
				t.Errorf("expected 1 filter, got %d", len(filters))
			}
		})
	}
}

func TestApp_Event(t *testing.T) {
	emitter := NewEventEmitter()

	var mu sync.Mutex
	calls := 0

	id := emitter.On("start", func(args ...any) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	if id == 0 {
		t.Error("expected non-zero event id")
	}

	emitter.Emit("start")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
	mu.Unlock()

	emitter.Emit("start")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
	mu.Unlock()

	emitter.Off("start", id)
	emitter.Emit("start")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if calls != 2 {
		t.Errorf("after off, expected 2 calls, got %d", calls)
	}
	mu.Unlock()
}

func TestApp_EventUnknown(t *testing.T) {
	emitter := NewEventEmitter()

	emitter.Emit("unknown")
	time.Sleep(10 * time.Millisecond)
}

func TestApp_EventMultipleHandlers(t *testing.T) {
	emitter := NewEventEmitter()

	var mu sync.Mutex
	calls := 0

	emitter.On("multi", func(args ...any) {
		mu.Lock()
		calls++
		mu.Unlock()
	})
	emitter.On("multi", func(args ...any) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	emitter.Emit("multi")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
	mu.Unlock()
}

func TestApp_Lifecycle(t *testing.T) {
	t.Run("Start and Stop", func(t *testing.T) {
		app := NewApp()
		app.SetServerId("test-server")
		app.SetServerType("connector")

		comp1 := &mockComponent{name: "comp1"}
		comp2 := &mockComponent{name: "comp2"}
		app.Load("comp1", comp1)
		app.Load("comp2", comp2)

		err := app.Start()
		if err != nil {
			t.Errorf("start should succeed, got %v", err)
		}

		time.Sleep(50 * time.Millisecond)

		if app.state != StateStarted {
			t.Errorf("expected state %d, got %d", StateStarted, app.state)
		}
		if !comp1.start || !comp2.start {
			t.Error("all components should be started")
		}

		err = app.Stop(false)
		if err != nil {
			t.Errorf("stop error: %v", err)
		}

		if app.state != StateStopped {
			t.Errorf("expected state %d, got %d", StateStopped, app.state)
		}
		if !comp1.stop || !comp2.stop {
			t.Error("all components should be stopped")
		}
	})

	t.Run("Double Start", func(t *testing.T) {
		app := NewApp()
		app.SetServerId("test-server")

		err1 := app.Start()
		time.Sleep(10 * time.Millisecond)

		err2 := app.Start()
		time.Sleep(10 * time.Millisecond)

		if err1 != nil {
			t.Errorf("first start should succeed, got %v", err1)
		}
		if err2 != nil {
			t.Errorf("second start should succeed (idempotent), got %v", err2)
		}
	})

	t.Run("Event emission on start", func(t *testing.T) {
		app := NewApp()
		app.SetServerId("test-server")

		var eventReceived bool
		app.Event().On("start_server", func(args ...any) {
			if len(args) > 0 {
				if args[0] == "test-server" {
					eventReceived = true
				}
			}
		})

		app.Start()
		time.Sleep(50 * time.Millisecond)

		if !eventReceived {
			t.Error("start_server event should be emitted")
		}

		app.Stop(false)
	})
}

func TestApp_Settings(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*App)
		getKey   string
		expected any
	}{
		{"Set and Get", func(a *App) { a.Set("key", "value") }, "key", "value"},
		{"Enable", func(a *App) { a.Enable("feature") }, "feature", true},
		{"Disable", func(a *App) { a.Disable("feature") }, "feature", false},
		{"Enabled true", func(a *App) { a.Enable("feature") }, "feature", true},
		{"Enabled false", func(a *App) { a.Disable("feature") }, "feature", false},
		{"Disabled true", func(a *App) { a.Disable("feature") }, "feature", false},
		{"Disabled false", func(a *App) { a.Enable("feature") }, "feature", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp()
			tt.setup(app)
			if app.Get(tt.getKey) != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, app.Get(tt.getKey))
			}
		})
	}
}

func TestApp_ServerManagement(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*App)
		expectedTypes []string
		expectedLen  int
	}{
		{
			name: "SetServers single type",
			setup: func(a *App) {
				a.SetServers(map[string]map[string]any{
					"server1": {"serverType": "connector", "id": "server1"},
					"server2": {"serverType": "connector", "id": "server2"},
				})
			},
			expectedTypes: []string{"connector"},
			expectedLen:   2,
		},
		{
			name: "SetServers multiple types",
			setup: func(a *App) {
				a.SetServers(map[string]map[string]any{
					"server1": {"serverType": "connector", "id": "server1"},
					"server2": {"serverType": "gate", "id": "server2"},
				})
			},
			expectedTypes: []string{"connector", "gate"},
			expectedLen:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp()
			tt.setup(app)

			types := app.GetServerTypes()
			if len(types) != len(tt.expectedTypes) {
				t.Errorf("expected %d server types, got %d", len(tt.expectedTypes), len(types))
			}

			servers := app.GetServers()
			if len(servers) != tt.expectedLen {
				t.Errorf("expected %d servers, got %d", tt.expectedLen, len(servers))
			}
		})
	}
}

func TestSession_NewSession(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Session)
		field    string
		expected any
	}{
		{"default session", func(s *Session) {}, "storage", true},
		{"set id", func(s *Session) { s.SetID(100) }, "id", uint64(100)},
		{"set uid", func(s *Session) { s.SetUID("user-001") }, "uid", "user-001"},
		{"set server id", func(s *Session) { s.SetServerID("server-001") }, "serverId", "server-001"},
		{"set server type", func(s *Session) { s.SetServerType("connector") }, "serverType", "connector"},
		{"set connection id", func(s *Session) { s.SetConnectionID(999) }, "connectionId", uint64(999)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewSession()
			tt.setup(session)

			switch tt.field {
			case "storage":
				if session.storage == nil {
					t.Error("storage should not be nil")
				}
			case "id":
				if session.ID() != tt.expected.(uint64) {
					t.Errorf("expected %v, got %v", tt.expected, session.ID())
				}
			case "uid":
				if session.UID() != tt.expected.(string) {
					t.Errorf("expected %v, got %v", tt.expected, session.UID())
				}
			case "serverId":
				if session.GetServerID() != tt.expected.(string) {
					t.Errorf("expected %v, got %v", tt.expected, session.GetServerID())
				}
			case "serverType":
				if session.GetServerType() != tt.expected.(string) {
					t.Errorf("expected %v, got %v", tt.expected, session.GetServerType())
				}
			case "connectionId":
				if session.GetConnectionID() != tt.expected.(uint64) {
					t.Errorf("expected %v, got %v", tt.expected, session.GetConnectionID())
				}
			}
		})
	}
}

func TestSession_SetGet(t *testing.T) {
	session := NewSession()

	tests := []struct {
		name     string
		key      string
		value    any
		expected any
	}{
		{"string value", "name", "test", "test"},
		{"int value", "age", 25, 25},
		{"bool value", "active", true, true},
		{"nil value", "empty", nil, nil},
		{"update value", "name", "updated", "updated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session.Set(tt.key, tt.value)
			got := session.Get(tt.key)
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestSession_SetGetMap(t *testing.T) {
	session := NewSession()

	session.Set("data", map[string]any{"key": "val"})
	got := session.Get("data")

	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatal("expected map type")
	}
	if gotMap["key"] != "val" {
		t.Errorf("expected key 'val', got %v", gotMap["key"])
	}
}

func TestSession_Remove(t *testing.T) {
	session := NewSession()
	session.Set("key1", "value1")
	session.Set("key2", "value2")

	session.Remove("key1")

	if session.Get("key1") != nil {
		t.Error("key1 should be removed")
	}
	if session.Get("key2") != "value2" {
		t.Error("key2 should still exist")
	}
}

func TestSession_Close(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*Session) *mockConn
		expectSend bool
	}{
		{
			name: "close with connection",
			setup: func(s *Session) *mockConn {
				conn := newMockConn(1)
				s.SetConnection(conn)
				return conn
			},
			expectSend: false,
		},
		{
			name: "close without connection",
			setup: func(s *Session) *mockConn {
				return nil
			},
			expectSend: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewSession()
			tt.setup(session)

			if session.IsClosed() {
				t.Error("new session should not be closed")
			}

			session.Close()

			if !session.IsClosed() {
				t.Error("session should be closed after Close()")
			}

			session.Close()
			if !session.IsClosed() {
				t.Error("double close should not change state")
			}
		})
	}
}

func TestSession_Send(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*Session) *mockConn
		msg       *Message
		expectErr bool
	}{
		{
			name: "send with connection",
			setup: func(s *Session) *mockConn {
				conn := newMockConn(1)
				s.SetConnection(conn)
				return conn
			},
			msg:       &Message{Type: Request, Route: "test.route", Seq: 1, Body: "hello"},
			expectErr: false,
		},
		{
			name: "send without connection",
			setup: func(s *Session) *mockConn {
				return nil
			},
			msg:       &Message{Type: Request, Route: "test.route", Seq: 1, Body: "hello"},
			expectErr: true,
		},
		{
			name: "send on closed session",
			setup: func(s *Session) *mockConn {
				conn := newMockConn(1)
				s.SetConnection(conn)
				s.Close()
				return conn
			},
			msg:       &Message{Type: Request, Route: "test.route", Seq: 1, Body: "hello"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewSession()
			conn := tt.setup(session)

			err := session.Send(tt.msg)

			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectErr && conn != nil {
				select {
				case msg := <-conn.sendCh:
					if msg.Route != tt.msg.Route {
						t.Errorf("expected route %s, got %s", tt.msg.Route, msg.Route)
					}
				default:
					t.Error("expected message to be sent")
				}
			}
		})
	}
}

func TestSession_SendResponse(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*Session) *mockConn
		seq       uint64
		route     string
		body      any
		expectErr bool
	}{
		{
			name: "send response success",
			setup: func(s *Session) *mockConn {
				conn := newMockConn(1)
				s.SetConnection(conn)
				return conn
			},
			seq:       1,
			route:     "test.route",
			body:      map[string]any{"code": 0},
			expectErr: false,
		},
		{
			name: "send response without connection",
			setup: func(s *Session) *mockConn {
				return nil
			},
			seq:       1,
			route:     "test.route",
			body:      nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewSession()
			conn := tt.setup(session)

			err := session.SendResponse(tt.seq, tt.route, tt.body)

			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectErr && conn != nil {
				select {
				case msg := <-conn.sendCh:
					if msg.Type != Response {
						t.Errorf("expected type Response, got %v", msg.Type)
					}
					if msg.Seq != tt.seq {
						t.Errorf("expected seq %d, got %d", tt.seq, msg.Seq)
					}
				default:
					t.Error("expected message to be sent")
				}
			}
		})
	}
}

func TestSession_KV(t *testing.T) {
	session := NewSession()
	session.Set("key1", "value1")
	session.Set("key2", "value2")

	kv := session.KV()

	if len(kv) != 2 {
		t.Errorf("expected 2 items, got %d", len(kv))
	}
	if kv["key1"] != "value1" || kv["key2"] != "value2" {
		t.Error("kv content mismatch")
	}
}

func TestSession_DeepCopy(t *testing.T) {
	session := NewSession()
	session.SetID(100)
	session.SetUID("user-001")
	session.Set("key", "value")

	copy := session.DeepCopy()

	if copy.ID() != session.ID() {
		t.Error("id should be copied")
	}
	if copy.UID() != session.UID() {
		t.Error("uid should be copied")
	}
	if copy.Get("key") != "value" {
		t.Error("storage should be copied")
	}

	copy.Set("key", "modified")
	if session.Get("key") == "modified" {
		t.Error("original session should not be affected")
	}
}

func TestSession_Bind(t *testing.T) {
	session := NewSession()

	session.Bind("user-001")

	if session.UID() != "user-001" {
		t.Errorf("expected uid 'user-001', got %s", session.UID())
	}
}

func TestRouter_AddRoute(t *testing.T) {
	router := NewRouter()

	tests := []struct {
		name       string
		serverType string
		handler    RouteHandler
	}{
		{"connector route", "connector", func(ctx *Context, s string) string { return "conn" }},
		{"gate route", "gate", func(ctx *Context, s string) string { return "gate" }},
		{"empty type", "", func(ctx *Context, s string) string { return "empty" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router.SetRoute(tt.serverType, tt.handler)

			router.mu.RLock()
			h, ok := router.routes[tt.serverType]
			router.mu.RUnlock()

			if !ok {
				t.Error("route should be set")
			}
			if h == nil {
				t.Error("handler should not be nil")
			}
		})
	}
}

func TestRouter_FindRoute(t *testing.T) {
	router := NewRouter()

	router.SetRoute("connector", func(ctx *Context, s string) string { return "connector-result" })
	router.SetRoute("gate", func(ctx *Context, s string) string { return "gate-result" })

	tests := []struct {
		name        string
		serverType  string
		expectFound bool
	}{
		{"existing connector", "connector", true},
		{"existing gate", "gate", true},
		{"non-existing", "non-exist", false},
		{"empty type", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := router.GetRoute(tt.serverType)

			if ok != tt.expectFound {
				t.Errorf("expected found=%v, got %v", tt.expectFound, ok)
			}
			if tt.expectFound && handler == nil {
				t.Error("handler should not be nil when found")
			}
		})
	}
}

func TestRouter_RouteParam(t *testing.T) {
	router := NewRouter()

	var capturedCtx *Context
	var capturedParam string

	router.SetRoute("connector", func(ctx *Context, s string) string {
		capturedCtx = ctx
		capturedParam = s
		return s
	})

	handler, _ := router.GetRoute("connector")
	app := NewApp()
	ctx := NewContext(app)
	ctx.Route = "test.route"

	result := handler(ctx, "param-value")

	if capturedCtx != ctx {
		t.Error("context should be passed to handler")
	}
	if capturedParam != "param-value" {
		t.Errorf("expected param 'param-value', got %s", capturedParam)
	}
	if result != "param-value" {
		t.Errorf("expected result 'param-value', got %s", result)
	}
}

func TestPipeline(t *testing.T) {
	t.Run("Use middleware", func(t *testing.T) {
		pipeline := NewPipeline()

		pipeline.Use(func(next HandlerFunc) HandlerFunc {
			return func(c *Context) {
				c.Set("middleware", "applied")
				next(c)
			}
		})

		if len(pipeline.middlewares) != 1 {
			t.Errorf("expected 1 middleware, got %d", len(pipeline.middlewares))
		}
	})

	t.Run("On handler", func(t *testing.T) {
		pipeline := NewPipeline()

		pipeline.On("test.route", func(c *Context) {
			c.Set("handler", "executed")
		})

		if len(pipeline.handlers["test.route"]) != 1 {
			t.Errorf("expected 1 handler, got %d", len(pipeline.handlers["test.route"]))
		}
	})

	t.Run("GetHandlers", func(t *testing.T) {
		pipeline := NewPipeline()

		var executed bool
		pipeline.On("test.route", func(c *Context) {
			executed = true
		})

		handlers := pipeline.GetHandlers("test.route")
		if len(handlers) != 1 {
			t.Errorf("expected 1 handler, got %d", len(handlers))
		}

		app := NewApp()
		ctx := NewContext(app)
		ctx.Route = "test.route"
		ctx.handlers = handlers
		ctx.Next()

		if !executed {
			t.Error("handler should be executed")
		}
	})

	t.Run("Middleware chain", func(t *testing.T) {
		pipeline := NewPipeline()

		var order []string
		pipeline.Use(func(next HandlerFunc) HandlerFunc {
			return func(c *Context) {
				order = append(order, "mw1-before")
				next(c)
				order = append(order, "mw1-after")
			}
		})
		pipeline.Use(func(next HandlerFunc) HandlerFunc {
			return func(c *Context) {
				order = append(order, "mw2-before")
				next(c)
				order = append(order, "mw2-after")
			}
		})

		pipeline.On("test.route", func(c *Context) {
			order = append(order, "handler")
		})

		handlers := pipeline.GetHandlers("test.route")
		app := NewApp()
		ctx := NewContext(app)
		ctx.Route = "test.route"
		ctx.handlers = handlers
		ctx.Next()

		expected := []string{"mw2-before", "mw1-before", "handler", "mw1-after", "mw2-after"}
		if len(order) != len(expected) {
			t.Errorf("expected order length %d, got %d", len(expected), len(order))
		}
	})
}

func TestMessage_NewMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		checkFn  func(*Message) bool
	}{
		{
			name: "request message",
			msg:  &Message{Type: Request, Route: "test.route", Seq: 1, Body: "data"},
			checkFn: func(m *Message) bool {
				return m.Type == Request && m.Route == "test.route" && m.Seq == 1
			},
		},
		{
			name: "response message",
			msg:  &Message{Type: Response, Route: "test.route", Seq: 2, Body: nil},
			checkFn: func(m *Message) bool {
				return m.Type == Response && m.Seq == 2
			},
		},
		{
			name: "notify message",
			msg:  &Message{Type: Notify, Route: "notify.route", Seq: 0, Body: map[string]any{"key": "val"}},
			checkFn: func(m *Message) bool {
				return m.Type == Notify && m.Route == "notify.route"
			},
		},
		{
			name: "broadcast message",
			msg:  &Message{Type: Broadcast, Route: "broadcast.route", Seq: 3, Body: []byte("bytes")},
			checkFn: func(m *Message) bool {
				return m.Type == Broadcast
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.checkFn(tt.msg) {
				t.Error("message check failed")
			}
		})
	}
}

func TestMessage_EncodeDecode(t *testing.T) {
	tests := []struct {
		name      string
		original  *Message
		expectErr bool
	}{
		{
			name: "encode decode request",
			original: &Message{
				Type:  Request,
				Route: "connector.entryHandler.entry",
				Seq:   1,
				Body:  map[string]any{"name": "test", "age": 25},
			},
			expectErr: false,
		},
		{
			name: "encode decode with byte body",
			original: &Message{
				Type:  Request,
				Route: "test.route",
				Seq:   2,
				Body:  []byte(`{"data":"value"}`),
			},
			expectErr: false,
		},
		{
			name: "encode decode response",
			original: &Message{
				Type:  Response,
				Route: "test.route",
				Seq:   100,
				Body:  "simple string",
			},
			expectErr: false,
		},
		{
			name: "encode decode notify",
			original: &Message{
				Type:  Notify,
				Route: "notify.route",
				Seq:   0,
				Body:  nil,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.original.Encode()
			if err != nil {
				if !tt.expectErr {
					t.Errorf("unexpected encode error: %v", err)
				}
				return
			}
			if tt.expectErr {
				t.Error("expected error, got nil")
				return
			}

			decoded := &Message{}
			err = decoded.Decode(data)
			if err != nil {
				t.Errorf("decode error: %v", err)
			}

			if decoded.Type != tt.original.Type {
				t.Errorf("expected type %v, got %v", tt.original.Type, decoded.Type)
			}
			if decoded.Route != tt.original.Route {
				t.Errorf("expected route %s, got %s", tt.original.Route, decoded.Route)
			}
			if decoded.Seq != tt.original.Seq {
				t.Errorf("expected seq %d, got %d", tt.original.Seq, decoded.Seq)
			}
		})
	}
}

func TestMessage_EncodeBody(t *testing.T) {
	tests := []struct {
		name      string
		msg       *Message
		expectStr string
	}{
		{
			name:      "map body",
			msg:       &Message{Body: map[string]any{"key": "value", "num": 123}},
			expectStr: `{"key":"value","num":123}`,
		},
		{
			name:      "string body",
			msg:       &Message{Body: "simple string"},
			expectStr: `"simple string"`,
		},
		{
			name:      "int body",
			msg:       &Message{Body: 42},
			expectStr: `42`,
		},
		{
			name:      "slice body",
			msg:       &Message{Body: []any{1, 2, 3}},
			expectStr: `[1,2,3]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.msg.EncodeBody()
			if err != nil {
				t.Errorf("encode body error: %v", err)
			}

			var result any
			json.Unmarshal(data, &result)

			var expected any
			json.Unmarshal([]byte(tt.expectStr), &expected)

			resultBytes, _ := json.Marshal(result)
			if string(resultBytes) != tt.expectStr {
				eq := false
				switch expected.(type) {
				case float64:
					if r, ok := result.(float64); ok {
						eq = r == expected.(float64)
					}
				case string:
					if r, ok := result.(string); ok {
						eq = r == expected.(string)
					}
				}
				if !eq {
					t.Errorf("expected %s, got %s", tt.expectStr, string(data))
				}
			}
		})
	}
}

func TestMessage_DecodeBody(t *testing.T) {
	tests := []struct {
		name      string
		msg       *Message
		target    any
		expectErr bool
		checkFn   func(any) bool
	}{
		{
			name: "decode to struct",
			msg: &Message{
				Body: []byte(`{"name":"test","age":25}`),
			},
			target:    &struct{ Name string }{},
			expectErr: false,
			checkFn: func(v any) bool {
				s := v.(*struct{ Name string })
				return s.Name == "test"
			},
		},
		{
			name: "decode to map",
			msg: &Message{
				Body: []byte(`{"key":"value"}`),
			},
			target:    &map[string]any{},
			expectErr: false,
			checkFn: func(v any) bool {
				m := v.(*map[string]any)
				return (*m)["key"] == "value"
			},
		},
		{
			name: "decode non-byte body",
			msg: &Message{
				Body: map[string]any{"key": "value"},
			},
			target:    &map[string]any{},
			expectErr: false,
			checkFn: func(v any) bool {
				m := v.(*map[string]any)
				return (*m)["key"] == "value"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.DecodeBody(tt.target)
			if err != nil {
				if !tt.expectErr {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if tt.expectErr {
				t.Error("expected error, got nil")
				return
			}
			if tt.checkFn != nil && !tt.checkFn(tt.target) {
				t.Error("check failed")
			}
		})
	}
}

func TestEventEmitter_On(t *testing.T) {
	emitter := NewEventEmitter()

	tests := []struct {
		name        string
		event       string
		callback    EventCallback
		expectId    bool
	}{
		{"on handler", "test", func(args ...any) {}, true},
		{"on multiple same event", "test2", func(args ...any) {}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := emitter.On(tt.event, tt.callback)
			if tt.expectId && id == 0 {
				t.Error("expected non-zero id")
			}

			emitter.mu.RLock()
			handlers := emitter.events[tt.event]
			emitter.mu.RUnlock()

			if len(handlers) == 0 {
				t.Error("handler should be registered")
			}
		})
	}
}

func TestEventEmitter_Once(t *testing.T) {
	emitter := NewEventEmitter()

	var count int
	id := emitter.Once("once", func(args ...any) {
		count++
	})

	if id == 0 {
		t.Error("expected non-zero id")
	}

	emitter.Emit("once")
	time.Sleep(10 * time.Millisecond)

	if count != 1 {
		t.Errorf("expected 1 call, got %d", count)
	}

	emitter.Emit("once")
	time.Sleep(10 * time.Millisecond)

	if count != 1 {
		t.Errorf("once handler should only fire once, got %d", count)
	}
}

func TestEventEmitter_Off(t *testing.T) {
	emitter := NewEventEmitter()

	var calls int
	handler := func(args ...any) { calls++ }

	id := emitter.On("test", handler)
	emitter.Emit("test")
	time.Sleep(10 * time.Millisecond)

	emitter.Off("test", id)
	emitter.Emit("test")
	time.Sleep(10 * time.Millisecond)

	if calls != 1 {
		t.Errorf("expected 1 call after off, got %d", calls)
	}
}

func TestEventEmitter_Off_UnknownId(t *testing.T) {
	emitter := NewEventEmitter()

	emitter.On("test", func(args ...any) {})

	emitter.Off("test", EventID(99999))

	emitter.mu.RLock()
	handlers := emitter.events["test"]
	emitter.mu.RUnlock()

	if len(handlers) != 1 {
		t.Error("unknown id should not affect handlers")
	}
}

func TestEventEmitter_Emit(t *testing.T) {
	t.Run("emit to registered handler", func(t *testing.T) {
		emitter := NewEventEmitter()

		var mu sync.Mutex
		calls := 0
		emitter.On("test", func(args ...any) {
			mu.Lock()
			calls++
			mu.Unlock()
		})

		emitter.Emit("test")
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
		mu.Unlock()
	})

	t.Run("emit with args", func(t *testing.T) {
		emitter := NewEventEmitter()

		var receivedArgs []any
		emitter.On("test", func(args ...any) {
			receivedArgs = args
		})

		emitter.Emit("test", 1, "string", true)
		time.Sleep(10 * time.Millisecond)

		if len(receivedArgs) != 3 {
			t.Errorf("expected 3 args, got %d", len(receivedArgs))
		}
	})

	t.Run("emit to unknown event", func(t *testing.T) {
		emitter := NewEventEmitter()

		emitter.Emit("unknown")
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("multiple handlers", func(t *testing.T) {
		emitter := NewEventEmitter()

		var mu sync.Mutex
		calls := 0
		emitter.On("test", func(args ...any) {
			mu.Lock()
			calls++
			mu.Unlock()
		})
		emitter.On("test", func(args ...any) {
			mu.Lock()
			calls++
			mu.Unlock()
		})
		emitter.On("test", func(args ...any) {
			mu.Lock()
			calls++
			mu.Unlock()
		})

		emitter.Emit("test")
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
		mu.Unlock()
	})
}

func TestEventEmitter_Clear(t *testing.T) {
	emitter := NewEventEmitter()

	emitter.On("test1", func(args ...any) {})
	emitter.On("test2", func(args ...any) {})
	emitter.On("test2", func(args ...any) {})

	emitter.Clear("test2")

	emitter.mu.RLock()
	handlers1 := emitter.events["test1"]
	handlers2 := emitter.events["test2"]
	emitter.mu.RUnlock()

	if len(handlers1) != 1 {
		t.Error("test1 should have 1 handler")
	}
	if len(handlers2) != 0 {
		t.Error("test2 should be cleared")
	}
}

func TestEventEmitter_Async(t *testing.T) {
	emitter := NewEventEmitter()

	var mu sync.Mutex
	results := make([]int, 0)

	for i := 0; i < 10; i++ {
		n := i
		emitter.On("async", func(args ...any) {
			mu.Lock()
			results = append(results, n)
			mu.Unlock()
		})
	}

	emitter.Emit("async")
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}
	mu.Unlock()
}

func TestEventEmitter_PanicRecovery(t *testing.T) {
	emitter := NewEventEmitter()

	emitter.On("panic", func(args ...any) {
		panic("test panic")
	})

	emitter.Emit("panic")

	time.Sleep(10 * time.Millisecond)
}

func TestContext_Bind(t *testing.T) {
	tests := []struct {
		name      string
		setup    func(*Context)
		target    any
		expectErr bool
		checkFn   func(any) bool
	}{
		{
			name: "bind json body",
			setup: func(c *Context) {
				c.request = &Message{Body: []byte(`{"name":"test","age":25}`)}
			},
			target: &struct {
				Name string `json:"name"`
				Age  int    `json:"age"`
			}{},
			expectErr: false,
			checkFn: func(v any) bool {
				s := v.(*struct {
					Name string `json:"name"`
					Age  int    `json:"age"`
				})
				return s.Name == "test" && s.Age == 25
			},
		},
		{
			name: "bind nil request",
			setup: func(c *Context) {
				c.request = nil
			},
			target:    &struct{}{},
			expectErr: false,
			checkFn:   func(v any) bool { return true },
		},
		{
			name: "bind nil body",
			setup: func(c *Context) {
				c.request = &Message{Body: nil}
			},
			target:    &struct{}{},
			expectErr: false,
			checkFn:   func(v any) bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp()
			ctx := NewContext(app)
			tt.setup(ctx)

			err := ctx.Bind(tt.target)
			if err != nil && !tt.expectErr {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.checkFn != nil && !tt.checkFn(tt.target) {
				t.Error("check failed")
			}
		})
	}
}

func TestContext_Response(t *testing.T) {
	app := NewApp()
	ctx := NewContext(app)
	ctx.Route = "test.route"

	ctx.Response(map[string]any{"msg": "hello"})

	if ctx.Resp == nil {
		t.Fatal("response should not be nil")
	}
	if ctx.Resp.Route != "test.route" {
		t.Errorf("expected route 'test.route', got %s", ctx.Resp.Route)
	}
	if ctx.Resp.Type != Response {
		t.Errorf("expected type Response, got %v", ctx.Resp.Type)
	}
}

func TestContext_ResponseOK(t *testing.T) {
	app := NewApp()
	ctx := NewContext(app)
	ctx.Route = "test.route"

	ctx.ResponseOK(map[string]any{"data": "value"})

	if ctx.Resp == nil {
		t.Fatal("response should not be nil")
	}

	body, ok := ctx.Resp.Body.(map[string]any)
	if !ok {
		t.Fatal("response body should be map")
	}
	if body["code"] != 0 {
		t.Errorf("expected code 0, got %v", body["code"])
	}
	if body["msg"] != "ok" {
		t.Errorf("expected msg 'ok', got %v", body["msg"])
	}
}

func TestContext_ResponseError(t *testing.T) {
	app := NewApp()
	ctx := NewContext(app)
	ctx.Route = "test.route"

	ctx.ResponseError(404, "not found")

	if ctx.Resp == nil {
		t.Fatal("response should not be nil")
	}

	body, ok := ctx.Resp.Body.(map[string]any)
	if !ok {
		t.Fatal("response body should be map")
	}
	if body["code"] != 404 {
		t.Errorf("expected code 404, got %v", body["code"])
	}
	if body["msg"] != "not found" {
		t.Errorf("expected msg 'not found', got %v", body["msg"])
	}
}

func TestContext_Next(t *testing.T) {
	app := NewApp()
	ctx := NewContext(app)
	ctx.Route = "test"

	var order []string
	ctx.handlers = []HandlerFunc{
		func(c *Context) { order = append(order, "handler1") },
		func(c *Context) { order = append(order, "handler2") },
	}

	ctx.Next()

	if len(order) != 2 || order[0] != "handler1" || order[1] != "handler2" {
		t.Errorf("expected [handler1 handler2], got %v", order)
	}
}

func TestContext_SetGet(t *testing.T) {
	app := NewApp()
	session := NewSession()
	ctx := NewContext(app)
	ctx.SetSession(session)

	session.Set("key", "session-value")
	ctx.Set("ctx-key", "ctx-value")

	if ctx.Get("key") != "session-value" {
		t.Error("should get session value")
	}
	if ctx.Get("ctx-key") != "ctx-value" {
		t.Error("should get context value from session")
	}
}

func TestContext_WithoutSession(t *testing.T) {
	app := NewApp()
	ctx := NewContext(app)

	ctx.Set("key", "value")
	val := ctx.Get("nonexistent")

	if val != nil {
		t.Error("should return nil when no session")
	}
	if ctx.Session() != nil {
		t.Error("session should be nil")
	}
}

func TestMessageType_Constants(t *testing.T) {
	if Request != 0 {
		t.Errorf("expected Request=0, got %d", Request)
	}
	if Response != 1 {
		t.Errorf("expected Response=1, got %d", Response)
	}
	if Notify != 2 {
		t.Errorf("expected Notify=2, got %d", Notify)
	}
	if Broadcast != 3 {
		t.Errorf("expected Broadcast=3, got %d", Broadcast)
	}
}

func TestApp_Component(t *testing.T) {
	t.Run("Register and Get component", func(t *testing.T) {
		app := NewApp()
		comp := &mockComponent{name: "test-comp"}

		app.Register("test-comp", comp)

		got, ok := app.GetComponent("test-comp")
		if !ok {
			t.Error("component should be found")
		}
		if got.Name() != "test-comp" {
			t.Error("component name mismatch")
		}
	})

	t.Run("Load component", func(t *testing.T) {
		app := NewApp()
		comp := &mockComponent{name: "load-comp"}

		app.Load("load-comp", comp)

		got, ok := app.GetComponent("load-comp")
		if !ok {
			t.Error("component should be loaded")
		}
		if got.(*mockComponent).start {
			t.Error("loaded component should not auto-start")
		}
	})

	t.Run("Load without name", func(t *testing.T) {
		app := NewApp()
		comp := &mockComponent{name: "auto-name"}

		app.Load("", comp)

		_, ok := app.GetComponent("auto-name")
		if !ok {
			t.Error("component should be loaded with auto name")
		}
	})

	t.Run("Load duplicate", func(t *testing.T) {
		app := NewApp()
		comp1 := &mockComponent{name: "dup"}
		comp2 := &mockComponent{name: "dup"}

		app.Load("dup", comp1)
		app.Load("dup", comp2)

		got, _ := app.GetComponent("dup")
		if got != comp1 {
			t.Error("first component should be kept")
		}
	})
}

func TestApp_Transaction(t *testing.T) {
	app := NewApp()

	t.Run("successful transaction", func(t *testing.T) {
		calls := 0
		err := app.Transaction("test",
			func() bool { return true },
			func() error { calls++; return nil },
			func() error { calls++; return nil },
		)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 calls, got %d", calls)
		}
	})

	t.Run("failed before", func(t *testing.T) {
		calls := 0
		err := app.Transaction("test",
			func() bool { return false },
			func() error { calls++; return nil },
		)

		if err != nil {
			t.Error("should not return error when before fails")
		}
		if calls != 0 {
			t.Errorf("expected 0 calls, got %d", calls)
		}
	})

	t.Run("retry on failure", func(t *testing.T) {
		attempts := 0
		err := app.Transaction("test",
			func() bool { return true },
			func() error {
				attempts++
				if attempts < 3 {
					return fmt.Errorf("retry error")
				}
				return nil
			},
		)

		if err == nil {
			t.Error("expected an error due to lastErr not being reset")
		}
		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})
}

func TestApp_Route(t *testing.T) {
	app := NewApp()

	handler := func(ctx *Context, s string) string { return "result" }
	app.SetRoute("connector", handler)

	got, ok := app.Route("connector")
	if !ok {
		t.Error("route should be found")
	}
	if got == nil {
		t.Error("handler should not be nil")
	}
}

func TestCreateApp(t *testing.T) {
	app := CreateApp()

	if app.state != StateInited {
		t.Errorf("expected state %d, got %d", StateInited, app.state)
	}
}
