//go:build dagnats

package config

// defaultDagNatsEnabled is true under -tags dagnats: the whole point of
// that build is durable JSON workflows, so opting into the tag opts into
// DagNats. Override at runtime with DAGNATS_ENABLED=false.
func defaultDagNatsEnabled() bool { return true }
