# Examples

Your program and a Stackchan robot through Dev. Set three variables (Dev's page shows them with a
new key; the whole way there: [getting started](../docs/getting-started.md)):

```bash
export SDEV_URL=https://dev.q41.eu SDEV_ROBOT=stackchan-0a1b2c3d4e50 SDEV_KEY=sdev_…
```

| Example | curl | Python (standard library) | JavaScript (Node 18+, only `fetch`) | Go ([client](../client/client.go)) |
|---|---|---|---|---|
| hello: a card, LEDs, a nod, a chime | `sh curl/hello.sh` | `python3 python/hello.py` | `node js/hello.mjs` | `go run ./examples/go/hello` |
| ask: a question, the answer tapped | `sh curl/ask.sh` | `python3 python/ask.py "Deploy?" Yes No` | `node js/ask.mjs "Deploy?" Yes No` | `go run ./examples/go/ask "Deploy?" Yes No` |
| build status: around any command | `sh curl/build_status.sh make test` | `python3 python/build_status.py make test` | `node js/build_status.mjs make test` | `go run ./examples/go/buildstatus go test ./...` |
| menu: four tiles, the one tapped | `sh curl/menu.sh` | `python3 python/menu.py` | `node js/menu.mjs` | `go run ./examples/go/menu` |
| picture: yours on the screen, the quarter tapped | `sh curl/picture.sh photo.jpg` | `python3 python/picture.py photo.jpg` | `node js/picture.mjs photo.jpg` | `go run ./examples/go/picture photo.jpg` |
| events: what people do, and answers | `sh curl/events.sh` | `python3 python/events.py` | `node js/events.mjs` | `go run ./examples/go/events` |

**For an AI agent** (MCP): [mcp/](mcp/) has each example as what to ask the agent and the tool
calls it makes (`robot_show`, `robot_ask`, …), with Dev added as an MCP server
([getting started](../docs/getting-started.md#5-an-ai-agent)).
