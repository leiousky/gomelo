package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const version = "1.5.5"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "-v", "--version":
		fmt.Printf("gomelo version %s\n", version)
	case "-h", "--help":
		printUsage()
	case "init":
		handleInit(args)
	case "start":
		handleStart(args)
	case "build":
		handleBuild(args)
	case "routes":
		handleRoutes(args)
	case "list":
		handleList(args)
	case "doc":
		handleDoc(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func handleRoutes(args []string) {
	basePath := "servers"
	if len(args) > 0 {
		basePath = args[0]
	}

	cmd := exec.Command("go", "run", "./cmd/codegen", "--list", basePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running codegen: %v\n", err)
		os.Exit(1)
	}
}

func handleList(args []string) {
	masterAddr := "127.0.0.1:3006"
	for i, arg := range args {
		if arg == "--master" && i+1 < len(args) {
			masterAddr = args[i+1]
			break
		}
	}

	resp, err := fetchServers(masterAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching servers: %v\n", err)
		os.Exit(1)
	}

	printServerTable(resp.Servers)
}

func handleDoc(args []string) {
	basePath := "servers"
	if len(args) > 0 {
		basePath = args[0]
	}

	cmd := exec.Command("go", "run", "./cmd/doc", basePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running doc: %v\n", err)
		os.Exit(1)
	}
}

type serverListResp struct {
	Servers []serverInfo `json:"servers"`
}

type serverInfo struct {
	ID         string `json:"id"`
	ServerType string `json:"serverType"`
	State      int    `json:"state"`
	Count      int    `json:"count"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Frontend   bool   `json:"frontend"`
	RegisterAt int64  `json:"registerAt"`
}

func fetchServers(masterAddr string) (*serverListResp, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	url := fmt.Sprintf("http://%s/api/servers", masterAddr)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result serverListResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func printServerTable(servers []serverInfo) {
	if len(servers) == 0 {
		fmt.Println("No servers found.")
		return
	}

	fmt.Println()
	fmt.Printf("%-18s %-12s %-22s %-8s %-8s\n",
		"SERVER ID", "TYPE", "HOST:PORT", "CLIENTS", "STATE")
	fmt.Println(strings.Repeat("-", 72))

	for _, s := range servers {
		addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
		stateStr := "offline"
		if s.State == 0 {
			stateStr = "online"
		}
		fmt.Printf("%-18s %-12s %-22s %-8d %-8s\n",
			s.ID, s.ServerType, addr, s.Count, stateStr)
	}
	fmt.Println()
}

func formatMemory(bytes int64) string {
	if bytes == 0 {
		return "-"
	}
	mb := bytes / (1024 * 1024)
	if mb < 1024 {
		return fmt.Sprintf("%dMB", mb)
	}
	gb := mb / 1024
	mb = mb % 1024
	return fmt.Sprintf("%.1fGB", float64(gb)+float64(mb)/1024)
}

func formatUptime(seconds int64) string {
	if seconds == 0 {
		return "-"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func printUsage() {
	fmt.Print(`Usage: gomelo [command] [options]

Commands:
  init <name>    Initialize a new gomelo project
  build          Build the application to binary
  start [dir]    Start the application (starts Master which auto-starts all configured servers)
  routes         List all registered routes
  list           List all running servers
  -v, --version  Show version
  -h, --help     Show this help

Start Options:
  --dir <path>         Specify server directory (default: current directory)
  --server-type <type> Set GOMELO_SERVER_TYPE (default: master)
  --production         Start with production environment
  --dev                Use go run instead of compiled binary

Examples:
  gomelo init
  cd game-project && gomelo build
  gomelo start
  gomelo start --production
`)
}

func handleInit(args []string) {
	name := "game-project"
	if len(args) > 0 {
		name = args[0]
	}

	dir := filepath.Join(".", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	files := map[string]string{
		"game-server/main.go": mainGoTemplate,

		"game-server/go.mod": goModTemplate(name),

		"game-server/config/servers.json": serversJsonTemplate,
		"game-server/config/log.json":     logConfigTemplate,
		"game-server/config/master.json":  masterConfigTemplate,

		"game-server/servers/connector/handler/entry.go":    connectorHandlerTemplate,
		"game-server/servers/connector/remote/connector.go": connectorRemoteTemplate,
		"game-server/servers/connector/filter/time.go":      filterTemplate("connector"),
		"game-server/servers/connector/cron/auto.go":        cronTemplate("connector"),

		"game-server/servers/gate/handler/gate.go": gateHandlerTemplate,
		"game-server/servers/gate/remote/gate.go":  gateRemoteTemplate,
		"game-server/servers/gate/filter/time.go":  filterTemplate("gate"),

		"web-server/admin/main.go":       adminTemplate(),
		"web-server/public/index.html":   webIndexTemplate,
		"web-server/public/js/client.js": webClientTemplate,

		"game-server/logs/.gitkeep": "",
		".gitignore":                gitignoreTemplate,
	}

	for filename, content := range files {
		path := filepath.Join(dir, filename)
		parent := filepath.Dir(path)
		if err := os.MkdirAll(parent, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
			os.Exit(1)
		}
		if content == "" {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", filename, err)
			os.Exit(1)
		}
	}

	fmt.Printf("Project '%s' created successfully!\n\n", name)
	printDirStructure()

	// Run go mod tidy to resolve dependencies
	gameDir := filepath.Join(dir, "game-server")
	fmt.Printf("Resolving dependencies (go mod tidy)...\n")
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = gameDir
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: go mod tidy failed (you may need to run it manually): %v\n", err)
	}
}

func printDirStructure() {
	fmt.Println(`  game-project/`)
	fmt.Println(`  ├── game-server/`)
	fmt.Println(`  │   ├── main.go`)
	fmt.Println(`  │   ├── go.mod`)
	fmt.Println(`  │   ├── config/`)
	fmt.Println(`  │   │   ├── servers.json`)
	fmt.Println(`  │   │   ├── log.json`)
	fmt.Println(`  │   │   └── master.json`)
	fmt.Println(`  │   ├── servers/`)
	fmt.Println(`  │   │   ├── connector/`)
	fmt.Println(`  │   │   │   ├── handler/`)
	fmt.Println(`  │   │   │   ├── remote/`)
	fmt.Println(`  │   │   │   ├── filter/`)
	fmt.Println(`  │   │   │   └── cron/`)
	fmt.Println(`  │   │   ├── gate/`)
	fmt.Println(`  │   └── cmd/`)
	fmt.Println(`  │       └── admin/`)
	fmt.Println(`  │           └── main.go`)
	fmt.Println(`  ├── web-server/`)
	fmt.Println(`  │   └── public/`)
	fmt.Println(`  │       ├── index.html`)
	fmt.Println(`  │       └── js/`)
	fmt.Println(`  │           └── client.js`)
	fmt.Println(`  └── logs/`)
}

func handleBuild(args []string) {
	dir := "."
	output := ""
	for i, arg := range args {
		if arg == "-o" && i+1 < len(args) {
			output = args[i+1]
			args = append(args[:i], args[i+2:]...)
			break
		}
	}
	if len(args) > 0 {
		dir = args[0]
	}

	mainPath := filepath.Join(dir, "game-server", "main.go")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		mainPath = filepath.Join(dir, "main.go")
		if _, err := os.Stat(mainPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: main.go not found in %s\n", dir)
			os.Exit(1)
		}
	}

	serverDir := filepath.Dir(mainPath)
	if output == "" {
		output = filepath.Join(serverDir, "server")
		if runtime.GOOS == "windows" {
			output += ".exe"
		}
	}

	fmt.Printf("Building gomelo server to %s...\n", output)
	cmd := exec.Command("go", "build", "-o", output, ".")
	cmd.Dir = serverDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error building server: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Build successful: %s\n", output)
}

func handleStart(args []string) {
	dir := "."
	var env string
	var serverType string
	devMode := false
	dirSet := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dir" && i+1 < len(args):
			dir = args[i+1]
			dirSet = true
			i++
		case arg == "--server-type" && i+1 < len(args):
			serverType = args[i+1]
			i++
		case arg == "--production":
			env = "production"
		case arg == "--dev":
			devMode = true
		case !strings.HasPrefix(arg, "-") && !dirSet:
			dir = arg
			dirSet = true
		}
	}

	mainPath := filepath.Join(dir, "game-server", "main.go")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		mainPath = filepath.Join(dir, "main.go")
		if _, err := os.Stat(mainPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: main.go not found in %s\n", dir)
			os.Exit(1)
		}
	}

	serverDir := filepath.Dir(mainPath)

	fmt.Printf("Ensuring dependencies...\n")
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = serverDir
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running go mod tidy: %v\n", err)
		os.Exit(1)
	}

	cmdEnv := os.Environ()
	if env != "" {
		cmdEnv = append(cmdEnv, "GOMELO_ENV="+env)
	}
	if serverType != "" {
		cmdEnv = append(cmdEnv, "GOMELO_SERVER_TYPE="+serverType)
	} else {
		cmdEnv = append(cmdEnv, "GOMELO_SERVER_TYPE=master")
	}

	if devMode {
		fmt.Printf("Starting gomelo (dev mode) from %s...\n", serverDir)
		cmd := exec.Command("go", "run", ".")
		cmd.Dir = serverDir
		cmd.Env = cmdEnv
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting gomelo: %v\n", err)
			os.Exit(1)
		}
		return
	}

	binaryName := "server"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(serverDir, binaryName)

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		fmt.Printf("Binary not found, building...\n")
		buildCmd := exec.Command("go", "build", "-o", binaryName, ".")
		buildCmd.Dir = serverDir
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error building server (use --dev for go run): %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Starting gomelo from %s...\n", serverDir)
	runCmd := exec.Command(binaryPath)
	runCmd.Dir = serverDir
	runCmd.Env = cmdEnv
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Stdin = os.Stdin
	if err := runCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting gomelo: %v\n", err)
		os.Exit(1)
	}
}

// autoSelectServerID reads servers.json and returns the first server ID for the given type.
// Currently unused; kept for potential use by external tooling.
func autoSelectServerID(configPath, serverType, env string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read servers.json failed: %w", err)
	}

	if env == "" {
		env = "development"
	}

	var cfg struct {
		Development map[string][]map[string]any `json:"development"`
		Production  map[string][]map[string]any `json:"production"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse servers.json failed: %w", err)
	}

	var servers []map[string]any
	switch env {
	case "production":
		servers = cfg.Production[serverType]
	default:
		servers = cfg.Development[serverType]
	}

	if len(servers) == 0 {
		return "", fmt.Errorf("no server found for type: %s", serverType)
	}

	if id, ok := servers[0]["id"].(string); ok {
		return id, nil
	}
	return "", fmt.Errorf("invalid server config for type: %s", serverType)
}

