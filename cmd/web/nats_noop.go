//go:build !jetstream

package main

import "github.com/calionauta/gogogo-fullstack-template/config"

func startNATS(cfg *config.Config) {
	// NATS not available without -tags jetstream
	_ = cfg
}
