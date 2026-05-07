package filter

import (
	"time"
	"github.com/chuhongliang/gomelo/lib"
)

type GATEFilter struct{}

func (f *GATEFilter) Name() string { return "gate" }

func (f *GATEFilter) Process(ctx *lib.Context) bool {
	ctx.Set("startTime", time.Now())
	return true
}

func (f *GATEFilter) After(ctx *lib.Context) {
}
