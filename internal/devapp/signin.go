package devapp

// Sign-in through the Stackchan manager (Config.SignIn, package sso from q41eu-raw): one
// sign-in for every app. A browser signed in at the manager is signed in here silently and gets
// the robots its account owns, without the QR code; anyone else pairs by the QR code.

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/i41-eu/q41eu-raw/sso"
)

const (
	ssoTriedCookie   = "dev_sso_tried" // the manager's hint a silent sign-in was tried with
	signInCheckEvery = time.Minute
)

// sessionFor is the browser's session, made (with its cookie) when it has none. a.mu held.
func (a *App) sessionForLocked(w http.ResponseWriter, r *http.Request) (string, *session) {
	if sid, s := a.sessionOfLocked(r); s != nil {
		return sid, s
	}
	sid := randomHex(16)
	s := &session{Since: time.Now(), LastSeen: time.Now(), Device: device(r.UserAgent())}
	a.st.Sessions[sid] = s
	a.dirty = true
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: strings.HasPrefix(a.cfg.PublicURL, "https://"), MaxAge: 365 * 24 * 3600})
	return sid, s
}

// pairOwnedLocked gives a signed-in session the robots its account owns. a.mu held.
func (a *App) pairOwnedLocked(s *session) {
	if s == nil || s.Account == nil || s.Account.Key == "" {
		return
	}
	for id, owner := range a.st.Owners {
		if owner == s.Account.Key && !slices.Contains(s.Robots, id) {
			s.Robots = append(s.Robots, id)
			a.dirty = true
		}
	}
}

// setOwner records whose robot it is (the manager's account key; "" for the shared token) and
// gives it to the owner's signed-in browsers.
func (a *App) setOwner(id, owner string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if owner == "" {
		if _, ok := a.st.Owners[id]; ok {
			delete(a.st.Owners, id)
			a.dirty = true
		}
		return
	}
	if a.st.Owners[id] != owner {
		a.st.Owners[id] = owner
		a.dirty = true
	}
	for _, s := range a.st.Sessions {
		a.pairOwnedLocked(s)
	}
}

// GET /auth/login?next=/path: off to the manager. With silent=1 (Run on a page, with no robot
// yet): only a sign-in the browser already has there, tried as on Your robots; when there is
// nothing to try, back to next at once, with ?signin=no unless signed in here.
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if a.cfg.SignIn == nil {
		http.NotFound(w, r)
		return
	}
	next := localPath(r.URL.Query().Get("next"))
	if r.URL.Query().Get("silent") != "1" {
		http.Redirect(w, r, a.cfg.SignIn.LoginURL(a.ssoReturn(next), false), http.StatusFound)
		return
	}
	if a.silentSignIn(w, r, next, true) {
		return
	}
	if !a.signedIn(r) {
		next = withSignInNo(next)
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) ssoReturn(next string) string {
	return a.cfg.PublicURL + "/auth/sso?next=" + url.QueryEscape(next)
}

// localPath is next if it is a path of this site, else "/".
func localPath(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n") {
		return "/"
	}
	return next
}

// trySignIn wraps a page that shows the person's robots: a browser not signed in here, but
// signed in at the manager lately (its hint cookie, sso.HintCookie), goes there silently once and
// comes back signed in. anyBrowser: also a browser without the hint, when Config.SignInAlways (Dev
// on another domain than the manager's, where the hint cannot be seen). The docs pages are not
// wrapped: they answer every request as they are, a search engine's and a link preview's too.
func (a *App) trySignIn(page http.HandlerFunc, anyBrowser bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.SignIn == nil || r.URL.Query().Has("signin") || !a.silentSignIn(w, r, r.URL.RequestURI(), anyBrowser) {
			page(w, r)
		}
	}
}

