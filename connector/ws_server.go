package connector

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chuhongliang/gomelo/forward"
	"github.com/chuhongliang/gomelo/lib"
	"github.com/chuhongliang/gomelo/schema"
	"github.com/chuhongliang/gomelo/selector"

	"github.com/gorilla/websocket"
)

type wsSessionData struct {
	heart   time.Time
	conn    *websocket.Conn
	session *lib.Session
	msgCh   chan *lib.Message
	mu      sync.RWMutex
}

type WebSocketServer struct {
	app         *lib.App
	ln          net.Listener
	opts        *WebSocketOptions
	onConnect   ConnectHandler
	onMessage   MessageHandler
	onClose     CloseHandler
	running     int32
	connections int64
	maxConns    int
	connID      uint64
	blackList   *sync.Map
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
	msgWg       sync.WaitGroup
	readPool    sync.Pool
	forwarder   forward.MessageForwarder
	forwardSel  selector.Selector
	handlers    map[string]Handler
	sessions    map[uint64]*wsSessionData
	heartMu     sync.RWMutex
	schemaMgr   *schema.Manager
	upgrader    websocket.Upgrader
}

type WebSocketOptions struct {
	Type              string
	Host              string
	Port              int
	MaxConns          int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	CheckOrigin       func(origin string) bool
}

func NewWebSocketServer(opts *WebSocketOptions) *WebSocketServer {
	if opts == nil {
		opts = &WebSocketOptions{
			Type:              "ws",
			Host:              "0.0.0.0",
			Port:              3012,
			MaxConns:          10000,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      10 * time.Second,
			HeartbeatInterval: 30 * time.Second,
			HeartbeatTimeout:  90 * time.Second,
		}
	}

	return &WebSocketServer{
		opts:      opts,
		blackList: &sync.Map{},
		stopCh:    make(chan struct{}),
		readPool: sync.Pool{
			New: func() any {
				b := make([]byte, 4096)
				return &b
			},
		},
		sessions:  make(map[uint64]*wsSessionData),
		schemaMgr: schema.NewManager(opts.Type, opts.Type),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *WebSocketServer) SetApp(app *lib.App)                      { s.app = app }
func (s *WebSocketServer) SetForwarder(f forward.MessageForwarder)  { s.forwarder = f }
func (s *WebSocketServer) SetForwardSelector(sel selector.Selector) { s.forwardSel = sel }
func (s *WebSocketServer) GetSchemaManager() *schema.Manager        { return s.schemaMgr }

func (s *WebSocketServer) OnConnect(fn func(*lib.Session)) { s.onConnect = fn }
func (s *WebSocketServer) OnMessage(fn func(*lib.Session, *lib.Message)) {
	s.onMessage = fn
}
func (s *WebSocketServer) OnClose(fn func(*lib.Session)) { s.onClose = fn }

func (s *WebSocketServer) Handle(route string, h Handler) {
	if s.handlers == nil {
		s.handlers = make(map[string]Handler)
	}
	s.handlers[route] = h
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecJSON)
}

func (s *WebSocketServer) RegisterJSONRoute(route string) {
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecJSON)
}

func (s *WebSocketServer) RegisterPBRoute(route string, typeURL string) {
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecProtobuf, typeURL)
}

func (s *WebSocketServer) generateRouteID() uint16 {
	id := atomic.AddUint32(&wsNextRouteID, 1)
	return uint16(id)
}

var wsNextRouteID uint32

func (s *WebSocketServer) Name() string {
	return "websocket"
}

func (s *WebSocketServer) removeSession(connID uint64) {
	s.heartMu.Lock()
	defer s.heartMu.Unlock()
	delete(s.sessions, connID)
}

func (s *WebSocketServer) updateSessionHeart(connID uint64) {
	s.heartMu.Lock()
	defer s.heartMu.Unlock()
	if sd, ok := s.sessions[connID]; ok {
		sd.heart = time.Now()
	}
}

func (s *WebSocketServer) Start(app *lib.App) error {
	s.app = app

	if s.opts.CheckOrigin != nil {
		s.upgrader.CheckOrigin = func(r *http.Request) bool {
			return s.opts.CheckOrigin(r.Header.Get("Origin"))
		}
	}

	addr := fmt.Sprintf("%s:%d", s.opts.Host, s.opts.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	s.ln = ln

	if s.opts.HeartbeatInterval > 0 {
		s.wg.Add(1)
		go s.heartbeatLoop()
	}

	s.wg.Add(1)
	go s.serveHTTP()

	return nil
}

func (s *WebSocketServer) serveHTTP() {
	defer s.wg.Done()
	http.Serve(s.ln, http.HandlerFunc(s.handleHTTP))
}

func (s *WebSocketServer) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	if s.ln != nil {
		s.ln.Close()
	}
	s.wg.Wait()
	s.msgWg.Wait()
	return nil
}

func (s *WebSocketServer) AddToBlackList(ip string) {
	s.blackList.Store(ip, true)
}

func (s *WebSocketServer) RemoveFromBlackList(ip string) {
	s.blackList.Delete(ip)
}

