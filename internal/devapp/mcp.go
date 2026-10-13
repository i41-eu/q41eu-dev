package devapp

// MCP (Model Context Protocol, modelcontextprotocol.io) for AI agents: POST /mcp takes JSON-RPC 2.0
// (the Streamable HTTP transport, answered as plain JSON, no event stream) with an API key as the
// bearer. Each action is a tool, robot_<action>, on the key's robot.

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
)

var mcpVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

const mcpInstructions = `You have a body: a Stackchan robot on someone's desk (a small screen face, a head that turns and nods, 12 LEDs, a speaker, a touch screen, a touch-sensitive head, sensors). ` +
	`Use robot_show to put text on its screen, robot_say for a speech bubble on its face, robot_move to nod or look, robot_leds and robot_sound to signal, ` +
	`robot_ask to ask the person at the robot a question they answer with a tap (robot_menu for up to six choices, robot_clickmap for places on a picture), robot_events to see what people did (taps, pats, a tag held near it), robot_state for its readings. ` +
	`Keep text on its screen short. Be calm: it sits next to a person who may be working.`

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func (a *App) handleMCP(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.accessOf(r)
	if !ok || acc.key == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="q41eu-dev"`)
		http.Error(w, "an API key from this app's page as the bearer", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var batch []rpcRequest
	single := !strings.HasPrefix(strings.TrimSpace(string(body)), "[")
	if single {
		var one rpcRequest
		if err := json.Unmarshal(body, &one); err != nil {
			writeJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			return
		}
		batch = []rpcRequest{one}
	} else if err := json.Unmarshal(body, &batch); err != nil {
		writeJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
		return
	}
	var out []rpcResponse
	for _, req := range batch {
		if len(req.ID) == 0 { // a notification: no answer
			continue
		}
		res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		result, rerr := a.mcpCall(r, acc, req)
		if rerr != nil {
			res.Error = rerr
		} else {
			res.Result = result
		}
		out = append(out, res)
	}
	switch {
	case len(out) == 0:
		w.WriteHeader(http.StatusAccepted)
	case single:
		writeJSON(w, http.StatusOK, out[0])
	default:
		writeJSON(w, http.StatusOK, out)
	}
}

func (a *App) mcpCall(r *http.Request, acc access, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		version := mcpVersions[0]
		if slices.Contains(mcpVersions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "q41eu-dev", "version": a.cfg.Version},
			"instructions":    mcpInstructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := []map[string]any{}
		for _, act := range Actions {
			tools = append(tools, map[string]any{"name": "robot_" + act.Name, "description": act.Help, "inputSchema": schema(act.Params)})
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string `json:"name"`
			Arguments args   `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		name, ok := strings.CutPrefix(p.Name, "robot_")
		if !ok || !slices.ContainsFunc(Actions, func(x Action) bool { return x.Name == name }) {
			return nil, &rpcError{-32602, "unknown tool " + p.Name}
		}
		if p.Arguments == nil {
			p.Arguments = args{}
		}
		out, err := a.run(r.Context(), acc, acc.robots[0], name, p.Arguments)
		if err != nil { // a tool's failure is a result the agent reads, not a protocol error
			return map[string]any{"content": []map[string]any{{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		if out == nil {
			out = map[string]any{"ok": true}
		}
		b, _ := json.Marshal(out)
		res := map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}}
		if _, isMap := out.(map[string]any); isMap {
			res["structuredContent"] = out
		}
		return res, nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

// schema is a JSON schema of an action's parameters.
func schema(params []Param) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range params {
		s := map[string]any{"type": p.Type, "description": p.Help}
		switch {
		case p.Type != "array":
		case p.Items == "object":
			s["items"] = map[string]any{"type": "object"}
		case p.Items == "string or object":
			s["items"] = map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "object"}}}
		default:
			s["items"] = map[string]any{"type": "string"}
		}
		if len(p.Enum) > 0 {
			s["enum"] = p.Enum
		}
		props[p.Name] = s
		if p.Required {
			required = append(required, p.Name)
		}
	}
	out := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}
