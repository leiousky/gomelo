package connector

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chuhongliang/gomelo/forward"
	"github.com/chuhongliang/gomelo/lib"
	routelib "github.com/chuhongliang/gomelo/route"
	"github.com/chuhongliang/gomelo/schema"
	"github.com/chuhongliang/gomelo/selector"
)

type (
	ConnectHandler = lib.ConnectHandler
	MessageHandler = lib.MessageHandler
	CloseHandler   = lib.CloseHandler
)

type Handler func(session *lib.Session, msg *lib.Message) (any, error)

type sessionData struct {
	heart time.Time
	conn  lib.Connection
	msgCh chan *lib.Message
	mu    sync.RWMutex
}

type Server struct {
	app         *lib.App
	ln          net.Listener
	opts        *ServerOptions
	onConnect   ConnectHandler
	onMessage   MessageHandler
	onClose     CloseHandler
	running     atomic.Bool
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
	sessions    map[uint64]*sessionData
	heartMu     sync.RWMutex
	schemaMgr   *schema.Manager
}

type ServerOptions struct {
	Type              string
	Host              string
	Port              int
	MaxConns          int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	CertFile          string
	KeyFile           string
}

func NewServer(opts *ServerOptions) *Server {
	if opts == nil {
		opts = &ServerOptions{
			Type:              "tcp",
			Host:              "0.0.0.0",
			Port:              3010,
			MaxConns:          10000,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      10 * time.Second,
			HeartbeatInterval: 30 * time.Second,
			HeartbeatTimeout:  90 * time.Second,
		}
	}

	return &Server{
		opts:      opts,
		blackList: &sync.Map{},
		stopCh:    make(chan struct{}),
		readPool: sync.Pool{
			New: func() any {
				b := make([]byte, 4096)
				return b
			},
		},
		sessions:  make(map[uint64]*sessionData),
		schemaMgr: schema.NewManager(opts.Type, opts.Type),
	}
}

func (s *Server) SetApp(app *lib.App)                      { s.app = app }
func (s *Server) SetForwarder(f forward.MessageForwarder)  { s.forwarder = f }
func (s *Server) SetForwardSelector(sel selector.Selector) { s.forwardSel = sel }
func (s *Server) GetSchemaManager() *schema.Manager        { return s.schemaMgr }

func (s *Server) OnConnect(fn func(*lib.Session)) { s.onConnect = fn }
func (s *Server) OnClose(fn func(*lib.Session))   { s.onClose = fn }

func (s *Server) Handle(route string, h Handler) {
	if s.handlers == nil {
		s.handlers = make(map[string]Handler)
	}
	s.handlers[route] = h
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecJSON)
}

func (s *Server) RegisterJSONRoute(route string) {
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecJSON)
}

func (s *Server) RegisterPBRoute(route string, typeURL string) {
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecProtobuf, typeURL)
}

func (s *Server) generateRouteID() uint16 {
	id := atomic.AddUint32((*uint32)(unsafe.Pointer(&nextRouteID)), 1)
	return uint16(id)
}

var nextRouteID uint32

func (s *Server) Name() string {
	return "connector"
}

func (s *Server) removeSession(connID uint64) {
	s.heartMu.Lock()
	defer s.heartMu.Unlock()
	delete(s.sessions, connID)
}

func (s *Server) updateSessionHeart(connID uint64) {
	s.heartMu.Lock()
	defer s.heartMu.Unlock()
	if sd, ok := s.sessions[connID]; ok {
		sd.heart = time.Now()
	}
}

