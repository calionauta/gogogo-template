//go:build !dagnats

package config

// defaultDagNatsEnabled is false without -tags dagnats: the durable
// workflow engine (and its JetStream state) isn't compiled in.
func defaultDagNatsEnabled() bool { return false }
