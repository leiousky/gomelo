package connector

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chuhongliang/gomelo/forward"
	"github.com/chuhongliang/gomelo/lib"
	"github.com/chuhongliang/gomelo/schema"
	"github.com/chuhongliang/gomelo/selector"
)

type udpSessionData struct {
	heart   time.Time
	conn    *lib.UDPConnection
	session *lib.Session
	addr    *net.UDPAddr
	mu      sync.RWMutex
}

type UDPServer struct {
	app        *lib.App
	conn       *net.UDPConn
	opts       *UDPServerOptions
	onConnect  lib.ConnectHandler
	onMessage  lib.MessageHandler
	onClose    lib.CloseHandler
	running    int32
	connID     uint64
	maxConns   int
	sessions   map[string]*udpSessionData
	sessionMu  sync.RWMutex
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
	readPool   sync.Pool
	forwarder  forward.MessageForwarder
	forwardSel selector.Selector
	handlers   map[string]Handler
	schemaMgr  *schema.Manager
}

type UDPServerOptions struct {
	Type              string
	Host              string
	Port              int
	MaxConns          int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

func NewUDPServer(opts *UDPServerOptions) *UDPServer {
	if opts == nil {
		opts = &UDPServerOptions{
			Host:              "0.0.0.0",
			Port:              3011,
			MaxConns:          10000,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      10 * time.Second,
			HeartbeatInterval: 30 * time.Second,
			HeartbeatTimeout:  90 * time.Second,
		}
	}

	return &UDPServer{
		opts:     opts,
		stopCh:   make(chan struct{}),
		sessions: make(map[string]*udpSessionData),
		readPool: sync.Pool{
			New: func() any {
				b := make([]byte, 65535)
				// returning &b is safe here: sync.Pool retains the value across Get/Put cycles
				return &b
			},
		},
		handlers:  make(map[string]Handler),
		schemaMgr: schema.NewManager(opts.Type, opts.Type),
	}
}

func (s *UDPServer) SetForwarder(f forward.MessageForwarder)  { s.forwarder = f }
func (s *UDPServer) SetForwardSelector(sel selector.Selector) { s.forwardSel = sel }
func (s *UDPServer) GetSchemaManager() *schema.Manager        { return s.schemaMgr }

func (s *UDPServer) Handle(route string, h Handler) {
	s.handlers[route] = h
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecJSON)
}

func (s *UDPServer) RegisterJSONRoute(route string) {
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecJSON)
}

func (s *UDPServer) RegisterPBRoute(route string, typeURL string) {
	s.schemaMgr.RegisterRoute(route, s.generateRouteID(), schema.CodecProtobuf, typeURL)
}

func (s *UDPServer) generateRouteID() uint16 {
	id := atomic.AddUint32(&udpNextRouteID, 1)
	return uint16(id)
}

var udpNextRouteID uint32

func (s *UDPServer) Start() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", s.opts.Host, s.opts.Port))
	if err != nil {
		return fmt.Errorf("resolve udp addr: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}

	s.conn = conn
	atomic.StoreInt32(&s.running, 1)

	s.wg.Add(2)
	go s.readLoop()
	go s.cleanupLoop()

	log.Printf("UDP server started on %s:%d", s.opts.Host, s.opts.Port)
	return nil
}

func (s *UDPServer) Stop() {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return
	}

	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.sessionMu.Lock()
		for key, sd := range s.sessions {
			if sd.session != nil && s.onClose != nil {
				s.onClose(sd.session)
			}
			delete(s.sessions, key)
		}
		s.sessionMu.Unlock()

		if s.conn != nil {
			s.conn.Close()
		}

		s.wg.Wait()
		log.Printf("UDP server stopped")
	})
}

func (s *UDPServer) readLoop() {
	defer s.wg.Done()

	for atomic.LoadInt32(&s.running) == 1 {
		select {
		case <-s.stopCh:
			return
		default:
		}

		s.conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout))

		bufPtr := s.readPool.Get().(*[]byte)
		n, addr, err := s.conn.ReadFromUDP(*bufPtr)
		if err != nil {
			s.readPool.Put(bufPtr)
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}

		s.handlePacket(addr, (*bufPtr)[:n])
		s.readPool.Put(bufPtr)
	}
}

