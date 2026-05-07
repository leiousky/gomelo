package handler

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
