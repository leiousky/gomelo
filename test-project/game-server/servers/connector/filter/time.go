package filter

import (
	"time"
	"github.com/chuhongliang/gomelo/lib"
)

type CONNECTORFilter struct{}

func (f *CONNECTORFilter) Name() string { return "connector" }

func (f *CONNECTORFilter) Process(ctx *lib.Context) bool {
	ctx.Set("startTime", time.Now())
	return true
}

func (f *CONNECTORFilter) After(ctx *lib.Context) {
}