var mainGoTemplate = `package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/chuhongliang/gomelo/connector"
	"github.com/chuhongliang/gomelo/lib"
	"github.com/chuhongliang/gomelo/loader"
	"github.com/chuhongliang/gomelo/master"
)

func main() {
	serverType := os.Getenv("GOMELO_SERVER_TYPE")

	if serverType == "master" {
		startMaster()
	} else {
		startGameServer()
	}
}

func startMaster() {
	env := os.Getenv("GOMELO_ENV")
	if env == "" {
		env = "development"
	}

	masterData, err := os.ReadFile("config/master.json")
	if err != nil {
		fmt.Printf("Load master.json failed: %v\n", err)
		os.Exit(1)
	}

	var masterConfig map[string]map[string]any
	if err := json.Unmarshal(masterData, &masterConfig); err != nil {
		fmt.Printf("Parse master.json failed: %v\n", err)
		os.Exit(1)
	}

	masterCfg, ok := masterConfig[env]
	if !ok {
		fmt.Printf("Env %s not found in master.json\n", env)
		os.Exit(1)
	}

	host, _ := masterCfg["host"].(string)
	port, _ := masterCfg["port"].(float64)
	masterAddr := fmt.Sprintf("%s:%d", host, int(port))

	fmt.Printf("Starting Master (env: %s, addr: %s)...\n", env, masterAddr)

	masterServer := master.New()
	masterServer.EnableAdmin(":3006")

	if err := masterServer.Start(masterData); err != nil {
		fmt.Printf("Master start failed: %v\n", err)
		os.Exit(1)
	}

	serversData, err := os.ReadFile("config/servers.json")
	if err != nil {
		fmt.Printf("Load servers.json failed: %v\n", err)
		os.Exit(1)
	}

	var serversConfig map[string][]map[string]any
	if err := json.Unmarshal(serversData, &serversConfig); err != nil {
		fmt.Printf("Parse servers.json failed: %v\n", err)
		os.Exit(1)
	}

	envServers, ok := serversConfig[env]
	if !ok {
		fmt.Printf("Env %s not found in servers.json\n", env)
		os.Exit(1)
	}

	var allServers []map[string]any
	serverCfgs := make(map[string]any)

	for _, srv := range envServers {
		srv["masterHost"] = masterAddr
		allServers = append(allServers, srv)

		serverType, _ := srv["serverType"].(string)
		if serverType != "" && serverCfgs[serverType] == nil {
			serverCfgs[serverType] = map[string]any{
				"path":      "",
				"instances": 1,
			}
		}
	}

	if len(serverCfgs) > 0 {
		masterServer.SetServerCfgs(serverCfgs)
	}

	if len(allServers) > 0 {
		masterServer.StartServers(allServers)
	}

	fmt.Println("Master is running...")
	masterServer.Wait()
}

func startGameServer() {
	flag.Parse()

	app := lib.NewApp(
		lib.WithEnv(os.Getenv("GOMELO_ENV")),
		lib.WithServerID(os.Getenv("GOMELO_SERVER_ID")),
	)

	app.Setup("./config")

	// Register with Master if master address is configured (set by Master auto-start)
	masterAddr := os.Getenv("GOMELO_MASTER_HOST")
	if masterAddr != "" {
		serverID := app.GetServerId()
		serverType := app.GetServerType()
		host := app.GetHost()
		port := app.GetPort()
		frontend := app.IsFrontend()

		mc, err := master.NewClientWithConfig(masterAddr, serverID, serverType, host, port, frontend)
		if err != nil {
			fmt.Printf("Connect to master failed: %v\n", err)
		} else {
			if err := mc.Register(); err != nil {
				fmt.Printf("Register with master failed: %v\n", err)
			} else {
				fmt.Printf("Registered with master at %s\n", masterAddr)
				go func() {
					ticker := time.NewTicker(30 * time.Second)
					defer ticker.Stop()
					for range ticker.C {
						if err := mc.Heartbeat(); err != nil {
							fmt.Printf("Heartbeat failed: %v\n", err)
							return
						}
					}
				}()
				defer mc.Unregister()
				defer mc.Close()
			}
		}
	}

	if app.IsFrontend() {
		conn := connector.NewServer(&connector.ServerOptions{
			Host: app.GetHost(),
			Port: app.GetPort(),
		})
		app.Register("connector", conn)
	}

	l := loader.NewLoader("servers")
	l.SetApp(app)
	loader.SetGlobalLoader(l)
	if err := l.Load(); err != nil {
		fmt.Printf("Load servers failed: %v\n", err)
		os.Exit(1)
	}

	if err := app.Start(); err != nil {
		fmt.Printf("Start failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Server %s started on %s:%d\n", app.GetServerType(), app.GetHost(), app.GetPort())

	app.Wait()
}
`

