package neo

import "strings"

type Platform string

const (
	PlatformApple   Platform = "apple"
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
	PlatformOther   Platform = ""
)

// goconst: extracted repeated key name strings
const (
	keyEsc   = "escape"
	keyEnter = "enter"
	keyUp    = "arrowup"
	keyDown  = "arrowdown"
	keyLeft  = "arrowleft"
	keyRight = "arrowright"
	keyDel   = "delete"
	keyShift = "shift"
	modCtrl  = "Ctrl"
)

func DetectPlatform(userAgent string) Platform {
	s := strings.ToLower(userAgent)
	switch {
	case s == "":
		return PlatformOther
	case strings.Contains(s, "mac") ||
		strings.Contains(s, "iphone") ||
		strings.Contains(s, "ipad") ||
		strings.Contains(s, "ipod"):
		return PlatformApple
	case strings.Contains(s, "win"):
		return PlatformWindows
	case strings.Contains(s, "linux") ||
		strings.Contains(s, "android") ||
		strings.Contains(s, "cros") ||
		strings.Contains(s, "x11"):
		return PlatformLinux
	default:
		return PlatformOther
	}
}

func platformMatches(expr string, p Platform) bool {
	e := strings.ToLower(strings.TrimSpace(expr))
	if e == "" {
		return true
	}
	negate := strings.HasPrefix(e, "not ")
	if negate {
		e = strings.TrimSpace(e[len("not "):])
	}
	matched := false
	for tok := range strings.FieldsSeq(e) {
		if platformToken(tok, p) {
			matched = true
			break
		}
	}
	if negate {
		return !matched
	}
	return matched
}

func platformToken(token string, p Platform) bool {
	switch token {
	case "apple", "mac", "macos", "ios":
		return p == PlatformApple
	case "windows", "win":
		return p == PlatformWindows
	case "linux":
		return p == PlatformLinux
	}
	return false
}

var keyAliases = map[string]string{
	"esc":      keyEsc,
	keyEnter:   keyEnter,
	"return":   keyEnter,
	"space":    " ",
	"spacebar": " ",
	"up":       keyUp,
	"down":     keyDown,
	"left":     keyLeft,
	"right":    keyRight,
	"plus":     "+",
	"comma":    ",",
	"del":      keyDel,
}

var keyGlyphsApple = map[string]string{
	" ":         "Space",
	keyEnter:    "↵",
	keyEsc:      "⎋",
	"tab":       "⇥",
	keyDel:      "⌦",
	"backspace": "⌫",
	keyUp:       "↑",
	keyDown:     "↓",
	keyLeft:     "←",
	keyRight:    "→",
}

var keyLabelsOther = map[string]string{
	" ":         "Space",
	keyEnter:    "Enter",
	keyEsc:      "Esc",
	"tab":       "Tab",
	keyDel:      "Del",
	"backspace": "Backspace",
	keyUp:       "↑",
	keyDown:     "↓",
	keyLeft:     "←",
	keyRight:    "→",
}

func keyLabel(key string, apple bool) string {
	m := keyLabelsOther
	if apple {
		m = keyGlyphsApple
	}
	if v, ok := m[key]; ok {
		return v
	}
	r := []rune(key)
	if len(r) == 1 {
		return strings.ToUpper(key)
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

var modifierGlyphsApple = map[string]string{
	"mod":     "⌘",
	"meta":    "⌘",
	"cmd":     "⌘",
	"command": "⌘",
	"super":   "⌘",
	"win":     "⌘",
	"ctrl":    "⌃",
	"control": "⌃",
	"alt":     "⌥",
	"option":  "⌥",
	"opt":     "⌥",
	keyShift:  "⇧",
}

var modifierLabelsOther = map[string]string{
	"mod":     modCtrl,
	"ctrl":    modCtrl,
	"control": modCtrl,
	"alt":     "Alt",
	"option":  "Alt",
	"opt":     "Alt",
	keyShift:  "Shift",
	"meta":    "Win",
	"cmd":     "Win",
	"command": "Win",
	"super":   "Win",
	"win":     "Win",
}

func formatKey(token string, p Platform) string {
	t := strings.ToLower(strings.TrimSpace(token))
	if t == "" {
		return ""
	}
	mods := modifierLabelsOther
	if p == PlatformApple {
		mods = modifierGlyphsApple
	}
	if v, ok := mods[t]; ok {
		return v
	}
	if a, ok := keyAliases[t]; ok {
		t = a
	}
	return keyLabel(t, p == PlatformApple)
}
