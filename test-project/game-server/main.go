package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

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

	app.Configure(func(s *lib.Server) {
		if s.Frontend() {
			conn := connector.NewServer(&connector.ServerOptions{
				Host: s.Host(),
				Port: s.Port(),
			})
			app.Register("connector", conn)
		}
	})

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
