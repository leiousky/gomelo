package rpc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type RPCServer interface {
	Register(service string, impl any) error
	Start() error
	Stop()
	Addrs() map[string]string
}

type rpcHandler struct {
	receiver any
	method   reflect.Method
	hasError bool
}

type ServerOptions struct {
	Timeout time.Duration
}

type rpcServer struct {
	addr      string
	listener  net.Listener
	handlers  map[string]map[string]*rpcHandler
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   atomic.Bool
	timeout   time.Duration
	semaphore chan struct{}
}

func NewServer(addr string) RPCServer {
	return NewServerWithOptions(addr, nil)
}

func NewServerWithOptions(addr string, opts *ServerOptions) RPCServer {
	if opts == nil {
		opts = &ServerOptions{Timeout: 30 * time.Second}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &rpcServer{
		addr:      addr,
		handlers:  make(map[string]map[string]*rpcHandler),
		ctx:       ctx,
		cancel:    cancel,
		stopCh:    make(chan struct{}),
		timeout:   opts.Timeout,
		semaphore: make(chan struct{}, 1000),
	}
}

func (s *rpcServer) Register(service string, impl any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handlers[service] != nil {
		return fmt.Errorf("service %s already registered", service)
	}

	t := reflect.TypeOf(impl)
	v := reflect.ValueOf(impl)

	methods := make(map[string]*rpcHandler)
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if m.Type.NumIn() != 3 {
			continue
		}

		if m.Type.In(1) != reflect.TypeOf((*context.Context)(nil)).Elem() {
			continue
		}

		numOut := m.Type.NumOut()
		if numOut != 1 && numOut != 2 {
			continue
		}
		hasError := numOut == 2
		if hasError && !m.Type.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			continue
		}

		methods[m.Name] = &rpcHandler{
			receiver: v.Interface(),
			method:   m,
			hasError: hasError,
		}
	}

	if len(methods) == 0 {
		return fmt.Errorf("no valid methods found for service %s", service)
	}

	s.handlers[service] = methods
	return nil
}

func (s *rpcServer) Start() error {
	if s.running.Load() {
		return nil
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	s.listener = ln

	s.mu.Lock()
	s.running.Store(true)
	s.mu.Unlock()

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *rpcServer) acceptLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			running := s.running.Load()
			s.mu.RUnlock()
			if !running {
				return
			}
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *rpcServer) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	acquired := false
	for !acquired {
		select {
		case s.semaphore <- struct{}{}:
			acquired = true
		case <-s.ctx.Done():
			return
		}
	}
	defer func() { <-s.semaphore }()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		header := make([]byte, 4)
		if err := s.readFull(conn, header); err != nil {
			return
		}

		size := binary.BigEndian.Uint32(header)
		if size > 1024*1024 {
			return
		}

		select {
		case <-s.ctx.Done():
			return
		default:
		}

		body := make([]byte, size)
		if err := s.readFull(conn, body); err != nil {
			return
		}

		s.handleRequest(conn, body)
	}
}

func (s *rpcServer) readFull(conn net.Conn, buf []byte) error {
	n := 0
	for n < len(buf) {
		nn, err := conn.Read(buf[n:])
		if err != nil {
			return err
		}
		n += nn
	}
	return nil
}

func (s *rpcServer) handleRequest(conn net.Conn, body []byte) {
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return
	}

	s.mu.RLock()
	service, ok := s.handlers[req.Service]
	s.mu.RUnlock()

	if !ok || service == nil {
		resp := rpcResponse{
			Seq:   req.Seq,
			Error: "service not found",
		}
		s.sendResponse(conn, req.Seq, resp)
		return
	}

	handler, ok := service[req.Method]
	if !ok || handler == nil {
		resp := rpcResponse{
			Seq:   req.Seq,
			Error: "method not found",
		}
		s.sendResponse(conn, req.Seq, resp)
		return
	}

	var args any
	if req.Args != nil {
		args = req.Args
	}

	if handler.receiver == nil {
		resp := rpcResponse{
			Seq:   req.Seq,
			Error: "handler receiver is nil",
		}
		s.sendResponse(conn, req.Seq, resp)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	var result []reflect.Value
	var callErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("rpc handler panic: service=%s method=%s err=%v\n%s", req.Service, req.Method, r, debug.Stack())
				result = nil
			}
		}()
		arg, err := coerceRPCArg(args, handler.method.Type.In(2))
		if err != nil {
			callErr = err
			return
		}
		result = handler.method.Func.Call([]reflect.Value{
			reflect.ValueOf(handler.receiver),
			reflect.ValueOf(ctx),
			arg,
		})
	}()

	if callErr != nil {
		resp := rpcResponse{
			Seq:   req.Seq,
			Error: fmt.Sprintf("invalid args: %v", callErr),
		}
		s.sendResponse(conn, req.Seq, resp)
		return
	}

	if result == nil {
		resp := rpcResponse{
			Seq:   req.Seq,
			Error: "handler panic",
		}
		s.sendResponse(conn, req.Seq, resp)
		return
	}

	var reply any
	if len(result) > 0 {
		v := result[0]
		if v.IsValid() && !(v.Kind() == reflect.Ptr && v.IsNil()) && !(v.Kind() == reflect.Interface && v.IsNil()) {
			reply = v.Interface()
		}
	}
	if handler.hasError && len(result) > 1 && !result[1].IsNil() {
		err, _ := result[1].Interface().(error)
		resp := rpcResponse{
			Seq:   req.Seq,
			Error: err.Error(),
		}
		s.sendResponse(conn, req.Seq, resp)
		return
	}

	resp := rpcResponse{
		Seq:   req.Seq,
		Reply: reply,
	}

	s.sendResponse(conn, req.Seq, resp)
}

func coerceRPCArg(args any, target reflect.Type) (reflect.Value, error) {
	if args == nil {
		return reflect.Zero(target), nil
	}

	v := reflect.ValueOf(args)
	if v.IsValid() && v.Type().AssignableTo(target) {
		return v, nil
	}
	if v.IsValid() && v.Type().ConvertibleTo(target) {
		return v.Convert(target), nil
	}

	ptr := reflect.New(target)
	data, err := json.Marshal(args)
	if err != nil {
		return reflect.Value{}, err
	}
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

func (s *rpcServer) sendResponse(conn net.Conn, seq uint64, resp rpcResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	_, err = conn.Write(append(header, data...))
	return err
}

func (s *rpcServer) Stop() {
	if !s.running.Load() {
		return
	}

	s.running.Store(false)
	s.cancel()
	close(s.stopCh)

	if s.listener != nil {
		s.listener.Close()
	}

	s.wg.Wait()
}

func (s *rpcServer) Addrs() map[string]string {
	return map[string]string{"rpc": s.addr}
}
