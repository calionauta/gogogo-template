//go:build turbine

package config

// defaultWorkflowEnabled is true under -tags turbine: the whole point of
// that build is durable multi-step workflows, so opting into the tag
// opts into Turbine. Override at runtime with WORKFLOW_ENABLED=false.
func defaultWorkflowEnabled() bool { return true }
