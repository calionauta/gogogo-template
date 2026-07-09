package todo

import "time"

type Todo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created"`
	UpdatedAt time.Time `json:"updated"`
}

type Signals struct {
	Todos     []Todo `json:"todos"`
	NewTitle  string `json:"newTitle"`
	Filter    string `json:"filter"` // "all", "active", "completed"
	EditingID string `json:"editingId"`
	EditTitle string `json:"editTitle"`
	Loading   bool   `json:"loading"`
	ItemCount int    `json:"itemCount"`
	// ClientID is the SSE client id assigned when the stream opens. The
	// UI uses it to route queued-job results and retry feedback back to
	// the correct browser tab, so a Suggest triggered here lands here
	// rather than broadcasting to every connected client.
	ClientID string `json:"clientID"`
	// AdminEnabled reflects whether the server was started with a
	// non-empty ADMIN_UNLOCK_TOKEN (loaded from the age-encrypted
	// secrets file). When true, the UI renders the "Admin unlock"
	// form. When false, the entire admin pathway is hidden — there is
	// no client-side check to bypass.
	AdminEnabled bool `json:"adminEnabled"`

	// LLMEnabled reflects whether the server was started with a
	// non-empty GOAI_API_KEY. When true, the UI renders the "Suggest"
	// button. When false, the entire AI pathway is hidden.
	LLMEnabled bool `json:"llmEnabled"`

	// SimulatedLLM reflects whether the server was started with
	// SIMULATE_LLM=true. When true, the UI renders the "Suggest
	// (simulated)" button, which exercises the same queue + retry path
	// as Suggest but against an in-process fake LLM (no API key needed).
	SimulatedLLM bool `json:"simulatedLLM"`

	// Suggestions is the latest AI-suggested completions, populated
	// by POST /api/todos/suggest. Empty when no suggestions or
	// LLMEnabled is false.
	Suggestions []string `json:"suggestions"`
	// SuggestErr surfaces a human-readable error from the LLM
	// provider (without leaking internals). Empty on success.
	SuggestErr string `json:"suggestErr"`

	// ConnectedClients is the number of browser tabs/clients currently
	// connected to the SSE stream. Streamed on connect and on every
	// connect/disconnect so the UI can show realtime presence.
	ConnectedClients int `json:"connectedClients"`

	// WorkflowEnabled reflects whether the binary was built with
	// -tags turbine AND started with WORKFLOW_ENABLED=true. When true,
	// the UI renders the "Run durable workflow" button.
	WorkflowEnabled bool `json:"workflowEnabled"`

	// Onboarding progress (Turbine durable workflow). Streamed as the
	// workflow advances so the UI can render a live stepper. Each event
	// carries the current step, total steps, phase, and a human-readable
	// detail. OnboardingActive flips true on the first progress event and
	// stays true so the stepper remains visible until the next run resets
	// it — this is what makes the durable, restart-tolerant nature of the
	// workflow observable in the UI.
	OnboardingActive bool   `json:"onboardingActive"`
	OnboardingStep   int    `json:"onboardingStep"`
	OnboardingTotal  int    `json:"onboardingTotal"`
	OnboardingPhase  string `json:"onboardingPhase"`
	OnboardingDetail string `json:"onboardingDetail"`
	// WorkflowCompleted is set true when the durable onboarding
	// workflow finishes all five steps. The UI renders a final alert
	// so the user gets an explicit completion signal (not just the
	// stepper reaching 5/5).
	WorkflowCompleted bool `json:"workflowCompleted"`
}
