package devapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/i41-eu/q41eu-dev/examples"
	"github.com/i41-eu/q41eu-dev/robot"
)

// handleExample serves an example's file (examples/), as text, to read or download.
func (a *App) handleExample(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(examples.FS, r.PathValue("path"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var ns robot.NotSupported
	switch {
	case errors.Is(err, errNoRobot):
		status = http.StatusNotFound
	case errors.Is(err, errTooMany):
		status = http.StatusTooManyRequests
	case errors.Is(err, robot.ErrGone):
		status = http.StatusServiceUnavailable
	case errors.As(err, &ns):
		status = http.StatusNotImplemented
	}
	writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
}

// handlePair: the robot's QR code. The browser gets a session (a cookie) with the robot.
func (a *App) handlePair(w http.ResponseWriter, r *http.Request) {
	ip := a.clientIP(r)
	if !a.pairFails.has(ip) {
		http.Error(w, "Too many wrong pairing codes from here: try again in a few minutes.", http.StatusTooManyRequests)
		return
	}
	id, ok := a.hub.Redeem(r.URL.Query().Get("code"))
	if !ok {
		a.pairFails.take(ip)
		http.Error(w, "This pairing code is not valid (any more): scan the robot's QR code again.", http.StatusNotFound)
		return
	}
	a.mu.Lock()
	_, s := a.sessionForLocked(w, r)
	if !slices.Contains(s.Robots, id) {
		s.Robots = append([]string{id}, s.Robots...)
	}
	a.dirty = true
	a.mu.Unlock()
	a.log.Info("browser paired", "robot", id)
	a.hub.TellPaired(id)
	http.Redirect(w, r, "/robots?paired="+id, http.StatusSeeOther)
}

// device is a short name for a browser, from its user agent (for the manager's page).
func device(ua string) string {
	browser := "a browser"
	for _, b := range []string{"Firefox", "Edg", "Chrome", "Safari"} {
		if strings.Contains(ua, b+"/") {
			browser = strings.Replace(b, "Edg", "Edge", 1)
			break
		}
	}
	for _, o := range []string{"Android", "iPhone", "iPad", "Windows", "Mac OS", "Linux"} {
		if strings.Contains(ua, o) {
			return browser + " on " + strings.Replace(o, "Mac OS", "macOS", 1)
		}
	}
	return browser
}

// handleState: the page's robots and their keys.
func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	_, s := a.sessionOfLocked(r)
	var robots []string
	if s != nil {
		robots = slices.Clone(s.Robots)
	}
	keys := []*apiKey{}
	for _, k := range a.st.Keys {
		if slices.Contains(robots, k.Robot) {
			keys = append(keys, k)
		}
	}
	a.mu.Unlock()
	slices.SortFunc(keys, func(x, y *apiKey) int { return x.Created.Compare(y.Created) })
	out := []map[string]any{}
	for _, id := range robots {
		out = append(out, a.robotInfo(id))
	}
	me := map[string]any{"sign_in": a.cfg.SignIn != nil, "manager_url": a.cfg.ManagerURL}
	a.mu.Lock()
	if _, s := a.sessionOfLocked(r); s != nil && s.Account != nil {
		me["signed_in"], me["name"] = true, s.Account.Name
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"robots": out, "keys": keys, "public_url": a.cfg.PublicURL,
		"actions": Actions, "events": eventNames(), "version": a.cfg.Version, "me": me})
}

// handleNewKey makes a key for one of the browser's robots; it is shown once.
func (a *App) handleNewKey(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.accessOf(r)
	if !ok || acc.sid == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "pair this browser with the robot first"})
		return
	}
	var in struct{ Robot, Name string }
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil || !slices.Contains(acc.robots, in.Robot) {
		writeErr(w, errNoRobot)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = "key"
	}
	if r := []rune(in.Name); len(r) > 40 {
		in.Name = string(r[:40])
	}
	key := "sdev_" + randomHex(20)
	h := hashKey(key)
	k := &apiKey{ID: h[:8], Robot: in.Robot, Name: in.Name, Created: time.Now()}
	a.mu.Lock()
	n := 0
	for _, x := range a.st.Keys {
		if x.Robot == in.Robot {
			n++
		}
	}
	if n >= 20 {
		a.mu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "20 keys for this robot already: delete some"})
		return
	}
	a.st.Keys[h] = k
	a.dirty = true
	a.mu.Unlock()
	a.hub.TellPaired(in.Robot)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": key, "id": k.ID})
}

