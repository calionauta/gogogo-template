//go:build !dagnats

package main

import (
	"github.com/pocketbase/pocketbase"

	"github.com/calionauta/gogogo-fullstack-template/config"
	"github.com/calionauta/gogogo-fullstack-template/features/todo/handlers"
)

// startDagNats is a no-op without -tags dagnats. The DagNats engine (and
// its worker handlers) is not compiled in.
func startDagNats(_ *config.Config, app *pocketbase.PocketBase, todoH *handlers.TodoHandler) {
	_ = app
	_ = todoH
}

func shutdownDagNats() {}