func goModTemplate(name string) string {
	return fmt.Sprintf(`module %s

go 1.21

require github.com/chuhongliang/gomelo v1.5.4

// If you are developing gomelo locally, uncomment and adjust the path:
// replace github.com/chuhongliang/gomelo => /your/local/gomelo/path
`, name)
}

var serversJsonTemplate = `{
  "development": [
    {"id": "connector-1", "serverType": "connector", "host": "127.0.0.1", "port": 3010, "frontend": true},
    {"id": "gate-1", "serverType": "gate", "host": "127.0.0.1", "port": 3011}
  ],
  "production": [
    {"id": "connector-1", "serverType": "connector", "host": "127.0.0.1", "port": 3010, "frontend": true},
    {"id": "gate-1", "serverType": "gate", "host": "127.0.0.1", "port": 3011}
  ]
}
`

var gitignoreTemplate = `# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
server
server.exe

# Logs
logs/
*.log

# Go
vendor/

# IDE
.idea/
.vscode/
*.swp
*.swo
`

var logConfigTemplate = `{
  "level": "info",
  "path": "./logs",
  "console": true,
  "format": "json",
  "rotate": {
    "enabled": true,
    "maxSize": 104857600,
    "maxFiles": 10
  }
}
`

var masterConfigTemplate = `{
  "development": {
    "id": "master-server-1",
    "host": "127.0.0.1",
    "port": 3005
  },
  "production": {
    "id": "master-server-1",
    "host": "0.0.0.0",
    "port": 3005
  }
}
`

