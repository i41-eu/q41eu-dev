package devapp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
)

// Presence: Jarvis, who comes through MCP, may use a robot only while the owner is with it.
// The laptop's watcher reports the USB (and the owner's override); Dev sees the touches.

const (
	usbFresh        = 3 * time.Minute  // a USB report older than this says nothing (fail closed)
	idleLimit       = 10 * time.Minute // a robot nobody touched for this long is alone
	maxOverrideMins = 240
)

// personActs are the events a person makes on the robot: any of them is a touch for the gate.
// Jarvis's own MCP calls are not events, and neither are Dev's (connected) or the robot's
// sensors of a hand coming near (near, away) or of the cable (plugged, unplugged).
var personActs = []string{"tap", "long_press", "pat", "pet", "stroke", "shaken", "picked_up", "tag", "tag_removed", "remote", "button"}

// presence is what Dev knows of one robot's owner (a.mu).
type presence struct {
	usb      string    // "connected", "disconnected" or "" (never reported)
	usbAt    time.Time // the last report
	usbSince time.Time // when it last changed
	override time.Time // the override's end
	act      time.Time // the last person's act
}

func (a *App) now() time.Time {
	if a.cfg.Now != nil {
		return a.cfg.Now()
	}
	return time.Now()
}

func (a *App) presenceOf(id string) *presence {
	p := a.presences[id]
	if p == nil {
		p = &presence{}
		a.presences[id] = p
	}
	return p
}

// markPerson: a person acted on the robot now. a.mu held.
func (a *App) markPerson(id string) { a.presenceOf(id).act = a.now() }

// check is the rule: allowed, and the plain sentence when not. now is passed in. a.mu held.
func (p *presence) check(now time.Time) (bool, string) {
	usbOK := p.usb == "connected" && now.Sub(p.usbAt) <= usbFresh
	overrideOK := now.Before(p.override)
	if !usbOK && !overrideOK {
		switch {
		case p.usb == "":
			return false, "The owner is not with the robot: the USB has not been reported and no override runs. It opens when the USB is connected or the owner sets an override."
		case p.usb == "connected":
			return false, "The owner is not with the robot: no report of the USB in the last 3 minutes and no override runs. It opens when the USB is reported connected or the owner sets an override."
		}
		return false, fmt.Sprintf("The owner is not with the robot: the USB was disconnected at %s and no override runs. It opens when the USB is connected or the owner sets an override.", p.usbSince.Format("15:04"))
	}
	if p.act.IsZero() {
		return false, "The robot is alone: nobody has touched it. It opens when the owner taps it."
	}
	if idle := now.Sub(p.act); idle > idleLimit {
		return false, fmt.Sprintf("The robot is alone: nobody touched it for %d minutes. It opens when the owner taps it.", int(idle.Minutes()))
	}
	return true, ""
}

// object is the presence as the API shows it. a.mu held.
func (p *presence) object(now time.Time) map[string]any {
	allowed, reason := p.check(now)
	usb := p.usb
	switch {
	case usb == "":
		usb = "unknown"
	case usb == "connected" && now.Sub(p.usbAt) > usbFresh:
		usb = "unknown"
	}
	o := map[string]any{"allowed": allowed, "reason": reason, "usb": usb,
		"usb_reported_at": nil, "override_until": nil, "idle_seconds": nil}
	if !p.usbAt.IsZero() {
		o["usb_reported_at"] = p.usbAt
	}
	if now.Before(p.override) {
		o["override_until"] = p.override
	}
	if !p.act.IsZero() {
		o["idle_seconds"] = int(now.Sub(p.act).Seconds())
	}
	return o
}

// presenceObject for the state action.
func (a *App) presenceObject(id string) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.presenceOf(id).object(a.now())
}

// gate: the refusal for an MCP tool call, or "" when it may run. robot_state always passes.
func (a *App) gate(id, tool string) string {
	if tool == "robot_state" {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if ok, reason := a.presenceOf(id).check(a.now()); !ok {
		return reason
	}
	return ""
}

// handlePresence: POST sets what the watcher or the owner reports, GET answers the object.
// It is a route, not an action: every action is an MCP tool, and an agent must not open its own gate.
func (a *App) handlePresence(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.authed(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !slices.Contains(acc.robots, id) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "not a robot of this key or browser"})
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, a.presenceObject(id))
		return
	}
	var in struct {
		USB             string  `json:"usb"`
		OverrideMinutes float64 `json:"override_minutes"`
		OverrideOff     bool    `json:"override_off"`
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err := json.Unmarshal(body, &in); err != nil {
		writeErr(w, fmt.Errorf("the body: a JSON object (%v)", err))
		return
	}
	switch {
	case in.USB != "" && in.USB != "connected" && in.USB != "disconnected":
		writeErr(w, fmt.Errorf("usb: connected or disconnected"))
		return
	case in.OverrideMinutes != 0 && (in.OverrideMinutes < 1 || in.OverrideMinutes > maxOverrideMins):
		writeErr(w, fmt.Errorf("override_minutes: 1 to %d", maxOverrideMins))
		return
	case in.USB == "" && in.OverrideMinutes == 0 && !in.OverrideOff:
		writeErr(w, fmt.Errorf(`send usb ("connected" or "disconnected"), override_minutes or override_off`))
		return
	}
	a.mu.Lock()
	now := a.now()
	p := a.presenceOf(id)
	if in.USB != "" {
		if in.USB != p.usb {
			p.usbSince = now
			if in.USB == "connected" && p.usb == "disconnected" {
				p.act = now // plugging in is a person's act; a heartbeat and a first report after a start are not
			}
		}
		p.usb, p.usbAt = in.USB, now
	}
	if in.OverrideMinutes > 0 {
		p.override = now.Add(time.Duration(in.OverrideMinutes * float64(time.Minute)))
		p.act = now
	}
	if in.OverrideOff {
		p.override = time.Time{}
	}
	obj := p.object(now)
	a.mu.Unlock()
	obj["ok"] = true
	writeJSON(w, http.StatusOK, obj)
}
