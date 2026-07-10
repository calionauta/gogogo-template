//go:build !jetstream

package main

import (
	"github.com/calionauta/gogogo-fullstack-template/config"
	"github.com/calionauta/gogogo-fullstack-template/internal/nats"
)

// startNATS is a no-op without -tags jetstream. When only -tags dagnats
// is active, DagNats boots its OWN embedded NATS, so the template does
// not start a second one. The caller falls back to the in-memory
// broadcaster for realtime within the instance.
func startNATS(cfg *config.Config) nats.JetStreamLike {
	_ = cfg
	return nil
}
