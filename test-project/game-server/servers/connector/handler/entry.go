package handler

import (
	"github.com/chuhongliang/gomelo/lib"
)

type EntryHandler struct {
	app *lib.App
}

func (h *EntryHandler) Init(app *lib.App) { h.app = app }

func (h *EntryHandler) Entry(ctx *lib.Context) {
	var req struct {
		Name string `json:"name"`
	}
	ctx.Bind(&req)
	ctx.Response(map[string]any{"msg": "hello " + req.Name})
}

func (h *EntryHandler) GetFriends(ctx *lib.Context) {
	ctx.ResponseOK(map[string]any{"friends": []string{}})
}

func (h *EntryHandler) Logout(ctx *lib.Context) {
	ctx.ResponseOK(nil)
}