func (s *WebSocketServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if s.opts.Type == "wss" {
		// For wss, TLS should be handled by a reverse proxy or cert files
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	s.handleWSConn(conn)
}

func (s *WebSocketServer) handleWSConn(conn *websocket.Conn) {
	ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		ip = conn.RemoteAddr().String()
	}
	if _, ok := s.blackList.Load(ip); ok {
		conn.Close()
		return
	}
	if s.opts.MaxConns > 0 && atomic.LoadInt64(&s.connections) >= int64(s.opts.MaxConns) {
		conn.Close()
		return
	}
	atomic.AddInt64(&s.connections, 1)

	defer func() {
		atomic.AddInt64(&s.connections, -1)
		conn.Close()
	}()

	connID := atomic.AddUint64(&s.connID, 1)
	s.heartMu.Lock()
	sd := &wsSessionData{
		heart: time.Now(),
		conn:  conn,
	}
	s.sessions[connID] = sd
	msgCh := make(chan *lib.Message, 256)
	sd.msgCh = msgCh
	s.heartMu.Unlock()

	session := lib.NewSession()
	session.SetID(connID)
	session.SetConnectionID(connID)
	session.SetConnection(&wsConnection{id: connID, Conn: conn})
	session.Set("remoteAddr", conn.RemoteAddr().String())

	s.msgWg.Add(1)
	go s.processSessionMessages(session, msgCh)

	if s.onConnect != nil {
		s.onConnect(session)
	}

	if s.schemaMgr != nil {
		schema := s.schemaMgr.GetServerSchema()
		session.SendSchema(&schema)
	}

	s.readLoop(conn, session, connID, msgCh)
}

func (s *WebSocketServer) readLoop(conn *websocket.Conn, session *lib.Session, connID uint64, msgCh chan *lib.Message) {
	defer func() {
		s.removeSession(connID)
		close(msgCh)
		if s.onClose != nil {
			s.onClose(session)
		}
	}()

	conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout))

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if msgType != websocket.BinaryMessage {
			continue
		}

		s.updateSessionHeart(connID)

		payload := data
		if len(data) >= 4 {
			length := binary.BigEndian.Uint32(data[:4])
			if int(length) == len(data)-4 {
				payload = data[4:]
			}
		}

		var msg lib.Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}

		select {
		case msgCh <- &msg:
		default:
			session.Close()
		}
	}
}

func (s *WebSocketServer) processSessionMessages(session *lib.Session, msgCh chan *lib.Message) {
	defer s.msgWg.Done()
	for msg := range msgCh {
		s.handleMessage(session, msg)
	}
}

func (s *WebSocketServer) handleMessage(session *lib.Session, msg *lib.Message) {
	if msg.Type == lib.Request || msg.Type == lib.Notify {
		ctx := lib.NewContext(s.app)
		ctx.SetSession(session)
		ctx.Route = msg.Route
		ctx.SetRequest(msg)

		if s.app != nil && s.app.Pipeline() != nil {
			s.app.Pipeline().Invoke(ctx)
			if ctx.Resp != nil && msg.Type == lib.Request {
				session.SendResponse(msg.Seq, msg.Route, ctx.Resp.Body)
				return
			}
		}

		handler, ok := s.handlers[msg.Route]
		if ok {
			resp, err := handler(session, msg)
			if msg.Type == lib.Request {
				session.SendResponse(msg.Seq, msg.Route, resp)
				if err == nil {
					return
				}
			}
			if err != nil {
				s.forwardMessage(session, msg)
				return
			}
		}

		if s.app != nil && s.app.IsFrontend() && s.shouldForward(msg.Route) {
			s.forwardMessage(session, msg)
		}
	}
}

func (s *WebSocketServer) shouldForward(route string) bool {
	if s.forwardSel == nil {
		return false
	}

	parts := splitRoute(route)
	if len(parts) == 0 {
		return false
	}

	serverType := parts[0]
	return s.forwardSel.Select(serverType).ID != ""
}

func (s *WebSocketServer) forwardMessage(session *lib.Session, msg *lib.Message) {
	if s.forwarder == nil {
		return
	}

	parts := splitRoute(msg.Route)
	if len(parts) == 0 {
		return
	}

	serverType := parts[0]
	server := s.forwardSel.Select(serverType)
	if server.ID == "" {
		return
	}

	go s.forwarder.Forward(context.Background(), session, msg, server)
}

func (s *WebSocketServer) GetConnectionCount() int64 {
	return atomic.LoadInt64(&s.connections)
}

func (s *WebSocketServer) heartbeatLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.opts.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkHeartbeats()
		}
	}
}

func (s *WebSocketServer) checkHeartbeats() {
	now := time.Now()
	timeout := s.opts.HeartbeatTimeout

	s.heartMu.Lock()
	defer s.heartMu.Unlock()

	for connID, sd := range s.sessions {
		elapsed := now.Sub(sd.heart)
		if elapsed > timeout {
			sd.conn.Close()
			delete(s.sessions, connID)
			if s.onClose != nil {
				s.onClose(sd.session)
			}
		}
	}
}

type wsConnection struct {
	id   uint64
	Conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsConnection) ID() uint64 {
	return c.id
}

func (c *wsConnection) Close() {
	c.Conn.Close()
}

func (c *wsConnection) Send(msg *lib.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(websocket.BinaryMessage, append(header, data...))
}

func (c *wsConnection) SendRaw(data []byte) error {
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(websocket.BinaryMessage, append(header, data...))
}

func (c *wsConnection) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

func (c *wsConnection) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

func (c *wsConnection) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *wsConnection) ReadMessage() ([]byte, error) {
	msgType, data, err := c.Conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if msgType != websocket.BinaryMessage {
		return nil, errors.New("not a binary message")
	}
	return data, nil
}

func (c *wsConnection) WriteMessage(data []byte) error {
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(websocket.BinaryMessage, append(header, data...))
}