// masterMainTemplate is an alternative standalone master entrypoint (not used by handleInit).
// The mainGoTemplate includes master startup logic within a single main.go via GOMELO_SERVER_TYPE.
var masterMainTemplate = `package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/chuhongliang/gomelo/master"
)

func main() {
	env := os.Getenv("GOMELO_ENV")
	if env == "" {
		env = "development"
	}

	if len(os.Args) > 1 && os.Args[1] == "--production" {
		env = "production"
	}

	masterData, err := os.ReadFile("config/master.json")
	if err != nil {
		fmt.Printf("Load master.json failed: %v\n", err)
		os.Exit(1)
	}

	var masterConfig map[string]map[string]any
	if err := json.Unmarshal(masterData, &masterConfig); err != nil {
		fmt.Printf("Parse master.json failed: %v\n", err)
		os.Exit(1)
	}

	masterCfg, ok := masterConfig[env]
	if !ok {
		fmt.Printf("Env %s not found in master.json\n", env)
		os.Exit(1)
	}

	host, _ := masterCfg["host"].(string)
	port, _ := masterCfg["port"].(float64)
	masterAddr := fmt.Sprintf("%s:%d", host, int(port))

	fmt.Printf("Starting Master (env: %s, addr: %s)...\n", env, masterAddr)

	masterServer := master.New(masterAddr)
	masterServer.EnableAdmin(":3006")

	if err := masterServer.Start(); err != nil {
		fmt.Printf("Master start failed: %v\n", err)
		os.Exit(1)
	}

	serversData, err := os.ReadFile("config/servers.json")
	if err != nil {
		fmt.Printf("Load servers.json failed: %v\n", err)
		os.Exit(1)
	}

	var serversConfig map[string][]map[string]any
	if err := json.Unmarshal(serversData, &serversConfig); err != nil {
		fmt.Printf("Parse servers.json failed: %v\n", err)
		os.Exit(1)
	}

	envServers, ok := serversConfig[env]
	if !ok {
		fmt.Printf("Env %s not found in servers.json\n", env)
		os.Exit(1)
	}

	var allServers []map[string]any
	serverCfgs := make(map[string]any)

	for _, srv := range envServers {
		srv["masterHost"] = masterAddr
		allServers = append(allServers, srv)

		serverType, _ := srv["serverType"].(string)
		if serverType != "" && serverCfgs[serverType] == nil {
			serverCfgs[serverType] = map[string]any{
				"path":      "",
				"instances": 1,
			}
		}
	}

	if len(serverCfgs) > 0 {
		masterServer.SetServerCfgs(serverCfgs)
	}

	if len(allServers) > 0 {
		masterServer.StartServers(allServers)
	}

	fmt.Println("Master is running...")
	masterServer.Wait()
}
`

