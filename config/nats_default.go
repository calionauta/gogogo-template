package config

// defaultNATSEnabled is true: NATS is always compiled in with the
// unified build. Override at runtime with NATS_ENABLED=false.
func defaultNATSEnabled() bool { return true }
