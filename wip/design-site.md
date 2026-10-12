# Dev's site and the dev package: design

**Status:** design, 2026-10-12. Nothing in it is built yet (docs describe only what exists;
this is a plan, so it lives in `wip/`). Names of new actions, events and files are proposals.

**The request** (the project's owner, 2026-10-12): a much better web site for dev.q41.eu.
Languages to pick at the top (several at once, all of them by default); every feature,
sensor and actuator of the robot covered by the API and by examples; a simple search; more
advanced apps shown; optional higher-level functions (menus, clickmaps, image uploading,
and others); the Focus and Pet apps as case studies for the dev package.

**In short:**

1. The site becomes a few pages (home, your robots, getting started, the API by feature,
   examples, helpers, advanced apps, case studies, coverage, security), built into Dev's
   binary, so a Dev at home has the same docs offline.
2. A language selector in the header: curl, Python, JavaScript, Go and MCP (AI agents),
   all on by default, remembered in the browser; every example is shown in each language
   picked, side by side (or in tabs).
3. A simple search over actions, arguments, events, readings, examples and pages, in the
   page itself (no service, no library).
4. A machine-readable catalog of the device protocol in `wire` (q41eu-raw), and a coverage
   table in Dev that a test checks against it: a new robot command, event or reading cannot
   go missing from Dev silently.
5. Thin client libraries for JavaScript and Python (and every action in Go's `client`),
   generated from Dev's action table, so a new action lands in every language at once.
6. Every robot feature reachable, one small release at a time, each with its examples;
   keys get scopes first, so the riskier features (infrared, files, camera) are opt-in.
7. Helpers on top: menus, clickmaps, image upload, countdowns, sequences (animations),
   `wait_for`, a "held" guard; in Dev (for every language) and in the Go kit (for apps).
8. Focus and Pet as case studies: their real code mapped to the kit; migrate them piece by
   piece rather than rebuild; small "mini" versions of both as examples.

## 1. Today

### 1.1 The site

dev.q41.eu runs Dev v0.6.2 with sign-in through m.i41.eu (the first visit goes to a silent
sign-in and back). It is one page, `internal/devapp/ui/index.html` (380 lines, CSS and JS
inline), the same as in the repository:

- the robots paired with this browser, each with **Try it** (a row of controls per action),
  **Keys** (new, list, delete) and **Events** (live, from `/api/events`);
- **Start here** (three steps) and **Examples**: hello, ask, build status, events, each with
  tabs for its languages (curl, Python, Go; build status has no curl) and **Run**, which runs
  a JavaScript copy of the example on the first paired robot;
- **The API**: the ten actions as collapsible rows with their arguments, and the event names.

Its other assets: `/examples/<lang>/<file>` (the example files as text), `/v1/actions`,
`/v1/openapi.json` (CORS open), `/api/state`, `/api/events`, `/mcp`.

What it lacks: a language choice across the page, search, an API reference by robot part,
the robot's readings, anything beyond the ten actions, advanced apps, and a JavaScript
client usable outside this page.

### 1.2 The API and the Go kit

Ten actions (`internal/devapp/actions.go`), each a REST call, an MCP tool, a row on the page
and an OpenAPI operation: `state`, `show`, `say`, `face`, `move`, `leds`, `sound`, `ask`,
`events`, `command`. `command` sends any robot command except a deny list (the robot's apps
and manager, standby, encryption, automation, the car switch, and the media and sensor
streams); its answers and events are not passed on. Events are what a person did, made by
`gesture` from the robot's raw events (15 names).

The Go kit for app authors: `robothub` (robot connections, tokens, pairing codes), `robot`
(cards, pictures, say, face, look, gestures, LEDs, effects, volume, paced sound), `screen`,
`sound`, `leds`, `gesture`. Focus and Pet already use `robothub` and `sound`.

### 1.3 Clients per language today

| Language | What exists | What it covers |
|---|---|---|
| curl (shell) | `examples/curl`: hello, ask, events, each with its own `act` function | any action, by hand |
| Python | `examples/python`: hello, ask, build status, events; standard library only, each file with its own `act` | any action, by hand; no package |
| Go | `client` (HTTP) and the four examples; the Go kit for apps | `client` has named methods for show (card), clear, say, face (emotion), gesture, look, LEDs (colour, bar, off), chime, notes, ask, events, state, and `Do` for anything; not named: pictures, the readings view, single LEDs and effects, volume, `command` |
| JavaScript | only inside Dev's page (`call()`, the page's cookie session) | every action by hand (Try it), the examples' Run; not usable from another page (no CORS for keys) or from Node |
| MCP (AI agents) | `/mcp` | all ten actions as `robot_<action>` tools |
| OpenAPI 3.1 | `/v1/openapi.json` | all actions, for generators (none tried) |

### 1.4 Drift found while writing this

The Stackchan's command list lives in four places, kept by hand: a comment in
`wire/wire.go`, the q41eu-raw readme (the catalog the spec in i41eu-arch points to), the
firmware's `Register`, and `fake-robot`:

- `fake-robot` registers 33 commands; the firmware lists 56 (23 missing: sprites, stored
  pictures and sounds, `light_stream`, `proximity`, `screen_snapshot`, the car, …).
- `screen_snapshot` and `speaker_flush` (both used by the pet) are in no command reference
  (the spec, the `wire` comment, the q41eu-raw readme, the firmware's README).
- The firmware sends the reading `camera_mic_off` but does not list it in `Register`.
- `screen_snapshot` answers with binary `0x07`, the same type as the camera's `snapshot`.

Section 6 turns this into one catalog and a test.

## 2. Inventory: what the robot offers, and what reaches a program

Sources: the firmware (i41eu-StackChan, branch embody: `app_embody_mode.cpp`, README,
HARDWARE), `wire/wire.go`, the q41eu-raw readme, `wire-protocol.md`. Columns:

- **Dev today**: **yes** = a Dev action (so HTTP, MCP and every client); **raw** = only
  through `command` (works, but no client names it and its answers and events are dropped);
  **partly** = only its readings (through `state`), not its events; **no** = a program cannot
  reach it through Dev; **owner** = deliberately not for programs.
- **Go kit**: **helper** = a kit function does it; **hub** = an app can do it through
  `robothub` with the raw protocol, no helper.

### 2.1 Sensors and inputs

| # | Feature | Part (chip) | Protocol | Dev today | Go kit |
|---|---|---|---|---|---|
| S1 | Taps and long presses on the screen, with the sprite hit | touch screen (FT6336U) | events `screen_tap`, `screen_long_press` | **yes**: `tap`, `long_press` | helper (`gesture`) |
| S2 | Raw multi-touch: fingers down and up, 50 Hz frames | touch screen | events `touch_down`, `touch_up`; `touch_stream` + binary `0x06` | **no** (events dropped, stream denied) | hub |
| S3 | Pat, pet, stroke on the head | head touch (Si12T, 3 zones) | events `head_release`, `head_swipe_forward`, `head_swipe_backward` | **yes**: `pat`, `pet`, `stroke` | helper (`gesture`) |
| S4 | Head zones and their pressure | head touch | event `head_press {z0,z1,z2}`; readings `head_zone0..2` | **partly**: the readings (every 2 s); the event is dropped | hub |
| S5 | Shaken, picked up | IMU (BMI270) | events `shake`, `pickup` | **yes**: `shaken`, `picked_up` | helper |
| S6 | Motion and compass readings | IMU, magnetometer (BMM150) | readings `imu_a{x,y,z}_g`, `imu_gyro_dps`, `mag_{x,y,z}_ut`, `mag_raw_*`, `mag_rhall` | **yes**: `state` (every 2 s) | hub |
| S7 | Motion at 100 Hz | IMU, magnetometer | `imu_stream` + binary `0x05` | **no** (denied) | hub |
| S8 | Room light and proximity readings | LTR-553 | readings `light_lux`, `light_ch0/1`, `proximity`, `proximity_on`, `auto_brightness` | **yes**: `state` | hub |
| S9 | Someone near, gone again | LTR-553 | events `proximity_near`, `proximity_far` | **yes**: `near`, `away` | helper |
| S10 | Light and proximity at 20 Hz (a hand over the sensor) | LTR-553 | `light_stream` + binary `0x08` | **no** (denied) | hub (the pet parses it) |
| S11 | NFC tags and cards | ST25R3916 | events `nfc_tag`, `nfc_removed` | **yes**: `tag`, `tag_removed` | helper |
| S12 | Infrared remotes | IR receiver | event `ir_received` | **yes**: `remote` | helper |
| S13 | Camera video | GC0308 | `camera {on,size}` + binary `0x01`; `camera_failed` | **no** (denied) | hub |
| S14 | Camera still, 640×480 | GC0308 | `snapshot` + binary `0x07`; `snapshot_failed`; `camera_config`; `camera_reg` + event | **no** (the picture is dropped); `camera_config` raw | hub |
| S15 | Microphones | ES7210 | `mic {on}` + binary `0x04` (`0x02` mono) | **no** (denied) | hub |
| S16 | Battery, charging, USB, power button | AXP2101, INA226 | readings `battery_pct`, `charging`, `core_*`, `pmic_*`, `body_*`, `usb_data`; events `usb_plugged`, `usb_unplugged`, `power_button`, `battery_inserted`, `battery_removed` | **yes**: `state`, `plugged`, `unplugged`, `button` (battery in/out dropped) | helper |
| S17 | Servo feedback: angles, load, temperature, voltage | SCS0009 ×2 | readings `head_yaw_deg`, `head_pitch_deg`, `{yaw,pitch}_{load_pct,temp_c,pos_raw,speed_raw,current_raw,moving}`, `servo_voltage_v`, `pitch_voltage_v` | **yes**: `state` | hub |
| S18 | Clock, processor, Wi-Fi, memory, uptime | BM8563, ESP32-S3 | readings `rtc_unix`, `system_unix`, `chip_temp_c`, `wifi_rssi_dbm`, `free_heap_kb`, `free_internal_kb`, `largest_internal_kb`, `uptime_s` | **yes**: `state` | hub |
| S19 | The robot's own camera and microphone switch | (setting on the robot) | events `privacy`, `privacy_refused`; reading `camera_mic_off` | **partly**: the reading; the events are dropped | hub |
| S20 | The app paused (the robot's QR or Manager screen open) | (Embody Mode) | events `paused`, `resumed` | **no** (dropped) | hub |
| S21 | Latency | (connection) | `ping` → `RobotPong` | **no** (the pong is dropped) | hub (the hub drops it too) |

### 2.2 Outputs

| # | Feature | Part | Protocol | Dev today | Go kit |
|---|---|---|---|---|---|
| A1 | Cards: title, text that fits itself, note, colours, progress | display | binary `0x10` (Dev draws it) | **yes**: `show` | helper (`screen.Card`) |
| A2 | Any picture, fitted to 320×240 | display | binary `0x10`; `face` takes it away | **yes**: `show image`, `show clear` | helper |
| A3 | The robot's own screens: face or readings | display | `view` | **yes**: `face view` | helper |
| A4 | Emotions | display | `emotion` | **yes**: `face emotion` | helper |
| A5 | Speech bubble | display | `say` | **yes** | helper |
| A6 | Stickers over the face | display | `sticker` | **raw** | hub |
| A7 | Brightness, automatic brightness | display | `brightness`; reading `brightness_pct` | **raw** | hub |
| A8 | Screen off and on | display | `screensaver`; events `screensaver_on/off`; reading `screensaver` | **raw** (events dropped) | hub |
| A9 | What the screen shows now, as a picture | display | `screen_snapshot` + binary `0x07`; `screen_snapshot_failed` | **no** (dropped) | hub |
| A10 | Sprites: stored pictures over the face that glide, turn, fade, and work as buttons | display | `sprite`, `sprite_hide`, `sprite_clear`; `sprite_error` | **no** in practice: `command` sends them, but a program cannot put a picture on the robot | hub (the pet) |
| A11 | A stored picture instead of the face | display | `picture` | **no** in practice (same) | hub |
| A12 | The robot's file store (1.9 MB): upload, list, delete | flash | binary `0x11`; `assets`, `asset_delete`; events `assets`, `asset_saved`, `asset_deleted`, `asset_error` | **no** (no upload; answers dropped) | hub (the pet syncs its files) |
| A13 | Nod, shake, face forward, look at an angle | head servos | `nod`, `shake`, `home`, `look` | **yes**: `move` | helper |
| A14 | Hold the head powered; servo power off | head servos | `hold`, `servo_power`; events `hold_on/off`, `servo_power_on/off`; readings `hold_s`, `servo_power` | **raw** (events dropped) | hub |
| A15 | Turn round continuously (asked on the robot) | yaw servo | `rotate`; events `rotate_asking`, `_confirmed`, `_refused`, `_stopped`, `_on`, `_off`, `_trace`; reading `rotate_s` | **raw** (events dropped) | hub |
| A16 | 12 LEDs: colour, a bar, single LEDs, effects | WS2812 ×12 | `leds` with `left`/`right`, `pixels` or `effect` | **yes**: `leds` (side fades only as pixels) | helper (`leds`) |
| A17 | Chimes and melodies | speaker (AW88298) | binary `0x03` (Dev makes the sound) | **yes**: `sound` | helper (`sound`) |
| A18 | Volume | speaker | `volume`; reading `volume_pct` | **yes**: `sound volume` | helper |
| A19 | Any audio: a WAV, speech, a stream | speaker | binary `0x03`; `speaker_flush` | **no** (only Dev's own sounds) | helper (`robot.Play`) |
| A20 | Stored sounds | speaker | `play`, `play_stop`; events `sound_done`, `sound_error` | **no** in practice (no upload) | hub |
| A21 | Send infrared (TVs, lights) | IR LED | `ir_send` | **raw** | hub |
| A22 | The red power LED | AXP2101 | `power_led` | **raw** | hub |
| A23 | NFC reader and proximity sensor on or off | ST25R3916, LTR-553 | `nfc`, `proximity` | **raw** | hub |
| A24 | The TPBot car (an optional extension) | BLE | `car_drive`, `car_stop`, `car_servo`, `car_headlights`, `car_sonar`, `car_watchdog`, `car_board`; readings `car_*` (13); events `car_connected`, `car_disconnected` | **raw** while the owner has the car on (`car_enable` denied); readings **yes**; events dropped | hub |

Dev's own higher-level features on top: `ask` (a question with buttons; the tap comes back)
and events as what a person did. Both **yes**.

### 2.3 The robot's own business (not for programs, by design)

| Area | Protocol | Dev today |
|---|---|---|
| The robot's apps and its manager | `server_switch`; events `servers`, `apps_updated`, `manager_changed`, `manager_on/off`, `server_asking`, `server_refused` | **owner** (denied) |
| Standby | `standby`; events `standby`, `standby_end` | **owner** (denied) |
| End-to-end encryption | `server_e2e`, `e2e_forget`, `E2E*` frames, binary `0x30`/`0x31`; events `e2e_forgotten`, `e2e_refused` | **owner** (Dev is a plaintext app, like the pet) |
| Automation | `automation`, `restart`, `launch`; events `automation`, `launch_unknown` | **owner** (denied) |
| The car switch | `car_enable` | **owner** (denied) |

### 2.4 Counts

By feature (sections 2.1 and 2.2, 45 rows): **20 yes, 11 partly** (9 raw, 2 readings only),
**14 no**; plus the 5 owner-only areas of 2.3.

By message name, as the firmware sends and accepts them:

| Kind | All | yes | raw | no | owner |
|---|---|---|---|---|---|
| Commands | 56 | 11 | 27 (`speaker` counted here: only Dev's sounds) | 10 | 8 |
| Events | 63 | 15 | – | 35 | 13 |
| Readings | 80 (67 + 13 of the car) | 80 (through `state`, the last value, every 2 s; no "tell me when") | – | – | – |
| Binary types | 12 | 1 (`0x10`) | 1 (`0x03`) | 8 | 2 (`0x30`, `0x31`) |

The **no** commands: `camera`, `mic`, `imu_stream`, `touch_stream`, `light_stream` (denied),
`snapshot`, `screen_snapshot`, `assets`, `camera_reg`, `ping` (their answers are dropped).

## 3. Languages

### 3.1 Proposed set

| In the selector | For whom | How it reaches Dev | Why |
|---|---|---|---|
| **curl** | anyone with a terminal; the shortest proof that it works | one line per action | exists; the API's own words, no library |
| **Python** | makers, kids learning to program (schools teach Python), a Raspberry Pi next to the robot | a single file `q41dev.py`, standard library only | exists as examples; the most likely second language of a family's makers |
| **JavaScript** | anyone with a browser: a page of your own on your computer, Node scripts | an ES module `dev.js` (browser and Node 18+, only `fetch`) | the page already runs examples in JS; it is the one language every phone and laptop runs; the base for blocks later |
| **Go** | app authors, the platform's own language (principle 24) | `client` (programs) and the kit (apps) | exists; the kit is Go |
| **MCP** (AI agents) | Claude Code and other agents | the tool call `robot_<action>` with its arguments, and a sentence to ask the agent | exists; an agent is a user of the API like a script |

Not now, each with its reason: **blocks** (Blockly) for kids who do not type yet: later, on
top of the JavaScript library (a decision for the owner, section 11); **MicroPython or
Arduino** on another board: the robot itself is the board, programs run elsewhere;
**Rust, C#, Java, PHP**: nobody asked; the OpenAPI document serves them.

### 3.2 How the libraries are made

- **One source:** Dev's action table (`internal/devapp/actions.go`) already makes the REST
  endpoints, the MCP tools, the page's list and OpenAPI. A generator (`go generate`,
  `cmd/gensdk`) writes from it one method per action in each library, with the help text as
  its comment: `sdk/js/dev.js`, `sdk/python/q41dev.py`, `client/actions.go`.
- **Helpers by hand,** in a file of their own per language: the event loop (an async
  iterator in JS, a generator in Python, a channel or callback in Go), image loading for
  upload, small conveniences. Thin: logic that needs timing or state is in Dev (section 7).
- **No package registry at first:** Dev serves the files itself (`/sdk/dev.js`,
  `/sdk/q41dev.py`), so a Dev at home serves the version that matches it, and the repository
  has them. `import { robot } from "https://dev.q41.eu/sdk/dev.js"` works in a page; Python
  is one file to copy. npm and PyPI are a later decision.
- **The API's snippets** (one call of each action, with its example arguments) are generated
  for every language from the action table, so the API reference is complete by
  construction. **Examples** (whole programs) are written by hand and tested (section 6.4).

## 4. Who uses the site

From i41eu-arch's personas: the owner at play (quick experiments), the kids (through what
grown-ups build: a quiz, a pet), a developer (clear protocols, a fake robot), the owner's AI
agents (narrow grants), and app authors (the next Focus or Pet). What they need from the
site, from the most common to the rarest: copy an example and see the robot react within
minutes; find how to do one thing (search); understand a feature fully (the API by part);
build an app (the kit, the case studies).

## 5. The site

### 5.1 Rules

- **Local first.** The pages, the examples, the libraries and the search index are in Dev's
  binary. A Dev at home has the whole site without the internet.
- **One source.** Pages are made from the same data as the API: the action table, the
  protocol catalog (section 6.1), the coverage table, the examples' manifest. Prose is in
  small HTML fragments next to the page templates.
- **Works without JavaScript** except the search, Run and Try it: all languages are shown by
  default, so the language selector only hides.
- **Go only, as now:** `html/template` and `embed`; no framework, no build step for the pages,
  no new dependencies.
- **Plain and calm,** as the pages of the other apps: the existing colours and dark mode,
  phone width first.
- **Only what exists.** Like the repo's docs, the site describes what Dev does now: a page or
  a section comes with the release that makes it true; plans stay in `wip/`.

### 5.2 Pages

| Path | Page | What is on it |
|---|---|---|
| `/` | Home | what Dev is in three lines, the three steps (robot, key, first program), the hello example in the languages picked, links to everything; your robots in brief when paired |
| `/robots` | Your robots | today's robot card: Try it (generated from the actions' arguments, so every new action is tryable at once), keys (with scopes, section 9), events, and the robot's readings live |
| `/docs/start` | Getting started | today's `docs/getting-started.md`, with every command in the languages picked |
| `/docs/api` | The API by part | one section per part of the robot (section 5.5), what it does first, then what it senses: screen, face, head, LEDs, speaker, infrared, files; touch and head touch, motion, light and proximity, NFC, camera, microphone, power; the car |
| `/docs/events` | Events | every event a program gets: its data, the raw event it comes from, an example |
| `/docs/readings` | Readings | every reading: unit, part, range, how often (from the catalog) |
| `/docs/helpers` | Helpers | menus, clickmaps, image upload, countdowns, sequences, `wait_for`, the held guard (section 7) |
| `/docs/examples` | Examples | the gallery: cards with tags (the parts used), the code in the languages picked, Run |
| `/docs/apps` | Advanced apps | bigger programs and the apps built on the kit (section 5.8) |
| `/docs/case-studies/focus`, `/docs/case-studies/pet` | Case studies | section 8 |
| `/docs/kit` | The Go kit | for app authors: `robothub` and the other packages, a minimal app |
| `/docs/coverage` | Coverage | the inventory of section 2 as it is now, made from the coverage table: part × language |
| `/docs/security` | Keys and privacy | section 9 for developers |
| `/docs/protocol` | Lower level | the device protocol, with links to the spec and Raw data |

`/pair` keeps redirecting to `/robots?paired=…`. The header on every page: Dev, the page
links, the search box, the language chips, the account.

### 5.3 The language selector

- **Chips in the header:** `curl`, `Python`, `JavaScript`, `Go`, `MCP`. Several can be on;
  **all are on by default**; at least one stays on (turning off the last turns all on).
- **Remembered per browser** in `localStorage` (`dev.langs`, a list), wrapped in try/catch: a
  browser without storage gets all languages every time. `?lang=python,go` in the address wins
  for that page, so a link can show one language; the chips then offer "keep this".
- **Applied before the first paint:** a small script in `<head>` sets classes on `<html>`
  (`lang-python lang-go`); CSS hides the code of the others. No flash, no layout jump.
- **Side by side or tabs:** a switch next to the chips, remembered too. Side by side: the
  picked languages in a grid (`repeat(auto-fit, minmax(320px, 1fr))`): two or three
  across on a laptop, one under the other on a phone. Tabs: one at a time per example, the
  tab picked follows across the page.
- **Every code block:** the language's name, Copy, Download (where it is a whole file), and
  Run for examples (it runs the JavaScript version on your robot, whichever language is
  shown). A language an example does not have yet says so ("not in Go yet") with a link to
  the coverage page, instead of hiding it.

### 5.4 Code blocks and Run

- Run uses the JavaScript library and the page's own session (no key needed in the page);
  it shows what the program prints and the robot's events meanwhile. Today the page keeps a
  separate JavaScript copy of each example for Run; with the library, Run runs the
  JavaScript example file itself, so what you read is what runs.
- Snippets in the API reference are generated; examples are files from `examples/`.

### 5.5 The API by part

Each part's section, made from the catalog and the action table:

1. What the part is (one sentence, a photo of where it sits on the robot, a link to the 3D
   model on s.i41.eu).
2. What a program can do with it: the actions (arguments, ranges, defaults), the events it
   causes, the readings it reports, with units.
3. A snippet per action in the languages picked; links to the examples that use the part.
4. Limits and safety in plain words (pitch 5–85°, the robot asks before rotating, the
   camera only with its scope and the LIVE badge, the IR LED is weak).
5. Its row of the coverage table: which languages have it, what is still missing.

### 5.6 Search

- **A box in the header;** `/` focuses it, Enter opens the first result, Esc closes. Results
  appear under it as you type, grouped: Actions, Arguments, Events, Readings, Helpers,
  Examples, Pages.
- **The index** (`/docs/search.json`, made at start from the same data, a few tens of KB)
  has an entry per action, argument, event, reading, helper, example and page heading:
  name, kind, a short text, the page and anchor, the parts it belongs to.
- **Matching:** words of the query against names (exact, then prefix) and texts (contains);
  names weigh more; a small synonym table helps people who do not know our words
  ("light" → LEDs and `light_lux`, "photo" → snapshot and image upload, "touch" → tap, pat,
  touch stream, "speak" → say, sound, speech, "colour"/"color"). About a hundred lines of
  JavaScript; no library, no server.
- Search works offline on a Dev at home, like the rest.

### 5.7 Examples

Each example: a title, what it shows in one sentence, its tags (the parts and helpers it
uses), the code in every language it exists in, Run, and what to expect on the robot. The
gallery filters by tag. Today's four stay; each new feature brings one (section 10). A
manifest (`examples/examples.json`) lists them; the tests read it (section 6.4).

### 5.8 Advanced apps

Programs longer than an example, each using several parts, each in every language unless
it needs a library a language lacks:

| App | Shows | Needs |
|---|---|---|
| **Focus, small** | a focus timer as a Dev program: a countdown ring, the LED bar, tap to pause, a chime and a nod | `countdown`, events, `leds`, `sound`, `move` |
| **Pet, small** | needs that drop, a mood face, feeding by NFC card, a menu of food on the screen | `menu`, files, events, `face`, `say` |
| **Quiz** | questions with picture answers, an NFC card as a joker, the score on the LEDs | `menu` or `clickmap`, image upload, `leds` |
| **Remote learner** | learn a TV remote's buttons, then a menu on the robot sends them | `remote` events, `ir`, `menu` |
| **Drawing pad** | draw with a finger on the robot's screen; the program keeps the drawing | the touch stream, `show image` |
| **Night light** | warm light when the room gets dark, a sleep timer, a soft melody | `wait_for light_lux`, `leds` effects, `sound` |
| **Dance** | head, LEDs and notes in time | `sequence` |
| **Agent desk buddy** | an AI agent that shows what it works on and asks before it pushes | MCP: `show`, `ask`, `events` |

Plus links to the apps built on the kit: Focus, Pet, Raw data (every sensor in a browser),
Sbot (a cockpit and the TPBot car), with what each one shows a developer.

### 5.9 Building the pages

`internal/devapp/site/`: one template per page, HTML fragments for prose, the shared header
and CSS; rendered once at start (and from disk with `-ui-dir`). The readme keeps its short
API table, checked by a test against the action table (today it is kept by hand).

## 6. API coverage

### 6.1 The protocol catalog (q41eu-raw, package `wire`)

`wire/catalog.go`: the device protocol as data, next to the frame types.

```go
type Command struct {
	Name  string // "look"
	Part  string // "head" (the parts as on Raw data's dashboard and the 3D model)
	Kind  string // "actuator", "stream", "query", "system", "owner"
	Args  []Arg  // name, type, unit, min, max, enum, help
	Help  string
	Reply string // the event or binary type that answers it ("assets", "0x07"), if any
}
type Event struct{ Name, Part, Kind, Help string; Data []Arg }      // Kind: "input", "answer", "status", "owner"
type Reading struct{ Name, Part, Unit, Help string; Min, Max float64 }
type Binary struct{ Type byte; Name, Dir, Help string }
var Catalog = struct{ Commands []Command; Events []Event; Readings []Reading; Binary []Binary }{…}
```

- The spec (i41eu-arch `wire-protocol.md`) stays the reference for frames; it says that the
  Stackchan catalog is `wire.Catalog`, and the q41eu-raw readme's Commands table is checked
  against it by a test (the AGENTS rule "spec first, then wire, then the readme" stays).
- `fake-robot` registers the catalog's commands and readings (and says which it only
  pretends), so it can no longer fall behind.
- The firmware is C++ and cannot import Go: `devcheck` compares a real robot's `Register`
  with the catalog (commands and readings both ways, build options marked), and Dev logs
  and shows on `/docs/coverage` any command or reading a connected robot lists that the
  catalog does not know.

### 6.2 Dev's coverage table and its test

`internal/devapp/coverage.go`: a row for every catalog entry:

```go
{Wire: "ir_send", Via: "ir", Scope: "ir"},                          // reached by an action
{Wire: "camera", Via: "-", Why: "streams are Dev's to switch; see snapshot"},
{Wire: "server_switch", Owner: true, Why: "the robot's apps are its owner's"},
{Wire: "touch_down", Planned: "F8"},                                 // a step of this plan
```

- `TestCoverage`: every command, event, reading and binary type of `wire.Catalog` has
  exactly one row; every `Via` names an existing action (and argument); no row is left
  `Planned` for a step marked done. A new message in q41eu-raw fails Dev's tests on the
  next `go get`, so it gets a decision, not silence.
- The same table makes `/docs/coverage`, `/v1/coverage` (JSON) and section 2's inventory,
  so the site's coverage page cannot disagree with the code.

### 6.3 Every action in every language

- The generator of section 3.2 writes the libraries; `TestGenerated` checks that the
  committed files are what the generator makes now (a new action without regenerating
  fails).
- MCP and OpenAPI already come from the action table.
- The coverage page shows part × language; for generated methods it is always complete,
  for helpers and examples it shows the gaps.

### 6.4 Examples are tested

- `examples/examples.json`: per example, its id, title, tags (catalog parts and helpers) and
  its file per language.
- `TestExamples` starts Dev in the test with a fake robot (the hub's test robot), makes a
  key, runs each example in each of its languages (`sh`, `python3`, `node`, `go run`) with a
  timeout and the `SDEV_*` variables, and checks the commands the robot received. A runner
  without a language skips it and says so; CI has all four.
- `TestCoverage` also wants an example for every part that is reachable.
- `devcheck` (a real robot, USB screenshots) gets a step for every new action.

### 6.5 New actions, by feature

Names follow today's style (short verbs, one per part); they shield programs from the
wire's planned renames (i41eu-arch `device-api.md`: `led`, `servo`, `display`, `speaker`).

| Feature (section 2) | Proposed | Scope | Phase |
|---|---|---|---|
| A6 stickers | `face {sticker, seconds}` | basic | F1 |
| A7, A8 brightness, screen off | `screen {brightness, auto_brightness, off}`; events `screen_off`, `screen_on` | basic | F1 |
| S19, S20 privacy, paused | events `privacy {camera_mic}`, `paused`, `resumed` | basic | F1 |
| A12 files | `POST /v1/robots/<id>/files/<name>` (the file as the body), `files` (list, free space), `files {delete}`; events `file_saved`, `file_error` | files | F2 |
| A11 stored picture | `show {file}` | basic | F2 |
| A10 sprites | `sprite {id, file, x, y, scale, angle, opacity, z, ms, tap}`, `sprite {hide}`, `sprite {clear}`; `tap` already carries the sprite | basic | F3 |
| A19, A20 audio | `sound {file}`, `sound {wav}` (base64, resampled by Dev), `sound {stop}`; events `sound_done`, `sound_error` | basic (`files` to store) | F4 |
| A14, A15 head | `move {hold}`, `move {power}`, `move {rotate, seconds}` (asked on the robot); events `rotation {state}` | motion | F5 |
| A21, A22, A23 | `ir {address, command}` or `ir {raw}`, with `repeat`, `frames`; `sensors {nfc, proximity}`; `leds {power_led}` | ir; basic | F6 |
| S6, S8, S17 readings | `wait_for {reading, above or below, seconds}`; `state {readings: [...]}` | basic | F7 |
| S2, S4, S7, S10 streams | `GET /v1/robots/<id>/stream?touch=1&imu=1&light=1` (newline-delimited JSON, the robot's stream on only while read); events `touch_down`, `touch_up`, `head_press {zones}` | sensors | F8 |
| A9 screen picture | `screenshot` → JPEG | basic | F9 |
| S21 latency | `ping` → milliseconds per leg | basic | F10 |
| S14 camera still | `snapshot` → JPEG | camera | 14 |
| S15 microphone | `listen {seconds ≤ 10}` → WAV | mic | 14 |
| A24 the car | `drive {left, right, seconds}` with a lease (wip/extensions.md) | car | 15 |
| S13 video, S15 streaming | not through Dev at first (section 9.4) | – | – |

## 7. Helpers

**Where a helper lives:** logic that needs the robot's timing or Dev's state (taps matched
to buttons, steps played on time, files kept in sync) runs **in Dev**, as an action, so
every language and every agent gets it with one call. Pure conveniences (loading an image
file, an event loop) are in the **libraries**. Everything Dev does is built from **kit
packages**, which apps (Focus, Pet) use too. Each helper below says where its code comes
from.

### 7.1 Menus

```json
POST …/menu {"title": "Snack?", "items": [
  {"id": "apple", "label": "Apple", "icon": "apple.png"},
  {"id": "cake", "label": "Cake", "emoji": "cake"},
  {"id": "more", "label": "More…"}], "timeout_s": 15, "wait": true}
→ {"ok": true, "item": "cake"}   or {"timed_out": true}
```

- Tiles in a grid that fits the count (up to 6 per screen; more pages with arrows), a back
  arrow when `back: true`; each tile is a sprite with `tap`, so the robot reports which one
  was hit (no coordinates to guess). Icons: a file uploaded before (`icon`), a built-in
  emoji (`emoji`, the Fluent set the apps use), or the label drawn as a tile.
- Dev renders the tiles once, uploads them to the robot's store under its folder and
  reuses them (by checksum) next time; a robot without sprites gets the menu as one
  picture with a clickmap instead.
- `wait: false` returns at once; the choice comes as the event `menu {menu, item}`. Nested
  menus: the program opens the next one on `more`. The menu closes after `timeout_s`, on a
  choice, or with `menu {close}`.
- `ask` stays; it is a menu of text buttons.
- **From the pet:** `internal/app/menu.go` (tiles as `tap` sprites, ids `m:<action>`, back,
  a 15 s timeout, the face behind it); kit package `menu`.

### 7.2 Clickmaps

```json
POST …/clickmap {"id": "map1", "image": "<base64>" or "file": "world.png",
  "regions": [{"id": "sea", "rect": [0,0,320,80]}, {"id": "sun", "circle": [260,40,30]},
              {"id": "hill", "polygon": [[0,240],[160,140],[320,240]]}],
  "once": false, "timeout_s": 60}
```

- Shows the picture and reports taps inside a region as the event `click {map, region, x,
  y}` (or answers the call with the first one when `once`); taps outside any region come as
  plain `tap`. A region list without a picture works over whatever the screen shows (the
  face, a card).
- **From:** `screen.Button`/`screen.Hit` (Dev's `ask`), and the pet's quarters
  (`robotpic.SpotAt`, catch the ball).

### 7.3 Image upload

- `POST /v1/robots/<id>/files/<name>` with a JPEG, PNG or GIF (first frame) as the body;
  `?fit=cover|contain|none&size=WxH&as=picture|sprite`. Dev decodes it, fits it (cover
  crops the middle, as `screen.Fit` does today; contain letterboxes), and stores a JPEG for
  full-screen pictures or a PNG with transparency for sprites. Re-encoding drops the
  photo's metadata (a phone photo's location never reaches the robot).
- Kept under Dev's folder on the robot, per key (`dev/<key id>/<name>`), so deleting a key
  deletes its files; listed with sizes and the free space (the store is about 1.9 MB,
  shared, no quotas: i41eu-arch `device-storage.md`).
- Libraries: `robot.upload("cat.jpg", as: "picture")` reads the file (a path in Python and
  Go, a `File` or `Blob` in the browser).
- **From:** the pet's `internal/app/assets.go` (list with checksums, upload what is missing
  or changed, delete what is no longer used, pace the chunks behind commands, check the
  checksum after) as kit package `assets` (`Sync`), used by Dev and then by the pet.

### 7.4 Countdowns and timers

- `countdown {seconds, title, color, leds: true, sound: "done", nod: true}`: Dev shows a ring
  with the time left, the LED bar shrinking, and at the end a chime, a nod and the event
  `countdown_done`; tap pauses and resumes it (events `countdown_paused`, `_resumed`),
  `countdown {stop}`. It runs in Dev, so a script may exit and the robot keeps counting.
- **From Focus:** the ring and digits (`internal/screen`), the LED bar (`screen.LEDs` is
  exactly `leds.Bar(f, leds.Dim(c, 0.25))`), the chimes. Focus's Pomodoro rules
  (`internal/timer`) stay Focus's.

### 7.5 Sequences (animations, choreography)

- `sequence {steps: [{at_ms: 0, action: "leds", args: {…}}, {at_ms: 400, action: "move", …}],
  repeat: 1}`: Dev plays the steps on time, near the robot (no network jitter between steps),
  answers when done or returns at once with `wait: false`; `sequence {stop}`; a newer
  sequence replaces the last one. Steps are any actions the key may use.
- Sprite steps glide with the robot's own `ms`; LED effects run on the robot.
- **From the pet:** `later`/`begin` and the generation counter (`engine.go`: a newer
  reaction cancels the pending steps of the last one), the food that glides into the mouth
  (`eatSprite`), the head's circle and wander (`game.go`), the sunrise and night-light fade.
  Kit package `scene` (a cancellable schedule per robot).

### 7.6 Waiting for readings and events

- `wait_for {reading: "light_lux", below: 5, seconds: 600}` answers when the reading crosses
  the value, or times out (wip/next.md lists it).
- Event loops in every library: `for await (const e of robot.events({names: ["tap"]}))` in
  JavaScript, `for e in robot.events("tap", "pat"):` in Python, `robot.Events(ctx, …)` in
  Go: the `after`/`last_seq` bookkeeping and reconnects done once.
- State machines: a page in the docs shows the pattern (states, events, timeouts) with the
  small Focus as the example, in each language; no framework.

### 7.7 Held, and other things people do

- `gesture` learns two more: `held` and `put_down`, from the motion readings (the pet's
  `held.go`: tilt from the rest pose, turning, a shake). While held, Dev drops head moves
  (as the pet does): a robot in a kid's hands does not twist. On for every key; a program
  sees `held` and can wait.
- `gesture` gets a debounce (Focus's one start per touch however it is reported, 1.5 s) and
  `hand_near` from the light stream (the pet's `handNear`: proximity rise or a shadow), the
  stream on only while a program waits for it.

### 7.8 Speech (optional)

- `speak {text, lang}` turns text into speech in Dev and plays it with the bubble. The pet's
  `internal/voice` (espeak-ng locally; the Edge voice sends the text to a third party) moves
  to the kit. On dev.q41.eu only the local voice; another voice is a home Dev's choice.

## 8. Case studies

### 8.1 Focus (q41eu-focus)

A focus timer: about 2,400 lines of Go and HTML without tests (520 of tests). Already on the
kit's `robothub` and `sound`.

| Focus today | With the dev package | Effect |
|---|---|---|
| `robot.go` `newHub`: connections, tokens, pairing codes | `robothub` (done) | – |
| `robotEvent`: tap or head touch toggles, with a 1.5 s gap; long press skips | `gesture` events with the debounce (7.7) | ≈ −20 lines, the rule shared |
| `reactLocked`: chimes, a nod, a look up | `robot.Play`, `robot.Gesture`, `robot.Look` (today raw commands) | clearer |
| `showLocked`: send the picture and LEDs only when changed | kit `robot.Shown` (a per-robot cache of the last picture and LEDs) | the same need in Pet and Dev |
| `screen.LEDs` | `leds.Bar(f, leds.Dim(c, 0.25))` | ≈ −20 lines |
| `screen.Frame`: the ring and a 5×7 pixel font | stays (its look); a `screen.Ring` in the kit for others | – |
| `timer`: the Pomodoro rules, pure, tested | stays: the app's own logic | – |
| `signin.go`, `pairings.go`, sessions and the event stream in `app.go`/`web.go` (≈ 400 lines) | kit package `webapp`: sessions, pairing by QR, sign-in through the manager, reporting pairings, the page's event stream (Dev, Focus and Pet each have their own copy now) | ≈ −350 lines |

**As a Dev program** ("Focus, small", an advanced example): a loop over events with three
states, `countdown` for each phase, `leds` and `sound` from it, in each language; about 60
lines. It shows the idea; it is not Focus: Focus runs all the time, keeps statistics and
settings per robot, and serves a page.

```python
robot = q41dev.from_env()                      # proposed library; names are proposals
phase, minutes = "focus", {"focus": 25, "break": 5}
robot.countdown(seconds=minutes[phase] * 60, title=phase.upper(), color="red", wait=False)
for e in robot.events("countdown_done", "long_press"):
    phase = "break" if phase == "focus" else "focus"
    robot.countdown(seconds=minutes[phase] * 60, title=phase.upper(),
                    color="green" if phase == "break" else "red", wait=False)
```

**Rebuild?** No. Move it to `webapp`, the debounce and `leds.Bar` as those land (one small
release each, its tests guarding); keep its timer and screen.

### 8.2 Pet (q41eu-pet)

A Tamagotchi for kids: about 7,900 lines without tests (2,400 of tests, the app test alone
1,300). On `robothub` and `sound`; everything else its own.

| Pet today | With the dev package | Effect |
|---|---|---|
| `menu.go` (144): tiles as `tap` sprites, nested menus, back, timeout | kit `menu` (7.1); the pet keeps its tiles and what they do | ≈ −90 lines |
| `assets.go` (216): its files on the robot, synced by checksum | kit `assets.Sync` (7.3) | ≈ −150 lines |
| `held.go` (75): no head moves in a kid's hands | `gesture` `held` and the guard (7.7) | ≈ −75 lines, and every app gets it |
| `game.go`: the quarters of catch the ball; the hand seen by the light sensor; the head's circle, wander, dodge | `clickmap` (7.2), `gesture` `hand_near` (7.7), `scene` (7.5) | ≈ −120 lines |
| `colorgame.go`: LED colours and six buttons | `leds` pixels and `menu` or `clickmap` | smaller |
| `engine.go` (1,120): `later`, `begin`, the generation counter; the night light's fade, the morning | kit `scene` (7.5); the reactions stay the pet's | ≈ −60 lines, the pattern named |
| `photos.go`: a leaderboard photo with the camera (`snapshot`) | kit `robot.Snapshot(ctx)` (waits for `0x07`); the pet is an app, not a key: section 9's scopes do not apply, the robot's own camera switch does | ≈ −30 lines |
| `voice.go`, `internal/voice`: speech | kit `voice` (7.8) | moved, not removed |
| `watchdog.go`: messages sent and dropped | `robothub` counters | ≈ −30 lines |
| `signin.go`, `pairings.go` (378), the parent PIN | kit `webapp` (the PIN stays the pet's) | ≈ −300 lines |
| `pet.go`, `settings.go`, the lines, `robotpic` (the faces, the needs picture, the podium) | stay: the pet's own | – |

**As a Dev program** ("Pet, small"): needs that drop over time, a mood face (`face`), food
from an NFC card or a menu, a stroke makes it happy; about 120 lines in each language. It
shows menus, files and events working together; it has no parent page, schedule or games.

**Rebuild?** No. The pet's 1,300-line app test is its safety net; move one piece at a time
when the kit has it (menu, assets, held, scene, webapp), each its own release, checked on the
real robot with its debug screenshots. Estimated ≈ −900 lines, and its behaviour unchanged.

### 8.3 What the two teach the kit

Both apps (and Dev) carry the same web plumbing (sessions, pairing, sign-in, the manager's
reports, the page's event stream): ≈ 400 lines each. `webapp` is the biggest saving and is
not on the owner's list; it should come before the app migrations. Both need "send only
when changed", a cancellable schedule, debounced touches and the held guard. Neither needs
a framework: small packages, each used first by Dev.

## 9. Security and privacy

### 9.1 Today

A key belongs to one robot, is made only by a browser paired with it (its QR code, or its
owner signed in through the manager), is stored as its SHA-256, is listed with when it was
last used, and can be deleted on Dev's page or on the manager's (Dev reports it there as
"API key: <name>"). It may run every action, including `command` (all but the deny list).
Limits: 10 actions a second per key (bursts of 30). Dev does not relay the camera, the
microphone or sensor streams.

### 9.2 Scopes on keys

Chosen when the key is made, shown on Dev's page and on the manager's, changed only by
making a new key.

| Scope | Allows | Default | Granted by |
|---|---|---|---|
| `basic` | state, events, show, say, face, screen, move (gestures, look), leds, sound, ask, menus, clickmaps, countdowns, sequences, wait_for, screenshot | on | a paired browser |
| `files` | upload and delete pictures and sounds in the key's folder on the robot | on (decision 6) | a paired browser |
| `sensors` | the touch, motion and light streams | on (decision 6) | a paired browser |
| `motion` | hold, servo power, rotation (still asked on the robot) | off | a paired browser |
| `ir` | send infrared (it switches TVs, lights, air conditioning) | off | a paired browser |
| `car` | drive the car, with a lease (only while the owner has it on) | off | the owner |
| `camera` | `snapshot`: one still per call | off | the owner, with an end time |
| `mic` | `listen`: up to 10 s per call | off | the owner, with an end time |
| `raw` | `command`: any robot command but the deny list | off | the owner |

- "The owner" is the robot's owner signed in (dev.q41.eu with sign-in), or on a home Dev
  without sign-in a paired browser (physical access to the robot's QR code is the proof,
  as for pairing).
- MCP lists only the tools the key's scopes allow; a call outside them answers 403 with the
  scope it needs.
- **A log per key:** the last 200 actions (what, when, the answer), on Dev's page. Principle
  16 (every action logged, readable by the owner).

### 9.3 What the robot itself enforces (unchanged)

The robot's camera and microphone switch (on, off at night, off; set only on the robot or
over USB) refuses `camera`, `mic` and `snapshot` from any app; the red LIVE badge shows
while they stream; rotation and a new start app are asked on its screen; pitch is clamped.
Dev adds no way around them. Every app starts clean (`reset_for_app`), and its input pauses
while the robot's own screens are open.

### 9.4 Media through Dev

- Snapshot and listen only, never a live stream to a program at first; each a short, rate
  limited call (one snapshot per 5 s; one listen at a time). Dev keeps nothing: the picture
  or the sound goes to the caller once.
- The first use by a key shows on the robot "‹key name› wants to take photos. Allow?" and
  waits for the person there (decision 8: Dev's own question now, the firmware's top-layer
  question later, which no app can fake).
- **Not on dev.q41.eu for now.** Through the public server, a photo would pass through a
  service we run in plain form (principles 4, 11, 15). So `camera` and `mic` exist only on
  a Dev at home (a flag, `-allow-media`) until a program can receive media sealed for it
  (below).
- Later: a program enrolls a key of its own with the robot through the QR code's secret, as
  a browser does (i41eu-arch `e2ee.md`), and Dev relays the sealed media unread. The robot's
  encryption is per server, all or nothing, today; media sealed for one enrolled program
  while Dev reads the rest needs a firmware change and its own design in `e2ee.md`.

### 9.5 Encryption and Dev

Dev is an app server, like the pet: it draws cards, matches taps to buttons, keeps the
robot's events; it needs the robot's traffic in plain form, so its entry in the manager's
catalog stays without `e2e`. Raw data stays the encrypted relay. A program that wants
encryption from the robot to itself uses Raw data's protocol and `e2e.js`, not Dev (the docs
say so on `/docs/security`).

### 9.6 Keys in web pages

- Bearer requests get CORS (`Access-Control-Allow-Origin: *`, the headers `Authorization`
  and `Content-Type` only, never credentials), so a page of your own can call Dev with a key.
  Cookies are never sent cross-origin with `*`, and the browser session's
  `X-Requested-With: dev` header is not allowed cross-origin, so the page's own session
  stays same-origin.
- A key in a page is readable by anyone who opens the page: the docs say to use it only in
  pages you run yourself, and to make a separate key for it. Later, a "connect your robot"
  flow (the owner approves a page on Dev's page; the page gets a short-lived token with
  chosen scopes) for pages shared with others.

### 9.7 Kids and agents

- A robot in a family is used by kids; a key is a grown-up's tool. With sign-in (dev.q41.eu),
  only the robot's owner may pair a browser with it and make keys (decision 12), as Raw data
  does for private robots.
- AI agents get `basic` keys (principle 18: agents propose, owners approve); `ask` is how an
  agent asks the person at the robot before acting; the agent instructions keep "be calm,
  keep text short".

## 10. The plan

Each step is a release that is useful on its own, in this order. Tests: `gofmt`, `go vet`,
`go test -race` everywhere, plus what each step names. Releases are tags, one at a time.

| # | Step | Repo | What changes | Tested by |
|---|---|---|---|---|
| 1 | **Pages and the language selector** | q41eu-dev | the page split into Home, Your robots, Start, API, Examples; the header with the chips (curl, Python, Go, MCP), side by side or tabs, remembered; the four examples in all their languages; `/pair` to `/robots` | a test renders every page: every example in each of its languages, every internal link answers 200; the readme's table equals the action table; a look at phone width |
| 2 | **Search** | q41eu-dev | `/docs/search.json` (actions, arguments, events, examples, pages, synonyms), the box in the header | a test: the index has every action, argument, event, example and page heading |
| 3 | **Case studies, first version** | q41eu-dev | Focus and Pet as they are today: what `robothub` and `sound` do for them, mapped to their code; each later step adds its piece when it lands | links checked; reviewed against the apps' code |
| 4 | **The protocol catalog** | i41eu-arch (one line in the spec), q41eu-raw | `wire/catalog.go`; `fake-robot` from it; `screen_snapshot` and `speaker_flush` documented; the readme's Commands table checked | q41eu-raw tests: readme ↔ catalog, fake-robot ↔ catalog |
| 4b | `camera_mic_off` declared in `Register` | i41eu-StackChan | one line in the readings list | a robot's `Register` (devcheck in step 5) |
| 5 | **Coverage** | q41eu-dev | `coverage.go`, `TestCoverage`, `/docs/coverage`, `/v1/coverage`; readings and the API by part from the catalog; Dev and devcheck compare a robot's `Register` with the catalog | `TestCoverage`; devcheck on a real robot |
| 6 | **Key scopes and the key log** | q41eu-dev | scopes (9.2), `command` behind `raw` (keys using it stop working: no backward compatibility), MCP tools by scope, the log on the page, scopes reported to the manager | app tests per scope (403 with the scope missing), MCP tool lists |
| 7 | **JavaScript library and CORS** | q41eu-dev | `cmd/gensdk`, `sdk/js/dev.js` served at `/sdk/dev.js`, the four examples in JS; Run runs the JS files; CORS for bearer requests | `TestGenerated`; CORS tests (no credentials, no `X-Requested-With`); the JS examples under `node` against a fake robot |
| 8 | **Python library, every action in Go** | q41eu-dev | `sdk/python/q41dev.py` (standard library, one file) at `/sdk/q41dev.py`, the examples on it; `client/actions.go` generated | `TestGenerated`; the Python examples run against a fake robot |
| 9 | **The example runner** | q41eu-dev | `examples/examples.json`; `TestExamples` runs every example in every language against Dev with a fake robot | itself; CI with sh, python3, node, go |
| F1–F10 | **The features, one per release** (6.5): F1 screen and stickers with the privacy and pause events; F2 files and image upload; F3 sprites; F4 audio; F5 head extras and the held guard; F6 infrared and sensor switches; F7 `wait_for` and the readings; F8 streams; F9 `screenshot` (after the `0x07` decision); F10 `ping` | q41eu-dev (F9 may need q41eu-raw and the firmware) | per release: the action, its coverage row, snippets in every language (generated), an example, a section of the API by part, a devcheck step | `TestCoverage`, `TestExamples`, devcheck on a real robot with USB screenshots |
| H1–H5 | **Helpers, one per release:** H1 menu, H2 clickmap, H3 countdown, H4 sequence, H5 speak | q41eu-dev (kit packages `menu`, `assets`, `scene`, `voice`) | the kit package, Dev's action, library helpers, an example, a docs section | kit unit tests (rendering, hit tests, schedules with a fake clock), `TestExamples`, devcheck |
| 10 | **Advanced apps** | q41eu-dev | the apps of 5.8 as examples, in every language they fit; the case studies updated with what the kit now has | `TestExamples` |
| 11 | **`webapp` in the kit** | q41eu-dev | sessions, pairing, sign-in, the manager's reports, the page's event stream; Dev on it first | Dev's app tests unchanged and green |
| 12 | **Focus on the kit** | q41eu-focus | `webapp`, the debounce, `leds.Bar`, `robot.Shown` | Focus's tests; a real robot |
| 13 | **Pet on the kit, piece by piece** | q41eu-pet | menu, assets, held, scene, webapp, voice: one release each | the pet's app test (1,300 lines), its debug screenshots on a real robot |
| 14 | **Media for programs (home only)** | q41eu-dev; the firmware if decision 8 picks its question | `snapshot`, `listen`, scopes `camera`/`mic` with an end time, the question on the robot, `-allow-media` | app tests (refused without scope, without the flag, when the robot's switch is off); a real robot: the LIVE badge, the night setting |
| 15 | **The car through Dev** | q41eu-dev | `drive` with a lease (wip/extensions.md) | as there |

**Why this order:** the site gets better at once (1–3); the catalog and the coverage test
(4–5) come before many new actions, so none is added without its row; scopes (6) before the
riskier features; the libraries and the runner (7–9) before the features (F) and helpers (H),
so each one lands in every language in one release; the kit grows inside Dev (H, 11) before
the apps move onto it (12–13); media last (14), behind the most rules.

## 11. Decisions for the owner

1. **Languages:** curl, Python, JavaScript, Go, and MCP as a column. Recommended. Blocks
   (Blockly) for kids later, on the JavaScript library: yes or no?
2. **Where the libraries come from:** served by Dev and kept in the repo (recommended), or
   also published to npm and PyPI (a public release each time).
3. **Docs prose:** HTML fragments with Go templates (recommended, no new dependency), or a
   Markdown library (goldmark), or a small Markdown subset of our own.
4. **The catalog's home:** `wire` in q41eu-raw (recommended; the spec in i41eu-arch names
   it), or a JSON file in i41eu-arch that every repo copies.
5. **`command` behind a `raw` scope,** which breaks keys that use `command` today
   (recommended; early stage, principle 27).
6. **Default scopes for a new key:** `basic`, `files`, `sensors` on (recommended), or `basic`
   only.
7. **Camera and microphone through Dev:** only on a Dev at home until media can be sealed for
   a program (recommended), or on dev.q41.eu with the owner's grant, or never.
8. **The question on the robot for media scopes:** Dev's own `ask` now, a firmware
   top-layer question (`confirm` command, answered only by the person at the robot) later
   (recommended), or the firmware's at once.
9. **Files on the robot:** `dev/<key id>/` by convention now (recommended), moving to the
   per-app folders of `device-storage.md` when the firmware has them (that needs `Accepted
   {app}`).
10. **`0x07` for both snapshots:** a new binary type for the screen picture (recommended,
    e.g. `0x09`, a spec change), or Dev asks for one at a time.
11. **Focus and Pet:** migrate piece by piece (recommended), not rebuilt; add "Focus, small"
    and "Pet, small" as examples: yes or no?
12. **Private robots on dev.q41.eu:** with sign-in, only the owner pairs and makes keys
    (recommended, as Raw data), or anyone with the robot's QR code, as today.
13. **Keys in web pages:** CORS for bearer requests now (recommended), and a "connect your
    robot" flow for shared pages later: yes or no?
14. **The site's language:** English only for now; Czech later?
15. **Runners in CI:** `node` and `python3` for the example tests (recommended; GitHub's
    runners have both), or only Go in CI and the other languages checked by hand.

## 12. Not in this design

- Other devices beside the robot (wip/extensions.md has its own design).
- A hosted app store or third-party apps on dev.q41.eu.
- Live video and audio for programs (after the sealed-media design).
- Changing the device protocol beyond the catalog, the `0x07` split and `camera_mic_off`.
