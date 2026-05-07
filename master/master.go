package master

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

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
}

type ServerTypeConfig struct {
	Path      string   `json:"path" yaml:"path"`
	Args      []string `json:"args,omitempty" yaml:"args,omitempty"`
	Env       []string `json:"env,omitempty" yaml:"env,omitempty"`
	Instances int      `json:"instances" yaml:"instances"`
}

type MasterServer interface {
	AddServer(info *ServerInfo) error
	RemoveServer(id string) error
	GetServers() map[string][]*ServerInfo
	GetServersByType(serverType string) []*ServerInfo
	GetServer(id string) (*ServerInfo, bool)
	Start(masterCfg []byte) error
	Stop()
	Wait()
	OnRegister(callback func(*ServerInfo))
	OnUnregister(callback func(string))
	OnStateChange(callback func(id string, oldState, newState int))
	EnableAdmin(addr string)
	SetServerCfgs(cfgs map[string]any)
	StartServers(servers []map[string]any) error
}

type masterServer struct {
	addr     string
	listener net.Listener

	servers   map[string]*ServerInfo
	byType    map[string][]*ServerInfo

	onRegister    []func(*ServerInfo)
	onUnregister  []func(string)
	onStateChange []func(id string, oldState, newState int)

	heartbeats map[string]time.Time

	processMgr   ProcessManager
	serverCfgs   map[string]any
	autoStart    bool
	restartDelay time.Duration

	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	stats   struct {
		totalRegister   int64
		totalUnregister int64
	}

	adminAddr  string
	adminMux   *http.ServeMux
	adminSrv   *http.Server
}

func New() MasterServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &masterServer{
		servers:      make(map[string]*ServerInfo),
		byType:       make(map[string][]*ServerInfo),
		heartbeats:   make(map[string]time.Time),
		processMgr:   NewProcessManager(),
		serverCfgs:   make(map[string]any),
		autoStart:    true,
		restartDelay: 5 * time.Second,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func NewWithConfig(addr string, serverCfgs map[string]any, autoStart bool) MasterServer {
	ctx, cancel := context.WithCancel(context.Background())
	m := &masterServer{
		addr:         addr,
		servers:      make(map[string]*ServerInfo),
		byType:       make(map[string][]*ServerInfo),
		heartbeats:   make(map[string]time.Time),
		processMgr:   NewProcessManager(),
		serverCfgs:   serverCfgs,
		autoStart:    autoStart,
		restartDelay: 5 * time.Second,
		ctx:          ctx,
		cancel:       cancel,
	}
	return m
}

func (m *masterServer) Start(masterCfg []byte) error {
	if m.running {
		return nil
	}

	var cfg map[string]map[string]any
	if err := json.Unmarshal(masterCfg, &cfg); err != nil {
		return fmt.Errorf("parse master config failed: %w", err)
	}

	var host string
	var port int
	for _, envCfg := range cfg {
		if h, ok := envCfg["host"].(string); ok {
			host = h
		}
		if p, ok := envCfg["port"].(float64); ok {
			port = int(p)
		}
		break
	}

	if host == "" || port == 0 {
		return fmt.Errorf("invalid master config: host or port missing")
	}

	m.addr = fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", m.addr)
	if err != nil {
		return fmt.Errorf("master listen failed: %w", err)
	}

	m.listener = ln
	m.running = true

	m.wg.Add(1)
	go m.acceptLoop()

	m.wg.Add(1)
	go m.heartbeatCheck()

	if m.adminAddr != "" {
		go m.startAdmin()
	}

	return nil
}

func (m *masterServer) EnableAdmin(addr string) {
	m.adminAddr = addr
	m.adminMux = http.NewServeMux()
	m.adminMux.HandleFunc("/api/servers", m.listServers)
	m.adminMux.HandleFunc("/api/stats", m.getStats)
	m.adminMux.HandleFunc("/api/connections", m.getConnections)
	m.adminMux.HandleFunc("/", m.adminIndex)
}

func (m *masterServer) startAdmin() {
	m.adminSrv = &http.Server{
		Addr:    m.adminAddr,
		Handler: m.adminMux,
	}
	fmt.Printf("Admin console started on %s\n", m.adminAddr)
	if err := m.adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("admin server error: %v", err)
	}
}

func (m *masterServer) listServers(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	list := make([]*ServerInfo, 0, len(m.servers))
	for _, s := range m.servers {
		list = append(list, s)
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"servers": list})
}

