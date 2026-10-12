package devapp

// The API as an OpenAPI 3.1 document (GET /v1/openapi.json), made from the actions: for docs
// tools and client generators. The actions are its single source, as for MCP and the page.

import "net/http"

func (a *App) openAPI() map[string]any {
	robotParam := map[string]any{"name": "robot", "in": "path", "required": true, "description": "the robot's id, e.g. stackchan-0a1b2c3d4e50",
		"schema": map[string]any{"type": "string"}}
	errResp := func(what string) map[string]any {
		return map[string]any{"description": what, "content": map[string]any{"application/json": map[string]any{
			"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}
	}
	errors := map[string]any{
		"400": errResp("a wrong argument"), "401": errResp("no key, or not a valid one"),
		"404": errResp("not this key's robot"), "429": errResp("too many requests: at most 10 a second per key (bursts of 30)"),
		"501": errResp("the robot does not offer it"), "503": errResp("the robot is offline"),
	}
	withErrors := func(ok map[string]any) map[string]any {
		out := map[string]any{"200": ok}
		for k, v := range errors {
			out[k] = v
		}
		return out
	}
	okJSON := func(desc string) map[string]any {
		return map[string]any{"description": desc, "content": map[string]any{"application/json": map[string]any{
			"schema": map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "additionalProperties": true}}}}
	}
	paths := map[string]any{
		"/v1/robots": map[string]any{"get": map[string]any{"operationId": "listRobots", "summary": "The key's robots",
			"responses": withErrors(okJSON("the robots: id, online, model, firmware, commands"))}},
		"/v1/robots/{robot}": map[string]any{"get": map[string]any{"operationId": "getState", "summary": "The robot's state: online, readings, last events",
			"parameters": []any{robotParam}, "responses": withErrors(okJSON("the robot's state"))}},
		"/v1/robots/{robot}/events": map[string]any{"get": map[string]any{"operationId": "getEvents",
			"summary": "What people did at the robot, after a sequence number; waits for the next one",
			"parameters": []any{robotParam,
				map[string]any{"name": "after", "in": "query", "schema": map[string]any{"type": "integer"}, "description": "only events after this seq (0: the last ones kept)"},
				map[string]any{"name": "seconds", "in": "query", "schema": map[string]any{"type": "integer", "maximum": 60}, "description": "wait this long for one"},
				map[string]any{"name": "names", "in": "query", "schema": map[string]any{"type": "string"}, "description": "only these events, comma-separated"}},
			"responses": withErrors(okJSON("events and last_seq"))}},
	}
	for _, act := range Actions {
		if act.Name == "state" || act.Name == "events" {
			continue // GET above
		}
		paths["/v1/robots/{robot}/"+act.Name] = map[string]any{"post": map[string]any{
			"operationId": act.Name, "summary": act.Help, "parameters": []any{robotParam},
			"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schema(act.Params)}}},
			"responses":   withErrors(okJSON("done (ask: the answer; sound: its length)")),
		}}
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{"title": "Dev: a Stackchan robot for your programs", "version": a.cfg.Version,
			"description": "Every action on a robot, with a key from Dev's page (Authorization: Bearer sdev_…). The same actions are MCP tools at /mcp. https://github.com/i41-eu/q41eu-dev",
			"license":     map[string]any{"name": "Apache-2.0"}},
		"servers":  []any{map[string]any{"url": a.cfg.PublicURL}},
		"security": []any{map[string]any{"key": []any{}}},
		"paths":    paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{"key": map[string]any{"type": "http", "scheme": "bearer", "description": "a key from Dev's page (sdev_…)"}},
			"schemas": map[string]any{"Error": map[string]any{"type": "object", "properties": map[string]any{
				"ok": map[string]any{"type": "boolean"}, "error": map[string]any{"type": "string"}}}},
		},
	}
}

func (a *App) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*") // docs tools elsewhere may read it: it holds no secrets
	writeJSON(w, http.StatusOK, a.openAPI())
}