var connectorHandlerTemplate = "package handler\n\nimport (\n\t\"github.com/chuhongliang/gomelo/lib\"\n)\n\ntype EntryHandler struct {\n\tapp *lib.App\n}\n\nfunc (h *EntryHandler) Init(app *lib.App) { h.app = app }\n\nfunc (h *EntryHandler) Entry(ctx *lib.Context) {\n\tvar req struct {\n\t\tName string `json:\"name\"`\n\t}\n\tctx.Bind(&req)\n\tctx.Response(map[string]any{\"msg\": \"hello \" + req.Name})\n}\n\nfunc (h *EntryHandler) GetFriends(ctx *lib.Context) {\n\tctx.ResponseOK(map[string]any{\"friends\": []string{}})\n}\n\nfunc (h *EntryHandler) Logout(ctx *lib.Context) {\n\tctx.ResponseOK(map[string]any{})\n}\n"

var connectorRemoteTemplate = `package remote

import (
	"context"
	"github.com/chuhongliang/gomelo/lib"
)

type ConnectorRemote struct {
	app *lib.App
}

func (r *ConnectorRemote) Init(app *lib.App) { r.app = app }

func (r *ConnectorRemote) AddUser(ctx context.Context, args struct {
	UserID string
}) (any, error) {
	return map[string]any{"code": 0, "user": args.UserID}, nil
}

func (r *ConnectorRemote) RemoveUser(ctx context.Context, args struct {
	UserID string
}) (any, error) {
	return map[string]any{"code": 0}, nil
}
`

