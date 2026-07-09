//go:build !turbine

package config

// defaultWorkflowEnabled is false without -tags turbine: the workflow
// runtime (and its PocketBase state collections) isn't compiled in.
func defaultWorkflowEnabled() bool { return false }