// silentSignIn sends a browser not signed in here to the manager for a silent sign-in, back to
// next, at most once per sso.RetryAfter: when the manager's hint cookie names it, or (anyBrowser
// and Config.SignInAlways) any browser. False: nothing to try, nothing written.
func (a *App) silentSignIn(w http.ResponseWriter, r *http.Request, next string, anyBrowser bool) bool {
	hint := sso.SilentTry(r, ssoTriedCookie, anyBrowser && a.cfg.SignInAlways)
	if hint == "" || a.signedIn(r) {
		return false
	}
	sso.MarkTried(w, ssoTriedCookie, hint, strings.HasPrefix(a.cfg.PublicURL, "https://"))
	http.Redirect(w, r, a.cfg.SignIn.LoginURL(a.ssoReturn(next), true), http.StatusFound)
	return true
}

// signedIn: the browser's session here is signed in through the manager.
func (a *App) signedIn(r *http.Request) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, s := a.sessionOfLocked(r)
	return s != nil && s.Account != nil
}

// withSignInNo is the local path next with ?signin=no: a silent sign-in came back without one (or
// was not tried), so the page shows as it is and does not try again.
func withSignInNo(next string) string {
	u, err := url.Parse(next)
	if err != nil {
		return "/?signin=no"
	}
	v := u.Query()
	v.Set("signin", "no")
	u.RawQuery = v.Encode()
	return u.String()
}

// GET /auth/sso?next=…&code=… (or &error=…): back from the manager.
func (a *App) handleSSOReturn(w http.ResponseWriter, r *http.Request) {
	if a.cfg.SignIn == nil {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	next := localPath(q.Get("next"))
	if q.Get("code") == "" { // not signed in at the manager (silent), or cancelled
		http.Redirect(w, r, withSignInNo(next), http.StatusSeeOther)
		return
	}
	ans, err := a.cfg.SignIn.Exchange(r.Context(), q.Get("code"))
	if err != nil || !ans.OK {
		if err != nil {
			a.log.Warn("sign-in failed", "err", err)
		}
		http.Error(w, "Sign-in failed, please try again.", http.StatusBadGateway)
		return
	}
	a.mu.Lock()
	_, s := a.sessionForLocked(w, r)
	s.Account, s.Handle = ans.Account, ans.Handle
	a.pairOwnedLocked(s)
	a.dirty = true
	robots := slices.Clone(s.Robots)
	a.mu.Unlock()
	a.log.Info("signed in", "robots", len(robots))
	for _, id := range robots {
		a.hub.TellPaired(id)
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// POST /auth/logout: here and, through the manager, in every app. The browser keeps the robots
// it paired by their QR codes.
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	h := ""
	if _, s := a.sessionOfLocked(r); s != nil {
		h = s.Handle
		s.Account, s.Handle = nil, ""
		a.dirty = true
	}
	a.mu.Unlock()
	if a.cfg.SignIn != nil && h != "" {
		if err := a.cfg.SignIn.Logout(r.Context(), h); err != nil {
			a.log.Warn("sign-out at the manager", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"signed_in": false})
}

// runSignInCheck asks the manager about every sign-in every minute, and ends those it ended.
func (a *App) runSignInCheck(ctx context.Context) {
	if a.cfg.SignIn == nil {
		return
	}
	t := time.NewTicker(signInCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.checkSignIns(ctx)
		}
	}
}

func (a *App) checkSignIns(ctx context.Context) {
	a.mu.Lock()
	handles := map[string]string{}
	for sid, s := range a.st.Sessions {
		if s.Handle != "" {
			handles[sid] = s.Handle
		}
	}
	a.mu.Unlock()
	for sid, h := range handles {
		acct, on, err := a.cfg.SignIn.Check(ctx, h)
		if err != nil {
			a.log.Warn("sign-in check", "err", err)
			continue
		}
		a.mu.Lock()
		if s := a.st.Sessions[sid]; s != nil && s.Handle == h {
			switch {
			case !on:
				s.Account, s.Handle = nil, ""
				a.dirty = true
			case acct != nil && acct.Key != "" && (s.Account == nil || *s.Account != *acct):
				fresh := *acct
				s.Account = &fresh // e.g. the manager joined two sign-ins into one user
				a.pairOwnedLocked(s)
				a.dirty = true
			}
		}
		a.mu.Unlock()
	}
}
