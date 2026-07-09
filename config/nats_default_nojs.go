//go:build !jetstream

package config

// defaultNATSEnabled is false without -tags jetstream: the in-memory
// broadcaster is the default single-instance realtime layer, and the
// nats package's jetstream code isn't even compiled in.
func defaultNATSEnabled() bool { return false }
