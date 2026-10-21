package devapp

import "net/http"

// handleListen: stub.
func (a *App) handleListen(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "not yet"})
}