func (s *Server) Start(app *lib.App) error {
	s.app = app
	s.running.Store(true)
	addr := fmt.Sprintf("%s:%d", s.opts.Host, s.opts.Port)
	var err error

	switch s.opts.Type {
	case "ssl":
		if s.opts.CertFile == "" || s.opts.KeyFile == "" {
			return fmt.Errorf("ssl requires cert and key files")
		}
		cert, err := tls.LoadX509KeyPair(s.opts.CertFile, s.opts.KeyFile)
		if err != nil {
			return fmt.Errorf("load cert failed: %w", err)
		}
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
		s.ln, err = tls.Listen("tcp", addr, tlsConfig)
	default:
		s.ln, err = net.Listen("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	if s.opts.HeartbeatInterval > 0 {
		s.wg.Add(1)
		go s.heartbeatLoop()
	}

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *Server) Stop() error {
	s.running.Store(false)
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

func (s *Server) AddToBlackList(ip string) {
	s.blackList.Store(ip, true)
}

func (s *Server) RemoveFromBlackList(ip string) {
	s.blackList.Delete(ip)
}

func (s *Server) handleConn(conn net.Conn) {
	ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		ip = conn.RemoteAddr().String()
	}
	if _, ok := s.blackList.Load(ip); ok {
		conn.Close()
		return
	}

	defer func() {
		atomic.AddInt64(&s.connections, -1)
	}()

	connID := atomic.AddUint64(&s.connID, 1)
	s.heartMu.Lock()
	sd := &sessionData{
		heart: time.Now(),
		conn:  &simpleConn{id: connID, conn: conn},
	}
	s.sessions[connID] = sd
	msgCh := make(chan *lib.Message, 256)
	sd.msgCh = msgCh
	s.heartMu.Unlock()

	sconn := sd.conn

	session := lib.NewSession()
	session.SetID(connID)
	session.SetConnectionID(connID)
	session.SetConnection(sconn)
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

	s.readLoop(conn, sconn, session, connID, msgCh)
}

func (s *Server) readLoop(conn net.Conn, sconn lib.Connection, session *lib.Session, connID uint64, msgCh chan *lib.Message) {
	const maxReadBufSize = 64 * 1024
	readBuf := make([]byte, 0, 4096)
	defer func() {
		s.removeSession(connID)
		sconn.Close()
		close(msgCh)
		if s.onClose != nil {
			s.onClose(session)
		}
	}()

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		buf := s.readPool.Get().([]byte)
		conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout))
		n, err := conn.Read(buf)
		if err != nil {
			s.readPool.Put(buf)
			return
		}

		s.updateSessionHeart(connID)

		readBuf = append(readBuf, buf[:n]...)
		s.readPool.Put(buf)

		if len(readBuf) > maxReadBufSize {
			// buffer overflow: close connection to prevent memory exhaustion
			log.Printf("tcp readLoop: readBuf exceeded max size (%d bytes), closing session=%d", len(readBuf), session.ID())
			s.readPool.Put(buf)
			return
		}

		s.dispatchMessages(session, &readBuf, msgCh)
	}
}

func (s *Server) dispatchMessages(session *lib.Session, buf *[]byte, msgCh chan *lib.Message) {
	for len(*buf) >= 4 {
		length := binary.BigEndian.Uint32((*buf)[:4])
		if length > 64*1024 {
			*buf = (*buf)[4:]
			continue
		}

		if int(length)+4 > len(*buf) {
			return
		}

		data := (*buf)[4 : 4+length]
		*buf = (*buf)[4+length:]

		var msg lib.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		select {
		case msgCh <- &msg:
		default:
			// drop message when channel is full instead of closing the session
			log.Printf("tcp dispatchMessages: msgCh full for session=%d, dropping message", session.ID())
		}
	}
}

func (s *Server) processSessionMessages(session *lib.Session, msgCh chan *lib.Message) {
	defer s.msgWg.Done()
	for msg := range msgCh {
		s.handleMessage(session, msg)
	}
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		conn, err := s.ln.Accept()
		if err != nil {
			continue
		}

		if s.opts.MaxConns > 0 && atomic.LoadInt64(&s.connections) >= int64(s.opts.MaxConns) {
			conn.Close()
			continue
		}
		atomic.AddInt64(&s.connections, 1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleMessage(session *lib.Session, msg *lib.Message) {
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

func (s *Server) shouldForward(route string) bool {
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

func (s *Server) forwardMessage(session *lib.Session, msg *lib.Message) {
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

type simpleConn struct {
	id   uint64
	conn net.Conn
	mu   sync.Mutex
}

func (c *simpleConn) ID() uint64           { return c.id }
func (c *simpleConn) Close()               { c.conn.Close() }
func (c *simpleConn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

func (c *simpleConn) Send(msg *lib.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := msg.Encode()
	if err != nil {
		return err
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	_, err = c.conn.Write(append(header[:], data...))
	return err
}

func (c *simpleConn) SendRaw(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	_, err := c.conn.Write(append(header[:], data...))
	return err
}

func splitRoute(route string) []string {
	return routelib.SplitRoute(route)
}

func (s *Server) GetConnectionCount() int64 {
	return atomic.LoadInt64(&s.connections)
}

func (s *Server) heartbeatLoop() {
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

func (s *Server) checkHeartbeats() {
	now := time.Now()
	timeout := s.opts.HeartbeatTimeout

	s.heartMu.Lock()
	var expired []*sessionData
	var expiredIDs []uint64
	for id, sd := range s.sessions {
		if now.Sub(sd.heart) > timeout {
			expired = append(expired, sd)
			expiredIDs = append(expiredIDs, id)
		}
	}
	for _, id := range expiredIDs {
		delete(s.sessions, id)
	}
	s.heartMu.Unlock()

	for _, sd := range expired {
		sd.conn.Close()
	}
}
