package auth

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterAuth wires all auth-related routes and middleware on the
// given PocketBase app. Designed to be called from router.Init, which
// owns the wiring across features. The login flow:
//
//   - GET  /login           renders the standalone LoginPage (no navbar)
//   - POST /login           handles credentials, sets pb_auth cookie
//   - POST /logout          clears cookie and redirects
//
// Middleware (in priority order, applied to every request):
//
//   - LoadAuthFromCookie   populates e.Auth from the cookie
//
// Cookie attributes: HttpOnly (set via CookieSecure for production).
func RegisterAuth(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// The /login route has TWO purposes:
		//   1. Redirect already-signed-in users to /
		//   2. Render the login form for everyone else
		// Combine both into a single handler so the chain order is
		// unambiguous. (Previously we used `GET("/login", RedirectIfAuthed)`
		// + `.BindFunc(handleLoginGet)`, which made the handler chain
		// [LoadAuthFromCookie, handleLoginGet, RedirectIfAuthed] and
		// caused handleLoginGet to render before RedirectIfAuthed
		// had a chance to short-circuit — which is fine in theory but
		// didn't match what we saw in production. Merging keeps the
		// routing layer simpler.)
		se.Router.GET("/login", handleLoginGetWithRedirect)
		se.Router.POST("/login", HandlePasswordLogin)
		se.Router.POST("/logout", HandleLogout)

		// Middleware: load auth from cookie on every request.
		// Must be registered AFTER all routes so it applies to them
		// (the PB Router adds group middleware to all routes registered
		// AFTER the BindFunc call). We register it last so every route
		// declared above picks it up.
		se.Router.BindFunc(LoadAuthFromCookie)

		return se.Next()
	})
}

// handleLoginGetWithRedirect renders the login form unless the user is
// already signed in, in which case it sends them to /. Folding the
// redirect into the render handler keeps the route's middleware chain
// flat (a single handler, no BindFunc composition), which avoids the
// subtle ordering bug we hit when /login and /api/* kept returning 303.
func handleLoginGetWithRedirect(e *core.RequestEvent) error {
	fmt.Fprintf(os.Stderr, "[DBG /login] auth=%v path=%s\n", e.Auth != nil, e.Request.URL.Path)
	if e.Auth != nil {
		return e.Redirect(http.StatusSeeOther, "/")
	}
	errMsg := ""
	if cookie, cookieErr := e.Request.Cookie(cookieName); cookieErr == nil && cookie.Value != "" {
		// Re-prompt for the password only if the cookie is
		// malformed, not when it's just an expired session.
		if _, err := e.App.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth); err != nil {
			errMsg = "Session expired. Please sign in again."
		}
	}
	return renderLoginPageTo(e, errMsg)
}

// handleLoginGet renders the standalone login form. Kept tiny so the
// HTTP route lives next to the wiring. Reads ?next= so the user
// returns to the page they tried to reach.
func handleLoginGet(e *core.RequestEvent) error {
	errMsg := ""
	if cookie, cookieErr := e.Request.Cookie(cookieName); cookieErr == nil && cookie.Value != "" {
		// Re-prompt for the password only if the cookie is
		// malformed, not when it's just an expired session.
		if _, err := e.App.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth); err != nil {
			errMsg = "Session expired. Please sign in again."
		}
	}
	return renderLoginPageTo(e, errMsg)
}

// HandleLoginGetForTest is the exported alias used by features that
// wire the login route outside the OnServe hook (e.g. integration
// tests that drive routes via PocketBase's router instead of the
// full server). Identical behaviour to handleLoginGet; exported so
// external test packages can bind it.
func HandleLoginGetForTest(e *core.RequestEvent) error {
	return handleLoginGet(e)
}

// renderLoginPageTo writes the login form to the response. Used by
// the package's HTTP handler so we don't need a separate
// RequestEvent-bound render path inside the OnServe callback.
func renderLoginPageTo(e *core.RequestEvent, errMsg string) error {
	next := e.Request.URL.Query().Get("next")
	if next == "" {
		next = "/"
	}
	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	e.Response.WriteHeader(http.StatusOK)
	return LoginPage(next, errMsg).Render(e.Request.Context(), e.Response)
}

// silence unused-import warnings if the package is built with strict
// linters that flag the strconv import separately.
var _ = strconv.Itoa