func (s *UDPServer) handlePacket(addr *net.UDPAddr, data []byte) {
	if len(data) < 4 {
		return
	}

	length := binary.BigEndian.Uint32(data[:4])
	if int(length)+4 > len(data) {
		return
	}

	payload := data[4 : 4+length]
	msg := &lib.Message{}
	if err := msg.Decode(payload); err != nil {
		log.Printf("UDP decode error: %v", err)
		return
	}

	key := sessionKey(addr)
	s.sessionMu.Lock()
	sd, exists := s.sessions[key]
	if !exists {
		if s.maxConns > 0 && len(s.sessions) >= s.maxConns {
			s.sessionMu.Unlock()
			return
		}

		connID := atomic.AddUint64(&s.connID, 1)
		udpConn := lib.NewUDPConnection(connID, addr, s.conn)

		session := lib.NewSession()
		session.SetConnection(udpConn)
		session.SetServerType(s.opts.Type)

		sd = &udpSessionData{
			heart:   time.Now(),
			conn:    udpConn,
			session: session,
			addr:    addr,
		}
		s.sessions[key] = sd

		if s.onConnect != nil {
			s.onConnect(session)
		}

		if s.schemaMgr != nil {
			schema := s.schemaMgr.GetServerSchema()
			session.SendSchema(&schema)
		}
	} else {
		sd.heart = time.Now()
	}
	s.sessionMu.Unlock()

	if s.app != nil && s.app.Pipeline() != nil {
		ctx := lib.NewContext(s.app)
		ctx.SetSession(sd.session)
		ctx.Route = msg.Route
		ctx.SetRequest(msg)
		s.app.Pipeline().Invoke(ctx)
		if ctx.Resp != nil && msg.Type == lib.Request {
			s.sendResponse(sd, msg.Seq, msg.Route, ctx.Resp.Body)
			return
		}
	}

	if handler, ok := s.handlers[msg.Route]; ok {
		resp, err := handler(sd.session, msg)
		if msg.Type == lib.Request {
			s.sendResponse(sd, msg.Seq, msg.Route, resp)
			if err == nil {
				return
			}
		}
		if err != nil {
			_ = s.forwardSession(sd.session, msg)
			return
		}
	}

	if s.onMessage != nil {
		s.onMessage(sd.session, msg)
	}
}

func (s *UDPServer) sendResponse(sd *udpSessionData, seq uint64, route string, data any) {
	msg := &lib.Message{
		Type:  lib.Response,
		Route: route,
		Seq:   seq,
		Body:  data,
	}

	sd.mu.RLock()
	closed := sd.conn == nil
	sd.mu.RUnlock()

	if closed {
		return
	}

	if err := sd.conn.Send(msg); err != nil {
		log.Printf("UDP send response error: %v", err)
	}
}

func (s *UDPServer) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.opts.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanupSessions()
		}
	}
}

func (s *UDPServer) cleanupSessions() {
	now := time.Now()
	timeout := s.opts.HeartbeatTimeout

	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()

	for key, sd := range s.sessions {
		sd.mu.RLock()
		elapsed := now.Sub(sd.heart)
		sd.mu.RUnlock()

		if elapsed > timeout {
			if s.onClose != nil && sd.session != nil {
				s.onClose(sd.session)
			}
			delete(s.sessions, key)
		}
	}
}

func (s *UDPServer) OnConnect(h lib.ConnectHandler) {
	s.onConnect = h
}

func (s *UDPServer) OnMessage(h lib.MessageHandler) {
	s.onMessage = h
}

func (s *UDPServer) OnClose(h lib.CloseHandler) {
	s.onClose = h
}

func (s *UDPServer) GetSessionCount() int {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	return len(s.sessions)
}

func (s *UDPServer) Broadcast(route string, msg *lib.Message) error {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()

	for _, sd := range s.sessions {
		sd.conn.Send(msg)
	}
	return nil
}

func (s *UDPServer) GetSession(addr *net.UDPAddr) *lib.Session {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()

	if sd, ok := s.sessions[sessionKey(addr)]; ok {
		return sd.session
	}
	return nil
}

func (s *UDPServer) RemoveSession(addr *net.UDPAddr) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()

	key := sessionKey(addr)
	if sd, ok := s.sessions[key]; ok {
		if s.onClose != nil && sd.session != nil {
			s.onClose(sd.session)
		}
		delete(s.sessions, key)
	}
}

func (s *UDPServer) Forward(route string, msg *lib.Message) error {
	return s.forwardSession(nil, msg)
}

func (s *UDPServer) forwardSession(session *lib.Session, msg *lib.Message) error {
	if s.forwarder == nil || s.forwardSel == nil {
		return fmt.Errorf("no forwarder or selector configured")
	}

	parts := strings.Split(msg.Route, ".")
	if len(parts) == 0 {
		return fmt.Errorf("invalid route")
	}

	serverType := parts[0]
	server := s.forwardSel.Select(serverType)
	if server.ID == "" {
		return fmt.Errorf("no server found for type: %s", serverType)
	}

	return s.forwarder.Forward(context.Background(), session, msg, server)
}

func (s *UDPServer) SetApp(app *lib.App) {
	s.app = app
}

func (s *UDPServer) SetType(t string) {
	s.opts.Type = t
}

func (s *UDPServer) SetMaxConns(n int) {
	s.maxConns = n
}

func (s *UDPServer) SetHeartbeat(interval, timeout time.Duration) {
	s.opts.HeartbeatInterval = interval
	s.opts.HeartbeatTimeout = timeout
}

func sessionKey(addr *net.UDPAddr) string {
	return addr.String()
}
