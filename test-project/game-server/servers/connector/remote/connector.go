package remote

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
