//go:build jetstream

package config

// defaultNATSEnabled is true under -tags jetstream: the entire point of
// that build is multi-instance realtime, so opting into the tag opts
// into JetStream. Override at runtime with NATS_ENABLED=false to fall
// back to the in-process SSE Hub.
func defaultNATSEnabled() bool { return true }
