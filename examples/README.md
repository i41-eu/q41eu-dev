# Examples

Your program and a Stackchan robot through Dev. Set three variables (Dev's page shows them with a
new key; the whole way there: [getting started](../docs/getting-started.md)):

```bash
export SDEV_URL=https://dev.q41.eu SDEV_ROBOT=stackchan-0a1b2c3d4e50 SDEV_KEY=sdev_…
```

| Example | curl | Python (standard library) | Go ([client](../client/client.go)) |
|---|---|---|---|
| hello: a card, LEDs, a nod, a chime | `sh curl/hello.sh` | `python3 python/hello.py` | `go run ./examples/go/hello` |
| ask: a question, the answer tapped | `sh curl/ask.sh` | `python3 python/ask.py "Deploy?" Yes No` | `go run ./examples/go/ask "Deploy?" Yes No` |
| build status: around any command | | `python3 python/build_status.py make test` | `go run ./examples/go/buildstatus go test ./...` |
| events: what people do, and answers | `sh curl/events.sh` | `python3 python/events.py` | `go run ./examples/go/events` |
