# q41eu-dev

**Dev** gives your Stackchan robot to your own programs and AI agents, and gives app authors a Go
kit, so a new robot app is a few hundred lines instead of a few thousand.

- **A script in any language** shows "Build passed" in green on the robot, nods, lights the LEDs
  as a progress bar, plays a chime: one `curl` each, no robot protocol to learn.
- **An AI agent** (Claude Code or any MCP client) gets a body on your desk: it shows short text,
  speaks in a bubble, asks you "Deploy now? Yes / Not yet" and gets the button you tapped, and
  hears when you pat the robot's head or hold a tag near it.
- **App authors** (Pet, Focus, the next one) use the same building blocks in Go: the robot
  connection, pairing codes, cards with good text, sounds, LED colours, and what a person did.

> **A proof of concept, vibe coded.** Written with AI agents and tested on real hardware at home,
> but neither the code nor its security has been reviewed by humans. Use it on your own network.
>
> **Early stage: no backward compatibility.** Protocols, APIs and stored settings change when
> something better comes along, without migrations.

Part of [i41eu-arch](https://github.com/i41-eu/i41eu-arch), a local first, privacy first platform
for a home. Stack-chan (スタックチャン) was created by Shinya Ishikawa and the Stack-chan community ([github.com/stack-chan/stack-chan](https://github.com/stack-chan/stack-chan)); this project is independent of it and of M5Stack.

**Start here:** [getting started](docs/getting-started.md): the robot set up with Dev, a key,
three variables in your shell, and the [examples](examples/) in curl, Python, JavaScript and Go,
and for an AI agent (MCP).

## How the pieces fit

| Piece | What it is | Needed? |
|---|---|---|
| **Robot** | a Stackchan with the [Embody Mode firmware](https://github.com/i41-eu/i41eu-StackChan/tree/embody/firmware/main/apps/app_embody_mode). On its screen the **app switcher** (the QR screen: Next, Connect) lists its apps and switches between them. | yes |
| **Apps** | what the robot does, one at a time. Each app is a web service the robot connects to, with a web page. This repo is **Dev**: the robot connects to it, and your programs reach the robot through it. | Dev |
| **Phone or browser** | opens Dev's page, pairs with the robot by scanning its QR code, and makes **keys** for your programs | once per key |
| **Your programs, AI agents** | use the robot through Dev's API with a key: HTTP (`/v1`) or MCP (`/mcp`) | the point |
| **Manager** | [i41eu-manager](https://github.com/i41-eu/i41eu-manager): sets robots up over USB and gives each its own token per app | optional |

## Use it

1. Run Dev (below) and give the robot Dev as one of its apps (its manager, or over USB:
   `i41eu-usb provision -url ws://<this computer>:8785 -name Dev -token-file <the token>`).
2. Open Dev's page signed in at the robot's manager (with `-manager-sign-in`: your robots show
   at once), or switch the robot to Dev (QR screen: Next, Connect) and scan its QR code with
   your phone or computer. Dev's page shows the robot.
3. **New key** on the page. It is shown once, with a `curl` line to try and the line that gives it
   to Claude Code:

   ```bash
   curl -s -X POST http://dev.local:8785/v1/robots/stackchan-0a1b2c3d4e50/show \
     -H "Authorization: Bearer sdev_…" -d '{"title":"Build","text":"All tests pass","color":"green","progress":1}'

   claude mcp add --transport http stackchan http://dev.local:8785/mcp --header "Authorization: Bearer sdev_…"
   ```

When it connects, the robot shows Dev's own card ("Dev · Ready"); then your programs' cards. The page's **Try it** runs every action by hand, and **Events** shows what people
do at the robot and what programs asked of it, as it happens.

## The API

Every action is `POST /v1/robots/<robot>/<action>` with a JSON object (all arguments optional
unless marked), answered `{"ok": true, …}` or `{"ok": false, "error"}` (400 a wrong argument, 404
not your robot, 501 the robot does not offer it, 503 the robot is offline). The same action is the
MCP tool `robot_<action>`. `GET /v1/actions` lists them with their arguments as JSON.

| Action | Arguments | What it does |
|---|---|---|
| `state` | | online, model, firmware, its commands, its readings (units in their names: `battery_pct`, `light_lux`, `head_yaw_deg`, …), the last events (also `GET /v1/robots/<robot>`) |
| `show` | `text`, `title`, `note`, `color` (the text's), `background`, `progress` 0..1, or `image` (JPEG/PNG, base64), or `clear` | a card on the screen: the text sizes itself to fit; or a picture, fitted to 320×240 |
| `say` | `text` (required, ≤ 80), `seconds` | a speech bubble over the face (after `show` `clear`) |
| `face` | `emotion` (neutral, happy, angry, sad, doubt, sleepy), `view` (face, readings) | the face, or the robot's own readings screen |
| `move` | `gesture` (nod, shake, home), or `yaw` −128..128 and `pitch` 5..85 (degrees) | the head |
| `leds` | `color`, `bar` 0..1, `pixels` (12), `effect` (rainbow, breathe, chase, blink, off) with `speed`, `seconds`; `off` | the 12 LEDs; screen colours corrected to look the same on the LEDs |
| `sound` | `chime` (start, done, break_over, chirp, tada, no, hello, morning, lullaby, yawn, whee, munch, ask, yes, error), or `notes` ("C5:120 E5:120 G5:300", `-` a rest) with `bell`; `volume` | a sound, played in parts the robot can buffer |
| `ask` | `question`, `answers` (2-4) required; `timeout_s` (30, ≤ 120) | the question and buttons on the screen; answers `{"answer", "index"}` when tapped, or `{"timed_out": true}` |
| `events` | `after` (a seq), `names`, `seconds` (≤ 60) | what people did, after `after`; waits up to `seconds` for the next (also `GET /v1/robots/<robot>/events?after=&names=&seconds=`) |
| `command` | `command` (required), `args` | any of the robot's own commands ([q41eu-raw](https://github.com/i41-eu/q41eu-raw#commands)), except those that switch its apps, restart it, or turn streams on |

**Events** are what a person did, made from the robot's raw events (the robot reports only
primary data, [package gesture](gesture/gesture.go)): `tap` {x, y}, `long_press`, `pat` (a short
touch on the head), `pet` {seconds}, `stroke` {direction}, `shaken`, `picked_up`, `near`, `away`
(proximity), `tag` {uid, type, text}, `tag_removed`, `remote` {address, command}, `button`
{press}, `plugged`, `unplugged`; plus `connected`. Each has a `seq`: ask for the ones after the
last you saw.

**OpenAPI:** `GET /v1/openapi.json` is the whole API as OpenAPI 3.1 (made from the same
actions): for API tools and client generators. For Go there is [`client`](client/client.go)
(`client.FromEnv()` reads `SDEV_URL`, `SDEV_ROBOT`, `SDEV_KEY`).

**MCP** (`POST /mcp`, JSON-RPC 2.0, the Streamable HTTP transport answered as plain JSON): the
tools above, on the key's robot, with instructions for the agent ("keep text on its screen short,
be calm: it sits next to a person who may be working").

**Keys** belong to one robot each, and only a browser paired with it (by its QR code) makes them.
Dev keeps only their SHA-256; the page lists them with when each was last used, and deletes them.
A robot with a key counts as used: it shows Dev, not its QR screen. For a robot set up by a
manager, Dev reports its keys there with its paired browsers ("API key: laptop"): the owner sees
them on the manager's page and removes them there too.

**Limits:** 10 actions a second per key or browser (bursts of 30), 20 wrong pairing codes per
address in 10 minutes, 20 open pages per address; over a limit the answer is 429 and says which.

## The Go kit

Packages for apps that the robot connects to ([pkg.go.dev](https://pkg.go.dev/github.com/i41-eu/q41eu-dev)):

| Package | What it does |
|---|---|
| [`robothub`](robothub/hub.go) | the robot side of an app: checks tokens (the shared one, or a manager's per robot through `robotauth`), keeps one connection per robot (a newer one replaces it), `Accepted` with the app's view, pairing codes (fresh after each use and every 5 min), `Paired`, pings; the app gets callbacks (connected, events, telemetry, binary streams) and sends commands and messages; `Report` tells the manager the paired browsers |
| [`robot`](robot/robot.go) | the higher-level calls on a connected robot: `ShowCard`, `Show`, `Clear`, `View`, `Say`, `Emotion`, `Look`, `Gesture`, `LEDs`, `Effect`, `Volume`, `Play` (paced) |
| [`screen`](screen/screen.go) | 320×240 pictures: `Card` (title, text that fits itself, note, progress), `Ask` (a question with buttons, `Hit` for taps), text in the Go fonts at any size, `JPEG` |
| [`sound`](sound/sound.go) | 16 kHz sounds: `Tone`, `Bell`, `Glide`, `Noise`, `Melody("C5:120 E5")`, named `Chimes`, `Chunks` for the speaker |
| [`leds`](leds/leds.go) | the 12 LEDs in the robot's order: `Bar`, `All`, `Correct` (screen colour to LED colour), `Parse` |
| [`gesture`](gesture/gesture.go) | the robot's raw events as what a person did (above) |

```go
hub := robothub.New(robothub.Config{Token: token, PublicURL: url,
	Connected: func(r *robothub.Robot) {
		robot.ShowCard(r, screen.Card{Title: "Hello", Text: "I am your app"})
		robot.LEDs(r, leds.Bar(0.5, leds.Names["green"]), true)
	},
	Event: func(r *robothub.Robot, ev wire.RobotEventBody) {
		for _, e := range rec.Feed(ev, time.Now()) { // a gesture.Recognizer per robot
			if e.Name == "pat" {
				robot.Play(ctx, r, sound.Chime("chirp"))
			}
		}
	}})
mux.Handle("GET "+wire.ConnectPath, hub)
```

## Run

```bash
go run ./cmd/q41eu-dev                  # listens on :8785
```

- The robot token is the shared one of the other apps (`~/.config/stackchan-server/robot-token`,
  made on the first run); `-manager-url` and `-manager-secret-file` let robots in that a manager
  set up, each with its own token, and report their paired browsers there.
- `-public-url` is the base URL in QR codes and the key's examples (default `http://<LAN IP>:8785`).
- `-state-file` (default `~/.local/state/q41eu-dev/state.json`) or `-state-database`
  (Postgres, q41eu-raw's `statestore`) keeps pairings and keys.
- `-ui-dir internal/devapp/ui` serves the page from disk (edits need only a reload).
- `-manager-sign-in` (with `-manager-url`): one sign-in for all the manager's apps; the owner's
  signed-in browsers get the robots they set up with Dev, without the QR code.
- `-trusted-proxies 1` behind one reverse proxy that appends the client address to
  `X-Forwarded-For`: the limits per address use it (the default 0 trusts no header).
- Container: `ghcr.io/i41-eu/q41eu-dev:<tag>` (the Dockerfile; tags `v*` build it).
- **End to end on a real robot:** `go run ./cmd/devcheck -usb <i41eu-usb> -o report -tap` runs
  every action through the API (`SDEV_*` set), takes the robot's own screenshot over USB after
  each, checks the pixels (the text's colour, the LEDs as the readings screen shows them, a
  question's buttons, a picture), answers a question with a USB tap, and writes `report/index.html`
  with every screenshot.
- `go test -race ./...` runs the tests: the hub against a fake robot, the app end to end (pairing,
  keys, actions, a question answered by a tap, events, MCP).

## License

Apache License 2.0 (see LICENSE). The Go fonts (golang.org/x/image/font/gofont) are BSD licensed.
