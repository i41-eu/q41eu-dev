# q41eu-dev

The dev app (higher-level robot API over HTTP and MCP) and the Go kit robot apps build on. See [readme.md](readme.md).

- Go only: the standard library, gorilla/websocket, golang.org/x/image (the Go fonts, scaling), and q41eu-raw's `wire`, `robotauth` and `statestore`.
- The kit's packages (`robothub`, `robot`, `screen`, `sound`, `leds`, `gesture`) are for every app: keep them free of Dev's own state. Dev itself is `internal/devapp` and `cmd/q41eu-dev`.
- An action is one entry in `internal/devapp/actions.go`: the REST endpoint, the MCP tool, the page's API list and the readme's table all come from it (update the readme's table too).
- The robot reports primary data only; meaning (a pat, a pet, picked up) is made in `gesture`.
- Before committing, run `gofmt -l .`, `go vet ./...` and `go test -race ./...`.
