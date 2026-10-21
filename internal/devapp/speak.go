package devapp

import "net/http"

// handleSpeak: a stub until the route is built.
func (a *App) handleSpeak(w http.ResponseWriter, r *http.Request) {
	refuse(w, http.StatusNotImplemented, "not yet")
}
