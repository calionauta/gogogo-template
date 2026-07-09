//go:build turbine

// Package workflow wires Turbine durable workflows into the binary.
//
// Turbine (https://turbine.yakir.io) embeds a PocketBase app to persist
// workflow state in SQLite. We use the standalone constructor so workflow
// state lives in its own data dir and does not contend with the main
// PocketBase database. Build with `-tags turbine` to enable.
package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/YakirOren/turbine"
	"github.com/pocketbase/pocketbase"
)

// onboardingPending tracks which users have an in-flight onboarding
// awaiting their first todo. Keyed by user id; set true by
// OnboardingStart (step await_todo) and false by OnboardingContinue
// (finalize). The create-todo handler reads this to decide whether to
// resume the onboarding flow. In-memory only: onboarding is a demo
// affordance, and a restart simply starts it fresh on next login.
var (
	pendingMu         sync.RWMutex
	onboardingPending = make(map[string]bool)
)

// setOnboardingPending flips a user's pending flag. Safe for concurrent
// use by the workflow runtime + the create handler.
func setOnboardingPending(user string, pending bool) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	onboardingPending[user] = pending
}

// IsOnboardingPending reports whether the given user has an onboarding
// flow awaiting their first todo. Read by the create handler to resume
// the flow via OnboardingContinue.
func IsOnboardingPending(user string) bool {
	pendingMu.RLock()
	defer pendingMu.RUnlock()
	return onboardingPending[user]
}

// SetOnboardingPendingForTest sets a user's pending flag. Exported
// (test-only) so integration tests can seed the state without driving
// the full login + workflow path. Production code should not call this.
func SetOnboardingPendingForTest(user string, pending bool) {
	setOnboardingPending(user, pending)
}

// Runtime is a thin handle around a Turbine runtime. It is safe to use
// zero-value before Start has been called; the embedded runtime handles
// all synchronization internally.
type Runtime struct {
	t *turbine.Runtime
}

// IsWorkflowRuntime is a marker that lets the router package accept a
// *Runtime via its WorkflowRuntime interface without importing this
// package at the type level. Keeps the build-tag boundary clean.
func (r *Runtime) IsWorkflowRuntime() {}

// T returns the underlying Turbine runtime. Exposed for tests that need
// to call turbine.Run against already-registered workflows. Production
// code should not need this — workflows are triggered via HTTP handlers
// or scheduled jobs, not directly.
func (r *Runtime) T() *turbine.Runtime { return r.t }

// stepDelay paces the demo so a human can actually follow the durable
// workflow advancing. Without it, all three todo-creation steps (plus
// greet/finalize) would complete in a few milliseconds and the live
// stepper would flash from 1→5 instantly. Turbine replays recorded steps
// on crash recovery, so the sleeps only run on the first execution of a
// step, never on replay — the demo stays followable without inflating
// recovery time.
const stepDelay = 1500 * time.Millisecond

// onboardingPause is the deliberate one-shot 1-minute pause in the
// event-driven onboarding flow (OnboardingContinue). It lets the user
// watch the stepper sit on "scheduled (1 min)" and then advance, without
// using turbine.WithSchedule (which is RECURRING cron and would spam).
const onboardingPause = 60 * time.Second

// Hello is the minimal example workflow. Each step's result is recorded
// in SQLite and replayed on recovery after a crash, so the function may
// be called many times per workflow run without re-executing side effects.
func Hello(ctx turbine.Context, name string) (string, error) {
	greeting, err := turbine.Do(ctx, func(ctx context.Context) (string, error) {
		return fmt.Sprintf("hello, %s", name), nil
	}, turbine.WithStepName("greet"))
	if err != nil {
		return "", err
	}

	return turbine.Do(ctx, func(ctx context.Context) (string, error) {
		return greeting + " (workflow step recorded)", nil
	}, turbine.WithStepName("finalize"))
}

// OnboardingStart is the first half of the event-driven onboarding
// flow, triggered automatically when a user logs in (see
// onboarding.go's login hook). It is scoped to the logged-in user (the
// input is the user's PocketBase record id), so each browser session
// gets its OWN onboarding instance — it is not a global broadcast.
//
// Steps:
//  1. greet — welcome the user (durable, paced).
//  2. await_todo — marks the user's onboarding as "pending first todo"
//     and ends. The flow then WAITS for the user to create a real todo;
//     the create handler resumes it via OnboardingContinue.
//
// Why split into two workflows instead of one long-running paused
// workflow: Turbine v0.3.0 has no "wait for external signal / suspend"
// primitive, so a single workflow cannot block on a user action without
// polling. Splitting keeps each workflow short and lets the app's own
// events (login, create-todo) drive the transitions — the idiomatic
// event-driven pattern, no polling, no blocked workers.
func OnboardingStart(ctx turbine.Context, user string) (string, error) {
	_, err := turbine.Do(ctx, func(ctx context.Context) (string, error) {
		time.Sleep(stepDelay) // visible pace: step 1
		return fmt.Sprintf("Welcome, %s!", user), nil
	}, turbine.WithStepName("greet"))
	if err != nil {
		return "", err
	}

	// Mark this user's onboarding as awaiting their first todo. The
	// create handler checks this flag and resumes the flow. Recorded as
	// a durable step so recovery replays it instead of re-flipping.
	_, err = turbine.Do(ctx, func(ctx context.Context) (string, error) {
		setOnboardingPending(user, true)
		return "awaiting-first-todo", nil
	}, turbine.WithStepName("await_todo"))
	if err != nil {
		return "", err
	}
	return "onboarding-started", nil
}

