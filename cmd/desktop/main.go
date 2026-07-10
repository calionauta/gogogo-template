// Command desktop builds the Wails v3 native app shell around the exact
// same Go backend the web app uses (PocketBase + goqite queue + router +
// handlers). It boots the server via internal/server.Run, serves
// PocketBase in a goroutine, and points the Wails webview at it through a
// reverse proxy — so the desktop app and the web app share 100% of the
// business logic.
//
// Build with -tags jetstream: the desktop becomes a NATS Leaf Node that
// syncs its JetStream streams with the central server (NATS_LEAFNODE_URL)
// — Phase B of the edge-sync design. Phase C (Loro CRDT collab) builds on
// top of this transport.
//
//go:build jetstream

package main

import (
	"fmt"
	"log"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/calionauta/gogogo-fullstack-template/config"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
	"github.com/calionauta/gogogo-fullstack-template/internal/server"
)

func main() {
	cfg := config.Load()

	// Edge sync (Phase B): if a central NATS URL is configured, boot this
	// instance as a Leaf Node so its JetStream streams replicate to/from
	// the central server (offline edits replay on reconnect). Otherwise
	// start a standalone embedded NATS for local realtime only.
	var js nats.JetStreamLike
	if cfg.NATS.LeafNodeURL != "" {
		if err := nats.StartLeafNode(cfg.NATS.StoreDir, cfg.NATS.LeafNodeURL); err != nil {
			log.Printf("WARN: leaf node start failed, falling back to standalone NATS: %v", err)
		} else {
			js = nats.JetStream()
		}
	} else if cfg.NATS.Enabled {
		if err := nats.StartEmbedded(cfg.NATS.StoreDir); err != nil {
			log.Printf("WARN: NATS start failed, in-memory broadcaster only: %v", err)
		} else {
			js = nats.JetStream()
		}
	}

	// Boot the shared backend (PocketBase + queue + router + handlers).
	// js is the JetStream the broadcaster uses (Leaf Node or standalone);
	// nil falls back to in-process SSE.
	pb, _, shutdown, err := server.Run(cfg, js)
	if err != nil {
		log.Fatalf("boot failed: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// PocketBase blocks on pb.Start(), so run it on a goroutine. The
	// Wails webview reaches it via the reverse proxy below.
	go func() {
		args := []string{"serve", "--http", addr}
		if len(os.Args) > 1 {
			args = os.Args[1:]
		}
		pb.RootCmd.SetArgs(args)
		if startErr := pb.Start(); startErr != nil {
			log.Fatalf("pocketbase start: %v", startErr)
		}
	}()

	// Reverse proxy: the Wails webview loads http://localhost:<port>
	// (where PocketBase serves the UI) through this handler.
	target, err := url.Parse(fmt.Sprintf("http://%s", addr))
	if err != nil {
		log.Fatalf("parse target url: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	app := application.New(application.Options{
		Name: "Gogogo Template",
		Assets: application.AssetOptions{
			Handler: proxy,
		},
		// Shut the backend down when the Wails app closes (window quit),
		// not via defer — defer would be skipped on a fatal error and the
		// gocritic linter flags it. OnShutdown runs on a clean exit.
		OnShutdown: func() {
			shutdown()
		},
	})

	log.Printf("desktop: PocketBase on %s, webview proxied", addr)
	if runErr := app.Run(); runErr != nil {
		log.Fatalf("wails run: %v", runErr)
	}
}