func cronTemplate(serverType string) string {
	title := strings.ToTitle(serverType)
	return fmt.Sprintf(`package cron

import (
	"context"
	"github.com/chuhongliang/gomelo/lib"
)

type %sCron struct {
	app *lib.App
}

func (c *%sCron) Init(app *lib.App) { c.app = app }

func (c *%sCron) Cleanup(ctx context.Context) {
	return nil
}
`, title, title, title)
}

func filterTemplate(serverType string) string {
	title := strings.ToTitle(serverType)
	return fmt.Sprintf(`package filter

import (
	"time"
	"github.com/chuhongliang/gomelo/lib"
)

type %sFilter struct{}

func (f *%sFilter) Name() string { return "%s-filter" }

func (f *%sFilter) Process(ctx *lib.Context) bool {
	ctx.Set("startTime", time.Now())
	return true
}

func (f *%sFilter) After(ctx *lib.Context) {
}
`, title, title, serverType, title, title)
}

var gateHandlerTemplate = `package handler

import (
	"github.com/chuhongliang/gomelo/lib"
)

type GateHandler struct {
	app *lib.App
}

func (h *GateHandler) Init(app *lib.App) { h.app = app }

func (h *GateHandler) Entry(ctx *lib.Context) {
	ctx.ResponseOK(map[string]any{"code": 0})
}
`

var gateRemoteTemplate = `package remote

import (
	"context"
	"github.com/chuhongliang/gomelo/lib"
)

type GateRemote struct {
	app *lib.App
}

func (r *GateRemote) Init(app *lib.App) { r.app = app }

func (r *GateRemote) QueryRoute(ctx context.Context, args struct {
	ServerType string
}) (any, error) {
	return map[string]any{"code": 0, "serverType": args.ServerType}, nil
}
`

var webIndexTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>gomelo Admin</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .server { border: 1px solid #ccc; padding: 10px; margin: 10px 0; }
        .online { color: green; }
        .offline { color: red; }
    </style>
</head>
<body>
    <h1>gomelo Game Server Monitor</h1>
    <div id="servers">Loading...</div>
    <script>
        async function loadServers() {
            try {
                var res = await fetch('/api/servers');
                var data = await res.json();
                document.getElementById('servers').innerHTML = '';
                for (var type in data) {
                    var html = '<h3>' + type + '</h3>';
                    for (var s of data[type]) {
                        html += '<div class="server">';
                        html += '<span class="' + (s.online ? 'online' : 'offline') + '">●</span> ';
                        html += s.id + ' - ' + s.host + ':' + s.port;
                        html += '</div>';
                    }
                    document.getElementById('servers').innerHTML += html;
                }
            } catch (e) {
                document.getElementById('servers').innerText = 'Error loading servers';
            }
        }
        loadServers();
        setInterval(loadServers, 5000);
    </script>
</body>
</html>
`

var webClientTemplate = `// gomelo Admin API Client

class AdminClient {
    constructor(baseUrl) {
        this.baseUrl = baseUrl || '';
    }

    async getServers() {
        var res = await fetch(this.baseUrl + '/api/servers');
        return await res.json();
    }

    async getStats() {
        var res = await fetch(this.baseUrl + '/api/stats');
        return await res.json();
    }

    async getConnections() {
        var res = await fetch(this.baseUrl + '/api/connections');
        return await res.json();
    }
}

window.gomeloAdmin = { AdminClient: AdminClient };
`

func adminTemplate() string {
	qt := "\""
	return "package main\n\nimport (\n\t\"encoding/binary\"\n\t\"encoding/json\"\n\t\"flag\"\n\t\"fmt\"\n\t\"io\"\n\t\"net\"\n\t\"net/http\"\n\t\"sync\"\n\t\"time\"\n)\n\nvar httpAddr = flag.String(\"http\", \":3006\", \"HTTP listen address\")\nvar masterAddr = flag.String(\"master\", \"127.0.0.1:3005\", \"Master server address\")\n\ntype AdminServer struct {\n\tmasterAddr string\n\tservers    map[string]*ServerStat\n\tmu         sync.RWMutex\n\tmux        *http.ServeMux\n\tserver     *http.Server\n}\n\ntype ServerStat struct {\n\tID      string " + qt + "json:\"id\"" + qt + "\n\tType    string " + qt + "json:\"type\"" + qt + "\n\tState   string " + qt + "json:\"state\"" + qt + "\n\tClients int    " + qt + "json:\"clients\"" + qt + "\n\tHost    string " + qt + "json:\"host\"" + qt + "\n\tPort    int    " + qt + "json:\"port\"" + qt + "\n}\n\ntype masterMessage struct {\n\tType string          " + qt + "json:\"type\"" + qt + "\n\tData json.RawMessage " + qt + "json:\"data\"" + qt + "\n}\n\ntype serverInfo struct {\n\tID         string " + qt + "json:\"id\"" + qt + "\n\tServerType string " + qt + "json:\"serverType\"" + qt + "\n\tHost       string " + qt + "json:\"host\"" + qt + "\n\tPort       int    " + qt + "json:\"port\"" + qt + "\n\tFrontend   bool   " + qt + "json:\"frontend\"" + qt + "\n\tState      int    " + qt + "json:\"state\"" + qt + "\n\tCount      int    " + qt + "json:\"count\"" + qt + "\n}\n\nfunc main() {\n\tflag.Parse()\n\n\tadmin := &AdminServer{\n\t\tmasterAddr: *masterAddr,\n\t\tservers:    make(map[string]*ServerStat),\n\t}\n\n\tadmin.mux = http.NewServeMux()\n\tadmin.mux.HandleFunc(\"/api/servers\", admin.listServers)\n\tadmin.mux.HandleFunc(\"/api/stats\", admin.getStats)\n\tadmin.mux.HandleFunc(\"/api/connections\", admin.getConnections)\n\tadmin.mux.HandleFunc(\"/\", admin.index)\n\n\tadmin.server = &http.Server{\n\t\tAddr:    *httpAddr,\n\t\tHandler: admin.mux,\n\t}\n\n\tgo admin.watchMaster()\n\n\tfmt.Printf(\"Admin server starting on %s\\n\", *httpAddr)\n\tadmin.server.ListenAndServe()\n}\n\nfunc (a *AdminServer) watchMaster() {\n\tticker := time.NewTicker(5 * time.Second)\n\tdefer ticker.Stop()\n\n\tfor range ticker.C {\n\t\tservers, err := a.queryMasterServers()\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\n\t\ta.mu.Lock()\n\t\ta.servers = make(map[string]*ServerStat)\n\t\tfor _, s := range servers {\n\t\t\ta.servers[s.ID] = &ServerStat{\n\t\t\t\tID:      s.ID,\n\t\t\t\tType:    s.ServerType,\n\t\t\t\tState:   \"online\",\n\t\t\t\tClients: s.Count,\n\t\t\t\tHost:    s.Host,\n\t\t\t\tPort:    s.Port,\n\t\t\t}\n\t\t}\n\t\ta.mu.Unlock()\n\t}\n}\n\nfunc (a *AdminServer) queryMasterServers() ([]serverInfo, error) {\n\tconn, err := net.DialTimeout(\"tcp\", a.masterAddr, 5*time.Second)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tdefer conn.Close()\n\n\treq := masterMessage{Type: \"query\"}\n\tdata, _ := json.Marshal(req)\n\tlenBuf := make([]byte, 4)\n\tbinary.BigEndian.PutUint32(lenBuf, uint32(len(data)))\n\tconn.Write(lenBuf)\n\tconn.Write(data)\n\n\theader := make([]byte, 4)\n\tconn.SetReadDeadline(time.Now().Add(10 * time.Second))\n\tif _, err := io.ReadFull(conn, header); err != nil {\n\t\treturn nil, err\n\t}\n\tlength := binary.BigEndian.Uint32(header)\n\tresp := make([]byte, length)\n\tif _, err := io.ReadFull(conn, resp); err != nil {\n\t\treturn nil, err\n\t}\n\n\tvar result map[string]any\n\tif err := json.Unmarshal(resp, &result); err != nil {\n\t\treturn nil, err\n\t}\n\n\tvar servers []serverInfo\n\tif serversRaw, ok := result[\"servers\"].(map[string]any); ok {\n\t\tfor _, val := range serversRaw {\n\t\t\tif arr, ok := val.([]any); ok {\n\t\t\t\tfor _, item := range arr {\n\t\t\t\t\tif m, ok := item.(map[string]any); ok {\n\t\t\t\t\t\tsi := serverInfo{}\n\t\t\t\t\t\tif id, ok := m[\"id\"].(string); ok {\n\t\t\t\t\t\t\tsi.ID = id\n\t\t\t\t\t\t}\n\t\t\t\t\t\tif t, ok := m[\"serverType\"].(string); ok {\n\t\t\t\t\t\t\tsi.ServerType = t\n\t\t\t\t\t\t}\n\t\t\t\t\t\tif h, ok := m[\"host\"].(string); ok {\n\t\t\t\t\t\t\tsi.Host = h\n\t\t\t\t\t\t}\n\t\t\t\t\t\tif p, ok := m[\"port\"].(float64); ok {\n\t\t\t\t\t\t\tsi.Port = int(p)\n\t\t\t\t\t\t}\n\t\t\t\t\t\tservers = append(servers, si)\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n\n\treturn servers, nil\n}\n\nfunc (a *AdminServer) listServers(w http.ResponseWriter, r *http.Request) {\n\ta.mu.RLock()\n\tdefer a.mu.RUnlock()\n\n\tbyType := make(map[string][]ServerStat)\n\tfor _, s := range a.servers {\n\t\tbyType[s.Type] = append(byType[s.Type], *s)\n\t}\n\n\tw.Header().Set(\"Content-Type\", \"application/json\")\n\tjson.NewEncoder(w).Encode(byType)\n}\n\nfunc (a *AdminServer) getStats(w http.ResponseWriter, r *http.Request) {\n\ta.mu.RLock()\n\tdefer a.mu.RUnlock()\n\n\ttotalServers := len(a.servers)\n\tvar totalClients int\n\tfor _, s := range a.servers {\n\t\ttotalClients += s.Clients\n\t}\n\n\tw.Header().Set(\"Content-Type\", \"application/json\")\n\tjson.NewEncoder(w).Encode(map[string]any{\n\t\t\"servers\": totalServers,\n\t\t\"clients\": totalClients,\n\t})\n}\n\nfunc (a *AdminServer) getConnections(w http.ResponseWriter, r *http.Request) {\n\ta.mu.RLock()\n\tdefer a.mu.RUnlock()\n\n\tvar totalClients int\n\tfor _, s := range a.servers {\n\t\ttotalClients += s.Clients\n\t}\n\n\tw.Header().Set(\"Content-Type\", \"application/json\")\n\tjson.NewEncoder(w).Encode(map[string]any{\"count\": totalClients})\n}\n\nfunc (a *AdminServer) index(w http.ResponseWriter, r *http.Request) {\n\tif r.URL.Path != \"/\" {\n\t\thttp.NotFound(w, r)\n\t\treturn\n\t}\n\thttp.ServeFile(w, r, \"public/index.html\")\n}\n"
}

var timeFilterTemplate = `package filter

import (
	"time"
	"github.com/chuhongliang/gomelo/lib"
)

type TimeFilter struct{}

func (f *TimeFilter) Name() string { return "time" }

func (f *TimeFilter) Process(ctx *lib.Context) bool {
	ctx.Set("startTime", time.Now())
	return true
}

func (f *TimeFilter) After(ctx *lib.Context) {
}
`