// OnboardingContinue is the second half of the event-driven onboarding
// flow, triggered by the create-todo handler when a user with a pending
// onboarding adds their first todo. It is also user-scoped.
//
// Steps:
//  1. todo_captured — records that the user's first todo arrived.
//  2. scheduled_pause — a deliberate 1-minute durable pause so the user
//     can watch the stepper sit on "scheduled (1 min)" and then advance.
//     Implemented as time.Sleep inside the durable step (Turbine replays
//     recorded steps on crash recovery, so the sleep is skipped on replay
//     — recovery stays fast). NOTE: this is NOT turbine.WithSchedule —
//     that registers a RECURRING cron workflow, which would spam todos
//     every minute; we want a one-shot 1-minute pause, so time.Sleep is
//     the correct primitive here.
//  3. finalize — clears the pending flag and ends with a completion alert.
func OnboardingContinue(ctx turbine.Context, user string) (string, error) {
	_, err := turbine.Do(ctx, func(ctx context.Context) (string, error) {
		time.Sleep(stepDelay) // visible pace: step "todo captured"
		return "todo-captured", nil
	}, turbine.WithStepName("todo_captured"))
	if err != nil {
		return "", err
	}

	// Deliberate one-shot 1-minute pause (see doc comment above).
	_, err = turbine.Do(ctx, func(ctx context.Context) (string, error) {
		time.Sleep(onboardingPause)
		return "scheduled-pause-done", nil
	}, turbine.WithStepName("scheduled_pause"))
	if err != nil {
		return "", err
	}

	finalized, err := turbine.Do(ctx, func(ctx context.Context) (string, error) {
		setOnboardingPending(user, false)
		return "onboarding-completed", nil
	}, turbine.WithStepName("finalize"))
	if err != nil {
		return "", err
	}
	return finalized, nil
}

// Config is the subset of the application config that workflow construction
// needs. Defined here so tests can construct one without importing the full
// config package (which transitively loads the secrets package).
type Config struct {
	Enabled    bool
	ExecutorID string
}

// New creates a Turbine runtime wired into the supplied PocketBase app.
//
// Turbine stores its durable workflow state (the pt_* collections) inside the
// app's own database, reusing the app's existing SQLite connection (the
// ncruces/go-sqlite3 driver, volume-backed and already writable in prod).
// This is why we pass the main *pocketbase.PocketBase instead of using
// turbine.NewStandalone: NewStandalone spins up a SECOND PocketBase with the
// default sqlite driver and a hardcoded data dir, which is exactly what broke
// in the container (mkdir /pb_data + modernc version mismatch). Sharing the
// app avoids a second data dir and a second driver entirely.
//
// Launch is deferred to the app's OnServe lifecycle hook (registered by
// turbine.Setup), which fires after the app is bootstrapped and its migrations
// have run. Callers in the normal serve path must NOT call Start() — doing so
// would double-launch (the OnServe hook already launched it). Tests that never
// invoke app.Start() may call Start() manually after Bootstrapping the app.
//
// Shutdown is wired into the app's OnTerminate hook by turbine.Setup.
func New(app *pocketbase.PocketBase, cfg Config, logger *slog.Logger) (*Runtime, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("workflow: WORKFLOW_ENABLED must be true to construct runtime")
	}

	tcfg := turbine.Config{
		ExecutorID:         cfg.ExecutorID,
		ApplicationVersion: "gogogo-fullstack-template",
		Logger:             logger,
	}

	// Setup wires Launch into app.OnServe (runs after bootstrap + migrations)
	// and Shutdown into app.OnTerminate. It shares the app's DB connection, so
	// workflow state lives alongside the main collections in the same SQLite file.
	rt := turbine.Setup(app, tcfg)

	turbine.Register(rt, Hello)
	turbine.Register(rt, OnboardingStart)
	turbine.Register(rt, OnboardingContinue)

	return &Runtime{t: rt}, nil
}

// Start launches the runtime. It creates the pt_* collections on first run,
// recovers any pending workflows from a previous process, and starts the
// queue runner and cron scheduler.
func (r *Runtime) Start() error {
	return r.t.Launch()
}

// Shutdown drains pending workflows (up to the configured ShutdownTimeout)
// and resets the embedded PocketBase bootstrap state.
func (r *Runtime) Shutdown() {
	r.t.Shutdown()
}
