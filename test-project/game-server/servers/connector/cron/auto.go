package cron

import (
	"context"
	"github.com/chuhongliang/gomelo/lib"
)

type CONNECTORCron struct {
	app *lib.App
}

func (c *CONNECTORCron) Init(app *lib.App) { c.app = app }

func (c *CONNECTORCron) Cleanup(ctx context.Context) error {
	return nil
}
