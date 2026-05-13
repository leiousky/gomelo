package gomelo

import (
	"io"
	"time"

	"github.com/chuhongliang/gomelo/broadcast"
	"github.com/chuhongliang/gomelo/codec"
	_ "github.com/chuhongliang/gomelo/component" // for side-effect init
	"github.com/chuhongliang/gomelo/config"
	"github.com/chuhongliang/gomelo/connector"
	"github.com/chuhongliang/gomelo/filter"
	"github.com/chuhongliang/gomelo/forward"
	"github.com/chuhongliang/gomelo/lib"
	"github.com/chuhongliang/gomelo/loader"
	"github.com/chuhongliang/gomelo/logger"
	"github.com/chuhongliang/gomelo/master"
	"github.com/chuhongliang/gomelo/pool"
	"github.com/chuhongliang/gomelo/plugin"
	"github.com/chuhongliang/gomelo/route"
	"github.com/chuhongliang/gomelo/rpc"
	_ "github.com/chuhongliang/gomelo/scheduler" // for side-effect init
	"github.com/chuhongliang/gomelo/selector"
	"github.com/chuhongliang/gomelo/server_registry"
)

const Version = "1.5.5"

type (
	Component       = lib.Component
	Context         = lib.Context
	Session         = lib.Session
	Message         = lib.Message
	MessageType     = lib.MessageType
	Server          = lib.Server
	HandlerFunc     = lib.HandlerFunc
	Middleware      = lib.Middleware
	RouteHandler    = lib.RouteHandler
	EventEmitter    = lib.EventEmitter
	ConnectorServer = connector.Server
)

type ConnectorServerOptions = connector.ServerOptions

func NewConnectorServer(opts *connector.ServerOptions) *connector.Server {
	return connector.NewServer(opts)
}

func NewApp(opts ...lib.AppOption) *lib.App    { return lib.NewApp(opts...) }
func WithEnv(env string) lib.AppOption         { return lib.WithEnv(env) }
func WithHost(host string) lib.AppOption       { return lib.WithHost(host) }
func WithPort(port int) lib.AppOption          { return lib.WithPort(port) }
func WithServerID(id string) lib.AppOption     { return lib.WithServerID(id) }
func WithMasterAddr(addr string) lib.AppOption { return lib.WithMasterAddr(addr) }

func NewEventEmitter() *lib.EventEmitter { return lib.NewEventEmitter() }

func NewSession() *lib.Session             { return lib.NewSession() }
func NewContext(app *lib.App) *lib.Context { return lib.NewContext(app) }

type (
	ServerInfo = server_registry.ServerInfo
	Selector   = selector.Selector
	Level      = logger.Level
)

func NewSelector(reg server_registry.ServerRegistry) selector.Selector { return selector.New(reg) }
func NewLoadBalancer() *selector.LoadBalancer                          { return selector.NewLoadBalancer() }

type Filter = filter.Filter
type FilterChain = filter.FilterChain
type FilterManager = filter.FilterManager

func NewFilterChain() *filter.FilterChain     { return filter.NewFilterChain() }
func NewFilterManager() *filter.FilterManager { return filter.NewFilterManager() }

func LoadConfig(path string) (*config.Config, error)          { return config.Load(path) }
func LoadConfigStrict(path string) (*config.Loader, error)    { return config.LoadStrict(path) }
func MergeConfig(base, overlay *config.Config) *config.Config { return config.Merge(base, overlay) }
func DefaultConfig() *config.Config                           { return config.Default() }

func NewMasterServer() master.MasterServer { return master.New() }

type RPCClientManager = lib.RPCClientManager

func NewRPCClientManager(registry server_registry.ServerRegistry, sel selector.Selector, opts *rpc.ClientOptions) (RPCClientManager, error) {
	return rpc.NewClientManager(registry, sel, opts)
}
func NewMasterClient(addr, id, serverType string) (master.MasterClient, error) {
	return master.NewClient(addr, id, serverType)
}

type ServersConfig = lib.ServersConfig

func LoadServersConfig(path, env string) ([]map[string]any, error) {
	return lib.LoadServersConfig(path, env)
}

type (
	Codec         = codec.Codec
	Compressor    = route.Compressor
	RouteTable    = route.RouterTable
	RouteManager  = route.RouteManager
	Plugin        = plugin.Plugin
	PluginManager = plugin.PluginManager
	HookManager   = plugin.HookManager
)

