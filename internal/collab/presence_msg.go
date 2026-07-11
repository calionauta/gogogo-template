package collab

// PresenceMsg is one ephemeral presence event shared by both the
// jetstream and web-only transports. Type is "cursor" | "join" | "leave".
// Cursor carries normalized coordinates (0..1) so any viewport can place a
// remote cursor regardless of the peer's canvas size.
//
// Declared here (build-tag-free) so presence_web.go and presence.go
// (jetstream) share a single definition.
type PresenceMsg struct {
	Type string  `json:"type"`
	Doc  string  `json:"doc"`
	User string  `json:"user"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	TS   int64   `json:"ts"`
}
