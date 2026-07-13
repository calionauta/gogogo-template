package config

// defaultDagNatsEnabled is true: DagNats is always compiled in with
// the unified build. Override at runtime with DAGNATS_ENABLED=false.
func defaultDagNatsEnabled() bool { return true }