func NewProtobufCodec() *codec.ProtobufCodec { return codec.NewProtobufCodec() }
func NewJSONCodec() *codec.JSONCodec         { return codec.NewJSONCodec() }
func NewCodec(t codec.CodecType) codec.Codec { return codec.NewCodec(t) }

func NewCompressor() *route.Compressor     { return route.NewCompressor() }
func NewRouterTable() *route.RouterTable   { return route.NewRouterTable() }
func NewRouteManager() *route.RouteManager { return route.NewRouteManager() }

func NewPluginManager() *plugin.PluginManager { return plugin.NewPluginManager() }
func NewHookManager() *plugin.HookManager     { return plugin.NewHookManager() }

type (
	Pool             = pool.Pool
	WorkerPool       = pool.WorkerPool
	BroadcastService = broadcast.BroadcastService
	BroadcastManager = broadcast.BroadcastManager
)

func NewPool(factory func() (any, error), maxConns, minConns int, maxWait, idleTimeout time.Duration) pool.Pool {
	return pool.NewPool(factory, maxConns, minConns, maxWait, idleTimeout)
}
func NewWorkerPool(workers, queueSize int) *pool.WorkerPool {
	return pool.NewWorkerPool(workers, queueSize)
}
func NewBroadcast(route string, opts ...broadcast.BroadcastOption) broadcast.BroadcastService {
	return broadcast.NewBroadcast(route, opts...)
}
func NewBroadcastManager() *broadcast.BroadcastManager { return broadcast.NewBroadcastManager() }

type (
	MessageForwarder = forward.MessageForwarder
	ForwardManager   = forward.ForwardManager
)

func NewForwarder(app *lib.App, sel selector.Selector) forward.MessageForwarder {
	return forward.NewForwarder(app, sel)
}
func NewForwardManager(app *lib.App, sel selector.Selector) *forward.ForwardManager {
	return forward.NewForwardManager(app, sel)
}

type Logger = logger.Logger

func NewLogger(output io.Writer, opts ...logger.Option) *logger.Logger {
	return logger.New(output, opts...)
}

func WithOutput(w io.Writer) logger.Option       { return logger.WithOutput(w) }
func WithPrefix(prefix string) logger.Option     { return logger.WithPrefix(prefix) }
func WithLevel(level logger.Level) logger.Option { return logger.WithLevel(level) }
func WithConsole(enable bool) logger.Option      { return logger.WithConsole(enable) }
func WithFile(path string) logger.Option         { return logger.WithFile(path) }

var (
	DebugLevel = logger.DebugLevel
	InfoLevel  = logger.InfoLevel
	WarnLevel  = logger.WarnLevel
	ErrorLevel = logger.ErrorLevel
	FatalLevel = logger.FatalLevel
)

func SetLevel(level logger.Level)       { logger.SetLevel(level) }
func SetDefaultLogger(l *logger.Logger) { logger.SetDefault(l) }

type (
	ServerLoader     = loader.Loader
	LoaderHandler    = loader.Handler
	LoaderRemote     = loader.Remote
	LoaderHandlerReg = loader.HandlerRegistry
	BeforeFilter     = loader.BeforeFilter
	AfterFilter      = loader.AfterFilter
	FilterInfo       = loader.FilterInfo
	CronMethod       = loader.CronMethod
)

func NewServerLoader(basePath string) *loader.Loader {
	return loader.NewLoader(basePath)
}
func NewHandlerRegistry() *loader.HandlerRegistry {
	return loader.NewHandlerRegistry()
}

func Debug(v ...any) { logger.Debug(v...) }
func Info(v ...any)  { logger.Info(v...) }
func Warn(v ...any)  { logger.Warn(v...) }
func Error(v ...any) { logger.Error(v...) }
func Fatal(v ...any) { logger.Fatal(v...) }

func Debugf(format string, v ...any) { logger.Debugf(format, v...) }
func Infof(format string, v ...any)  { logger.Infof(format, v...) }
func Warnf(format string, v ...any)  { logger.Warnf(format, v...) }
func Errorf(format string, v ...any) { logger.Errorf(format, v...) }
func Fatalf(format string, v ...any) { logger.Fatalf(format, v...) }