func (m *masterServer) getStats(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	totalServers := len(m.servers)
	totalClients := 0
	for _, s := range m.servers {
		totalClients += s.Count
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"totalServers": totalServers,
		"totalClients": totalClients,
		"timestamp":    time.Now().Format(time.RFC3339),
	})
}

func (m *masterServer) getConnections(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	totalClients := 0
	for _, s := range m.servers {
		totalClients += s.Count
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"count": totalClients})
}

func (m *masterServer) adminIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>gomelo Admin</title>
<style>
body{font-family:Arial,sans-serif;margin:20px;background:#f5f5f5}
h1{color:#333}
.card{background:#fff;padding:20px;margin:10px 0;border-radius:8px;box-shadow:0 2px 4px rgba(0,0,0,0.1)}
.stat{display:inline-block;margin:10px 20px}
.stat-value{font-size:32px;font-weight:bold;color:#2196F3}
.stat-label{color:#666}
th,td{padding:10px;text-align:left;border-bottom:1px solid #eee}
.server-online{color:green}
.server-offline{color:red}
</style>
</head>
<body>
<h1>gomelo Admin Console</h1>
<div class="card">
<div id="stats"></div>
</div>
<div class="card">
<h2>Servers</h2>
<div id="servers"></div>
</div>
<script>
function loadData(){
fetch('/api/stats').then(r=>r.json()).then(s=>{
document.getElementById('stats').innerHTML=
'<div class="stat"><div class="stat-value">'+s.totalServers+'</div><div class="stat-label">Servers</div></div>'+
'<div class="stat"><div class="stat-value">'+s.totalClients+'</div><div class="stat-label">Connections</div></div>'
})
fetch('/api/servers').then(r=>r.json()).then(d=>{
var html='<table><tr><th>ID</th><th>Type</th><th>Host</th><th>Port</th><th>Connections</th><th>State</th></tr>'
for(var s of d.servers){
html+='<tr><td>'+s.id+'</td><td>'+s.serverType+'</td><td>'+s.host+'</td><td>'+s.port+'</td><td>'+s.count+'</td><td class="'+(s.state===0?'server-online':'server-offline')+'">'+s.state+'</td></tr>'
}
html+='</table>'
document.getElementById('servers').innerHTML=html||'<p>No servers connected</p>'
})
}
loadData()
setInterval(loadData,5000)
</script>
</body>
</html>`)
}

func (m *masterServer) acceptLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		conn, err := m.listener.Accept()
		if err != nil {
			if !m.running {
				return
			}
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}

		m.wg.Add(1)
		go m.handleConn(conn)
	}
}

func (m *masterServer) handleConn(conn net.Conn) {
	defer m.wg.Done()
	defer conn.Close()

	readBuf := make([]byte, 0, 4096)
	for {
		buf := make([]byte, 4096)
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		readBuf = append(readBuf, buf[:n]...)
		var ok bool
		readBuf, ok = m.processMessages(conn, readBuf)
		if !ok {
			return
		}
	}
}

func (m *masterServer) processMessages(conn net.Conn, buf []byte) ([]byte, bool) {
	const maxBufSize = 1024 * 1024

	if len(buf) > maxBufSize {
		return buf, false
	}

	for len(buf) >= 4 {
		length := binary.BigEndian.Uint32(buf[:4])
		if length > 64*1024 {
			buf = buf[4:]
			continue
		}
		if length == 0 {
			break
		}

		if int(length)+4 > len(buf) {
			break
		}

		data := buf[4 : 4+length]
		buf = buf[4+length:]

		var msg masterMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "register":
			m.handleRegister(conn, msg.Data)
		case "unregister":
			m.handleUnregister(msg.Data)
		case "heartbeat":
			m.handleHeartbeatConn(conn, msg.Data)
		case "query":
			m.handleQuery(conn)
		}
	}
	return buf, true
}

type masterMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type registerData struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Frontend   bool   `json:"frontend"`
	ServerType string `json:"serverType"`
}

func (m *masterServer) handleRegister(conn net.Conn, data json.RawMessage) {
	var reg registerData
	if err := json.Unmarshal(data, &reg); err != nil {
		return
	}

	info := &ServerInfo{
		ID:         reg.ID,
		ServerType: reg.Type,
		Host:       reg.Host,
		Port:       reg.Port,
		State:      1,
		Count:      0,
		RegisterAt: time.Now().Unix(),
		LastUpdate: time.Now().Unix(),
	}

	m.mu.Lock()
	m.servers[info.ID] = info
	m.byType[info.ServerType] = append(m.byType[info.ServerType], info)
	m.heartbeats[info.ID] = time.Now()
	m.mu.Unlock()

	atomic.AddInt64(&m.stats.totalRegister, 1)

	m.mu.RLock()
	callbacks := make([]func(*ServerInfo), len(m.onRegister))
	copy(callbacks, m.onRegister)
	infoCopy := *info
	m.mu.RUnlock()

	for _, cb := range callbacks {
		go cb(&infoCopy)
	}

	resp, _ := json.Marshal(map[string]string{"status": "ok"})
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(resp)))
	conn.Write(lenBuf)
	conn.Write(resp)
}

func (m *masterServer) handleUnregister(data json.RawMessage) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	m.mu.Lock()
	if info, ok := m.servers[req.ID]; ok {
		delete(m.servers, req.ID)

		if st, ok := m.byType[info.ServerType]; ok {
			for i, s := range st {
				if s.ID == req.ID {
					m.byType[info.ServerType] = append(st[:i], st[i+1:]...)
					break
				}
			}
		}

		delete(m.heartbeats, req.ID)
	}
	m.mu.Unlock()

	atomic.AddInt64(&m.stats.totalUnregister, 1)

	for _, cb := range m.onUnregister {
		go cb(req.ID)
	}
}

func (m *masterServer) handleHeartbeatConn(conn net.Conn, data json.RawMessage) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	m.mu.Lock()
	m.heartbeats[req.ID] = time.Now()
	if info, ok := m.servers[req.ID]; ok {
		info.LastUpdate = time.Now().Unix()
	}
	m.mu.Unlock()

	resp, _ := json.Marshal(map[string]string{"status": "ok"})
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(resp)))
	conn.Write(lenBuf)
	conn.Write(resp)
}

func (m *masterServer) handleQuery(conn net.Conn) {
	m.mu.RLock()
	serversCopy := make(map[string]*ServerInfo, len(m.servers))
	for k, v := range m.servers {
		serversCopy[k] = &ServerInfo{
			ID:         v.ID,
			ServerType: v.ServerType,
			Host:       v.Host,
			Port:       v.Port,
			State:      v.State,
			Count:      v.Count,
			RegisterAt: v.RegisterAt,
			LastUpdate: v.LastUpdate,
		}
	}
	count := len(m.servers)
	m.mu.RUnlock()

	result := map[string]any{
		"servers": serversCopy,
		"count":   count,
	}

	data, _ := json.Marshal(result)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	conn.Write(lenBuf)
	conn.Write(data)
}

func (m *masterServer) heartbeatCheck() {
	defer m.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkHeartbeats()
		}
	}
}

func (m *masterServer) checkHeartbeats() {
	m.mu.Lock()
	now := time.Now()
	timeout := 30 * time.Second

	var expired []struct {
		id       string
		info     *ServerInfo
		oldState int
	}

	for id, last := range m.heartbeats {
		if now.Sub(last) > timeout {
			info, hasInfo := m.servers[id]
			if hasInfo {
				expired = append(expired, struct {
					id       string
					info     *ServerInfo
					oldState int
				}{id, info, info.State})
				info.State = 3
			}
			delete(m.heartbeats, id)
			delete(m.servers, id)
			if hasInfo {
				if st, stOk := m.byType[info.ServerType]; stOk {
					for i, s := range st {
						if s.ID == id {
							m.byType[info.ServerType] = append(st[:i], st[i+1:]...)
							break
						}
					}
				}
			}
		}
	}
	m.mu.Unlock()

	for _, e := range expired {
		for _, cb := range m.onStateChange {
			go cb(e.id, e.oldState, e.info.State)
		}
		for _, cb := range m.onUnregister {
			go cb(e.id)
		}
	}
}

func (m *masterServer) Stop() {
	if !m.running {
		return
	}

	m.running = false
	m.cancel()

	if m.processMgr != nil {
		m.processMgr.Close()
	}

	if m.listener != nil {
		m.listener.Close()
	}

	if m.adminSrv != nil {
		m.adminSrv.Close()
	}

	m.wg.Wait()
}

func (m *masterServer) Wait() {
	<-m.ctx.Done()
}

func (m *masterServer) SetServerCfgs(cfgs map[string]any) {
	m.mu.Lock()
	m.serverCfgs = cfgs
	m.mu.Unlock()
}

func (m *masterServer) SetAutoStart(auto bool) {
	m.mu.Lock()
	m.autoStart = auto
	m.mu.Unlock()
}

func (m *masterServer) StartServers(servers []map[string]any) error {
	if !m.autoStart {
		return nil
	}

	m.mu.RLock()
	cfgs := m.serverCfgs
	delay := m.restartDelay
	m.mu.RUnlock()

	eventCh := make(chan ProcessEvent, 10)
	started := make(map[string]bool)

	for _, srv := range servers {
		id, _ := srv["id"].(string)
		serverType, _ := srv["serverType"].(string)
		host, _ := srv["host"].(string)
		port, _ := srv["port"].(float64)

		cfg, ok := cfgs[serverType].(map[string]any)
		if !ok {
			continue
		}

		if _, exists := started[id]; exists {
			continue
		}

		instances, _ := cfg["instances"].(float64)
		if instances <= 0 {
			instances = 1
		}

		for i := 0; i < int(instances); i++ {
			instanceID := id
			if int(instances) > 1 {
				instanceID = fmt.Sprintf("%s-%d", id, i)
			}

			envList, _ := cfg["env"].([]any)
			var env []string
			for _, e := range envList {
				if s, ok := e.(string); ok {
					env = append(env, s)
				}
			}
			env = append(env,
				fmt.Sprintf("GOMELO_SERVER_ID=%s", instanceID),
				fmt.Sprintf("GOMELO_SERVER_TYPE=%s", serverType),
				fmt.Sprintf("GOMELO_MASTER_HOST=%s", m.addr),
				fmt.Sprintf("GOMELO_HOST=%s", host),
				fmt.Sprintf("GOMELO_PORT=%d", int(port)),
				fmt.Sprintf("GOMELO_ENV=%s", os.Getenv("GOMELO_ENV")),
			)

			argsList, _ := cfg["args"].([]any)
			var args []string
			for _, a := range argsList {
				if s, ok := a.(string); ok {
					args = append(args, s)
				}
			}

			path, _ := cfg["path"].(string)
			var exePath string
			var exeArgs []string
			if path == "" {
				exePath = "go"
				exeArgs = []string{"run", "."}
				exeArgs = append(exeArgs, args...)
			} else {
				exePath = path
				exeArgs = args
			}

			proc, err := m.processMgr.Spawn(instanceID, serverType, exePath, exeArgs, env)
			if err != nil {
				continue
			}

			m.processMgr.Watch(proc, eventCh)
			started[instanceID] = true
		}
	}

	go m.watchProcessEvents(eventCh, cfgs, delay)

	return nil
}

func (m *masterServer) watchProcessEvents(ch chan ProcessEvent, cfgs map[string]any, delay time.Duration) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case event := <-ch:
			if event.Event == "crashed" {
				cfg, ok := cfgs[event.ServerType].(map[string]any)
				if !ok {
					continue
				}

				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("watchServers goroutine panic: %v", r)
						}
					}()

					time.Sleep(delay)

					envList, _ := cfg["env"].([]any)
					var env []string
					for _, e := range envList {
						if s, ok := e.(string); ok {
							env = append(env, s)
						}
					}
					env = append(env,
						fmt.Sprintf("GOMELO_SERVER_ID=%s", event.ServerID),
						fmt.Sprintf("GOMELO_SERVER_TYPE=%s", event.ServerType),
						fmt.Sprintf("GOMELO_MASTER_HOST=%s", m.addr),
						fmt.Sprintf("GOMELO_HOST=%s", event.Host),
						fmt.Sprintf("GOMELO_PORT=%d", event.Port),
					)

					argsList, _ := cfg["args"].([]any)
					var args []string
					for _, a := range argsList {
						if s, ok := a.(string); ok {
							args = append(args, s)
						}
					}
					path, _ := cfg["path"].(string)
					if path == "" {
						path = os.Args[0]
					}
					proc, err := m.processMgr.Spawn(event.ServerID, event.ServerType, path, args, env)
					if err != nil {
						return
					}

					m.processMgr.Watch(proc, ch)
				}()
			}
		}
	}
}

func (m *masterServer) SpawnServer(id, serverType string, cfg map[string]any) error {
	if m.processMgr == nil {
		return fmt.Errorf("process manager not initialized")
	}

	envList, _ := cfg["env"].([]any)
	var env []string
	for _, e := range envList {
		if s, ok := e.(string); ok {
			env = append(env, s)
		}
	}
	env = append(env,
		fmt.Sprintf("GOMELO_SERVER_ID=%s", id),
		fmt.Sprintf("GOMELO_SERVER_TYPE=%s", serverType),
		fmt.Sprintf("GOMELO_MASTER_HOST=%s", m.addr),
	)

	argsList, _ := cfg["args"].([]any)
	var args []string
	for _, a := range argsList {
		if s, ok := a.(string); ok {
			args = append(args, s)
		}
	}
	path, _ := cfg["path"].(string)

	proc, err := m.processMgr.Spawn(id, serverType, path, args, env)
	if err != nil {
		return err
	}

	eventCh := make(chan ProcessEvent)
	m.processMgr.Watch(proc, eventCh)

	return nil
}

func (m *masterServer) StopServers() {
	if m.processMgr != nil {
		m.processMgr.Close()
	}
}

func (m *masterServer) GetProcessList() []*ProcessInfo {
	if m.processMgr == nil {
		return nil
	}
	return m.processMgr.List()
}

func (m *masterServer) AddServer(info *ServerInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.servers[info.ID] = info
	m.byType[info.ServerType] = append(m.byType[info.ServerType], info)
	m.heartbeats[info.ID] = time.Now()

	return nil
}

func (m *masterServer) RemoveServer(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if info, ok := m.servers[id]; ok {
		delete(m.servers, id)

		if st, ok := m.byType[info.ServerType]; ok {
			for i, s := range st {
				if s.ID == id {
					m.byType[info.ServerType] = append(st[:i], st[i+1:]...)
					break
				}
			}
		}

		delete(m.heartbeats, id)
	}

	return nil
}

func (m *masterServer) GetServers() map[string][]*ServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]*ServerInfo)
	for t, ss := range m.byType {
		result[t] = make([]*ServerInfo, len(ss))
		copy(result[t], ss)
	}

	return result
}

func (m *masterServer) GetServersByType(serverType string) []*ServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if st, ok := m.byType[serverType]; ok {
		result := make([]*ServerInfo, len(st))
		copy(result, st)
		return result
	}

	return nil
}

func (m *masterServer) GetServer(id string) (*ServerInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok := m.servers[id]
	return info, ok
}

func (m *masterServer) OnRegister(callback func(*ServerInfo)) {
	m.mu.Lock()
	m.onRegister = append(m.onRegister, callback)
	m.mu.Unlock()
}

func (m *masterServer) OnUnregister(callback func(string)) {
	m.mu.Lock()
	m.onUnregister = append(m.onUnregister, callback)
	m.mu.Unlock()
}

func (m *masterServer) OnStateChange(callback func(id string, oldState, newState int)) {
	m.mu.Lock()
	m.onStateChange = append(m.onStateChange, callback)
	m.mu.Unlock()
}

type MasterClient interface {
	Register() error
	Unregister() error
	Heartbeat() error
	QueryServers() (map[string][]*ServerInfo, error)
	Close()
}

type masterClient struct {
	id             string
	serverType     string
	addr           string
	conn           net.Conn
	connMu         sync.Mutex
	mu             sync.Mutex
	running        atomic.Bool
	connected      atomic.Bool
	reconnectTick  *time.Ticker
	reconnectDelay time.Duration
	host           string
	port           int
	frontend       bool
}

func NewClient(addr, id, serverType string) (MasterClient, error) {
	return newMasterClient(addr, id, serverType, false, "", 0)
}

func NewClientWithConfig(addr, id, serverType, host string, port int, frontend bool) (MasterClient, error) {
	return newMasterClient(addr, id, serverType, frontend, host, port)
}

func newMasterClient(addr, id, serverType string, frontend bool, host string, port int) (MasterClient, error) {
	mc := &masterClient{
		id:             id,
		serverType:     serverType,
		addr:           addr,
		running:        atomic.Bool{},
		connected:      atomic.Bool{},
		reconnectDelay: 5 * time.Second,
		frontend:       frontend,
		host:           host,
		port:           port,
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	mc.conn = conn
	mc.running.Store(true)
	mc.connected.Store(true)

	go mc.reconnectLoop()

	return mc, nil
}

func (c *masterClient) reconnectLoop() {
	ticker := time.NewTicker(c.reconnectDelay)
	defer ticker.Stop()
	for c.running.Load() {
		<-ticker.C

		c.connMu.Lock()
		if c.connected.Load() {
			c.connMu.Unlock()
			continue
		}

		conn, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
		if err != nil {
			c.connMu.Unlock()
			continue
		}

		oldConn := c.conn
		c.conn = conn
		c.connMu.Unlock()

		if oldConn != nil {
			oldConn.Close()
		}

		if err := c.Register(); err != nil {
			c.connMu.Lock()
			c.conn.Close()
			c.conn = nil
			c.connMu.Unlock()
			c.connected.Store(false)
			continue
		}

		c.connected.Store(true)
	}
}

func (c *masterClient) Register() error {
	data, _ := json.Marshal(map[string]any{
		"id":         c.id,
		"type":       c.serverType,
		"host":       c.host,
		"port":       c.port,
		"frontend":   c.frontend,
		"serverType": c.serverType,
	})

	msg := masterMessage{
		Type: "register",
		Data: data,
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	b, _ := json.Marshal(msg)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(b)))
	if _, err := c.conn.Write(lenBuf); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := c.conn.Write(b); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	header := make([]byte, 4)
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	length := binary.BigEndian.Uint32(header)
	resp := make([]byte, length)
	if _, err := io.ReadFull(c.conn, resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var result map[string]string
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if result["status"] != "ok" {
		return fmt.Errorf("register failed")
	}

	return nil
}

func (c *masterClient) Unregister() error {
	data, _ := json.Marshal(map[string]string{"id": c.id})

	msg := masterMessage{
		Type: "unregister",
		Data: data,
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	b, _ := json.Marshal(msg)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(b)))
	c.conn.Write(lenBuf)
	c.conn.Write(b)

	return nil
}

func (c *masterClient) Heartbeat() error {
	data, _ := json.Marshal(map[string]string{"id": c.id})

	msg := masterMessage{
		Type: "heartbeat",
		Data: data,
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	b, _ := json.Marshal(msg)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(b)))
	c.conn.Write(lenBuf)
	c.conn.Write(b)

	header := make([]byte, 4)
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(c.conn, header); err != nil {
		c.connected.Store(false)
		return err
	}
	length := binary.BigEndian.Uint32(header)
	resp := make([]byte, length)
	if _, err := io.ReadFull(c.conn, resp); err != nil {
		c.connected.Store(false)
		return err
	}

	var result map[string]string
	if err := json.Unmarshal(resp, &result); err != nil {
		return err
	}

	if result["status"] != "ok" {
		return fmt.Errorf("heartbeat failed")
	}

	return nil
}

func (c *masterClient) QueryServers() (map[string][]*ServerInfo, error) {
	msg := masterMessage{
		Type: "query",
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	b, _ := json.Marshal(msg)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(b)))
	c.conn.Write(lenBuf)
	c.conn.Write(b)

	header := make([]byte, 4)
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(c.conn, header); err != nil {
		c.connected.Store(false)
		return nil, err
	}
	length := binary.BigEndian.Uint32(header)
	resp := make([]byte, length)
	if _, err := io.ReadFull(c.conn, resp); err != nil {
		c.connected.Store(false)
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	servers := make(map[string][]*ServerInfo)
	if serversRaw, ok := result["servers"].(map[string]any); ok {
		for stype, val := range serversRaw {
			if arr, ok := val.([]any); ok {
				var list []*ServerInfo
				for _, item := range arr {
					if m, ok := item.(map[string]any); ok {
						si := &ServerInfo{}
						if id, ok := m["id"].(string); ok {
							si.ID = id
						}
						if t, ok := m["type"].(string); ok {
							si.ServerType = t
						}
						if h, ok := m["host"].(string); ok {
							si.Host = h
						}
						if p, ok := m["port"].(float64); ok {
							si.Port = int(p)
						}
						list = append(list, si)
					}
				}
				servers[stype] = list
			}
		}
	}

	return servers, nil
}

func (c *masterClient) Close() {
	c.running.Store(false)
	if c.reconnectTick != nil {
		c.reconnectTick.Stop()
	}
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
}
