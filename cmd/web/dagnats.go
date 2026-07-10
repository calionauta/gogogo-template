//go:build dagnats

package main

import (
	"context"
	"log"
	"time"

	"github.com/danmestas/dagnats/server"
	"github.com/danmestas/dagnats/worker"

	"github.com/pocketbase/pocketbase"

	"github.com/calionauta/gogogo-fullstack-template/config"
	"github.com/calionauta/gogogo-fullstack-template/features/todo/handlers"
	"github.com/calionauta/gogogo-fullstack-template/internal/dagnats"
)

var dagNatsServer *server.Server

// startDagNats boots the DagNats durable-workflow engine in the same
// binary on its own HTTP port (cfg.DagNats.HTTPAddr, default :8090). It
// registers the onboarding worker handlers (which write example todos to
// the main PocketBase collection) and starts the engine.
//
// Single-NATS convention: DagNats owns the embedded NATS on the
// conventional port 127.0.0.1:4222. When built with -tags "jetstream
// dagnats", the realtime broadcaster (cmd/web/nats.go) connects to THIS
// NATS instead of starting its own — one NATS server, two consumers
// (DagNats workflows + JetStream realtime). startDagNats does NOT block:
// it fires Run() in a goroutine and returns. The synchronization point
// is ConnectExisting (called by startNATS right after), which uses
// nats.RetryOnFailedConnect to block until the engine's NATS is
// reachable — no polling loop in our code.
func startDagNats(cfg *config.Config, app *pocketbase.PocketBase, todoH *handlers.TodoHandler) {
	if !cfg.DagNats.Enabled {
		return
	}

	srv := server.New(server.Config{
		DataDir:       cfg.DagNats.StoreDir,
		HTTPAddr:      cfg.DagNats.HTTPAddr,
		NATSPort:      4222,     // fixed conventional port — shared with the realtime broadcaster
		MaxStoreBytes: 10 << 30, // 10 GiB JetStream store cap (required by dagnats)
	})
	dagNatsServer = srv

	// Register the onboarding worker handlers on the same NATS the engine
	// uses. Handlers are plain functions keyed by task NAME (string), so
	// refactoring Go never orphans an in-flight workflow.
	shim := server.EmbeddedWorker(srv)
	shim.Handle("onboarding-greet", func(ctx worker.TaskContext) error {
		time.Sleep(1500 * time.Millisecond) // visible pace
		log.Printf("dagnats: onboarding greet")
		return ctx.Complete([]byte(`"welcomed"`))
	})
	// onboarding-await-first-todo blocks (in-process, on the engine's
	// signal KV) until the app signals "first-todo" — i.e. the user
	// created their first todo. This is the dagnats-documented resume
	// pattern and is how the durable run pauses for external input
	// without polling or an in-memory flag.
	shim.Handle("onboarding-await-first-todo", func(ctx worker.TaskContext) error {
		log.Printf("dagnats: awaiting first todo signal for run %s", ctx.RunID())
		_, err := ctx.WaitForSignal("first-todo", 50*time.Minute)
		if err != nil {
			log.Printf("dagnats: await first-todo timed out/failed: %v", err)
			return ctx.Fail(err)
		}
		log.Printf("dagnats: first todo signal received for run %s", ctx.RunID())
		return ctx.Complete([]byte(`"resumed"`))
	})
	shim.Handle("onboarding-create-todo", func(ctx worker.TaskContext) error {
		text := ""
		if md := ctx.Metadata(); md != nil {
			text = md["text"]
		}
		if text == "" {
			text = "Onboarding task"
		}
		if err := todoH.CreateTodoForOnboarding(text, ""); err != nil {
			log.Printf("dagnats: create todo failed: %v", err)
			return ctx.Fail(err)
		}
		return ctx.Complete([]byte(`"created"`))
	})
	shim.Handle("onboarding-finalize", func(ctx worker.TaskContext) error {
		log.Printf("dagnats: onboarding finalized")
		return ctx.Complete([]byte(`"done"`))
	})

	// Register the onboarding workflow definition idempotently so it is
	// always in sync with this binary. The REST API only comes up once
	// srv.Run() binds the port, so do it in a retry loop that waits for
	// the API to be reachable.
	go registerOnboardingWorkflowWithRetry(cfg.DagNats.HTTPAddr)

	go func() {
		if err := srv.Run(); err != nil {
			log.Printf("WARN: dagnats server stopped: %v", err)
		}
	}()
	log.Printf("dagnats: listening on %s (NATS on :4222)", cfg.DagNats.HTTPAddr)
}

// registerOnboardingWorkflowWithRetry registers the onboarding workflow,
// retrying until the DagNats REST API is reachable (it boots after
// srv.Run binds the port).
func registerOnboardingWorkflowWithRetry(httpAddr string) {
	client := dagnats.NewClient("http://" + httpAddr)
	for attempt := 0; attempt < 30; attempt++ {
		if err := client.RegisterWorkflow(context.Background(), []byte(dagnats.OnboardingWorkflowJSON)); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		log.Printf("dagnats: onboarding workflow registered")
		return
	}
	log.Printf("WARN: dagnats workflow register failed after retries")
}

func shutdownDagNats() {
	if dagNatsServer != nil {
		// server.Run blocks until context cancel; the engine's Shutdown
		// is wired internally — closing the process triggers graceful
		// drain via the server's own signal handling.
		dagNatsServer = nil
	}
}
