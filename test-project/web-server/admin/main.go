package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

var httpAddr = flag.String("http", ":3006", "HTTP listen address")
var masterAddr = flag.String("master", "127.0.0.1:3005", "Master server address")

type AdminServer struct {
	masterAddr string
	servers    map[string]*ServerStat
	mu         sync.RWMutex
	mux        *http.ServeMux
	server     *http.Server
}

type ServerStat struct {
	ID      string "json:"id""
	Type    string "json:"type""
	State   string "json:"state""
	Clients int    "json:"clients""
	Host    string "json:"host""
	Port    int    "json:"port""
}

type masterMessage struct {
	Type string          "json:"type""
	Data json.RawMessage "json:"data""
}

type serverInfo struct {
	ID         string "json:"id""
	ServerType string "json:"serverType""
	Host       string "json:"host""
	Port       int    "json:"port""
	Frontend   bool   "json:"frontend""
	State      int    "json:"state""
	Count      int    "json:"count""
}

func main() {
	flag.Parse()

	admin := &AdminServer{
		masterAddr: *masterAddr,
		servers:    make(map[string]*ServerStat),
	}

	admin.mux = http.NewServeMux()
	admin.mux.HandleFunc("/api/servers", admin.listServers)
	admin.mux.HandleFunc("/api/stats", admin.getStats)
	admin.mux.HandleFunc("/api/connections", admin.getConnections)
	admin.mux.HandleFunc("/", admin.index)

	admin.server = &http.Server{
		Addr:    *httpAddr,
		Handler: admin.mux,
	}

	go admin.watchMaster()

	fmt.Printf("Admin server starting on %s\n", *httpAddr)
	admin.server.ListenAndServe()
}

func (a *AdminServer) watchMaster() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		servers, err := a.queryMasterServers()
		if err != nil {
			continue
		}

		a.mu.Lock()
		a.servers = make(map[string]*ServerStat)
		for _, s := range servers {
			a.servers[s.ID] = &ServerStat{
				ID:      s.ID,
				Type:    s.ServerType,
				State:   "online",
				Clients: s.Count,
				Host:    s.Host,
				Port:    s.Port,
			}
		}
		a.mu.Unlock()
	}
}

func (a *AdminServer) queryMasterServers() ([]serverInfo, error) {
	conn, err := net.DialTimeout("tcp", a.masterAddr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := masterMessage{Type: "query"}
	data, _ := json.Marshal(req)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	conn.Write(lenBuf)
	conn.Write(data)

	header := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header)
	resp := make([]byte, length)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	var servers []serverInfo
	if serversRaw, ok := result["servers"].(map[string]any); ok {
		for _, val := range serversRaw {
			if arr, ok := val.([]any); ok {
				for _, item := range arr {
					if m, ok := item.(map[string]any); ok {
						si := serverInfo{}
						if id, ok := m["id"].(string); ok {
							si.ID = id
						}
						if t, ok := m["serverType"].(string); ok {
							si.ServerType = t
						}
						if h, ok := m["host"].(string); ok {
							si.Host = h
						}
						if p, ok := m["port"].(float64); ok {
							si.Port = int(p)
						}
						servers = append(servers, si)
					}
				}
			}
		}
	}

	return servers, nil
}

func (a *AdminServer) listServers(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	byType := make(map[string][]ServerStat)
	for _, s := range a.servers {
		byType[s.Type] = append(byType[s.Type], *s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(byType)
}

func (a *AdminServer) getStats(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	totalServers := len(a.servers)
	var totalClients int
	for _, s := range a.servers {
		totalClients += s.Clients
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"servers": totalServers,
		"clients": totalClients,
	})
}

func (a *AdminServer) getConnections(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var totalClients int
	for _, s := range a.servers {
		totalClients += s.Clients
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"count": totalClients})
}

func (a *AdminServer) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "public/index.html")
}