func (a *App) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.accessOf(r)
	if !ok || acc.sid == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "not paired"})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for h, k := range a.st.Keys {
		if k.ID == r.PathValue("id") && slices.Contains(acc.robots, k.Robot) {
			delete(a.st.Keys, h)
			a.dirty = true
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "no such key"})
}

func (a *App) handleUnpair(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.accessOf(r)
	if !ok || acc.sid == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "not paired"})
		return
	}
	var in struct{ Robot string }
	json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in)
	a.mu.Lock()
	if s := a.st.Sessions[acc.sid]; s != nil {
		s.Robots = slices.DeleteFunc(s.Robots, func(id string) bool { return id == in.Robot })
		a.dirty = true
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleEvents is the page's live stream: its robots' state, events, and actions by anyone.
func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	sid, s := a.sessionOfLocked(r)
	a.mu.Unlock()
	if s == nil {
		http.Error(w, "not paired", http.StatusUnauthorized)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ip := a.clientIP(r)
	sub := &subscriber{sid: sid, ch: make(chan [2]string, 64)}
	a.mu.Lock()
	if a.streams[ip] >= streamsPerAddr {
		a.mu.Unlock()
		http.Error(w, "too many open pages from here", http.StatusTooManyRequests)
		return
	}
	a.streams[ip]++
	a.subs[sub] = struct{}{}
	robots := slices.Clone(s.Robots)
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.subs, sub)
		if a.streams[ip]--; a.streams[ip] <= 0 {
			delete(a.streams, ip)
		}
		a.mu.Unlock()
		for _, id := range robots {
			a.hub.TellPaired(id)
		}
	}()
	for _, id := range robots {
		a.hub.TellPaired(id) // watching now
	}
	fmt.Fprint(w, ": hello\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-sub.ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", m[0], m[1])
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

/* ------------------------------------ /v1 ------------------------------------ */

func (a *App) authed(w http.ResponseWriter, r *http.Request) (access, bool) {
	acc, ok := a.accessOf(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="q41eu-dev"`)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false,
			"error": "an API key (Authorization: Bearer sdev_…) from this app's page, made by a browser paired with the robot"})
	}
	return acc, ok
}

func (a *App) handleRobots(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.authed(w, r)
	if !ok {
		return
	}
	out := []map[string]any{}
	for _, id := range acc.robots {
		info := a.robotInfo(id)
		delete(info, "readings")
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, map[string]any{"robots": out})
}

func (a *App) handleRobot(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.authed(w, r)
	if !ok {
		return
	}
	out, err := a.run(r.Context(), acc, r.PathValue("id"), "state", args{})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRobotEvents: GET …/events?after=&seconds=&names=a,b — the events action.
func (a *App) handleRobotEvents(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.authed(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	in := args{}
	for _, k := range []string{"after", "seconds"} {
		if v := q.Get(k); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				writeErr(w, fmt.Errorf("%s: a number", k))
				return
			}
			in[k] = f
		}
	}
	if v := q.Get("names"); v != "" {
		var l []any
		for _, n := range strings.Split(v, ",") {
			l = append(l, n)
		}
		in["names"] = l
	}
	out, err := a.run(r.Context(), acc, r.PathValue("id"), "events", in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAction: POST /v1/robots/{id}/{action} with the action's arguments as a JSON object.
func (a *App) handleAction(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.authed(w, r)
	if !ok {
		return
	}
	in := args{}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			writeErr(w, fmt.Errorf("the body: a JSON object (%v)", err))
			return
		}
	}
	out, err := a.run(r.Context(), acc, r.PathValue("id"), r.PathValue("action"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	res := map[string]any{"ok": true}
	if m, isMap := out.(map[string]any); isMap {
		for k, v := range m {
			res[k] = v
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// handleActions lists the actions and their arguments (no key needed: it is the docs).
func (a *App) handleActions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"actions": Actions, "events": eventNames()})
}
