package remote

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
