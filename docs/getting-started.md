# Getting started with Dev

From a Stackchan robot on your desk to your own program (or AI agent) driving it: about ten
minutes. You need the robot, a computer with Chrome (for the USB setup), and a phone or the same
computer for Dev's site. The site has this guide too (**Getting started**), with every command in
the languages you pick.

## 1. The robot gets Dev as one of its apps

Dev is an app like Pet or Focus: the robot connects to it, one app at a time.

- **Online (m.i41.eu):** sign in at [m.i41.eu](https://m.i41.eu), plug the robot into the
  computer over USB and set it up (one button: the official firmware and the apps you pick).
  Pick **Dev** among its apps. A robot already set up: add Dev on its card on m.i41.eu (the
  robot gets the new app list at once over its connection to the manager).
- **At home:** run Dev yourself (the [readme](../readme.md#run): one Go binary or the container)
  and add it with your home manager, or over USB without one:
  `i41eu-usb provision -url ws://<your computer>:8785 -name Dev -token-file ~/.config/stackchan-server/robot-token`.

On the robot: swipe up from the bottom edge, tap **QR**, then **Next** until it shows Dev, and
**Connect**. The robot shows Dev's QR code.

## 2. Your browser gets the robot

- **Signed in at the manager** (m.i41.eu): open Dev's site in the same browser. It signs you in
  through the manager, and **Your robots** shows every robot you set up with Dev, without a QR
  code. (**Sign in** at the top if it does not.)
- **Or by the robot's QR code:** scan it with your phone, or open the link under it on your
  computer. **Your robots** on Dev's site now shows your robot: **Try it** runs every action by
  hand, **Events** shows what people do at the robot.

## 3. A key for your program

On **Your robots**, **New key** (name it after where it will live: "laptop", "ci", "claude"). The key
is shown once, with the three lines for your shell:

```bash
export SDEV_URL=https://dev.q41.eu        # where Dev runs
export SDEV_ROBOT=stackchan-0a1b2c3d4e50      # your robot
export SDEV_KEY=sdev_…                        # the key: keep it like a password
```

Put them in your shell's profile, or in a file you `source` (never in a repository). The key
works for this robot only. You see it, with when it was last used, on Dev's site and on the
robot's card on m.i41.eu, and delete it in either place.

## 4. Run the examples

```bash
git clone https://github.com/i41-eu/q41eu-dev && cd q41eu-dev
sh examples/curl/hello.sh                         # curl
python3 examples/python/hello.py                  # Python 3, the standard library only
node examples/js/hello.mjs                        # JavaScript: Node 18 or newer, only fetch
go run ./examples/go/hello                        # Go, with the client package
```

| Example | What it shows | curl | Python | JavaScript | Go | MCP |
|---|---|---|---|---|---|---|
| hello | a card, green LEDs, a nod, a chime | [hello.sh](../examples/curl/hello.sh) | [hello.py](../examples/python/hello.py) | [hello.mjs](../examples/js/hello.mjs) | [hello](../examples/go/hello/main.go) | [hello](../examples/mcp/hello.txt) |
| ask | a question with buttons; prints the answer tapped | [ask.sh](../examples/curl/ask.sh) | [ask.py](../examples/python/ask.py) | [ask.mjs](../examples/js/ask.mjs) | [ask](../examples/go/ask/main.go) | [ask](../examples/mcp/ask.txt) |
| build status | runs a command (your tests): blue while it runs, then green and a chime, or red and a shake | [build_status.sh](../examples/curl/build_status.sh) | [build_status.py](../examples/python/build_status.py) | [build_status.mjs](../examples/js/build_status.mjs) | [buildstatus](../examples/go/buildstatus/main.go) | [build status](../examples/mcp/build_status.txt) |
| menu | a snack menu of four tiles; prints the one tapped, and the robot says it | [menu.sh](../examples/curl/menu.sh) | [menu.py](../examples/python/menu.py) | [menu.mjs](../examples/js/menu.mjs) | [menu](../examples/go/menu/main.go) | [menu](../examples/mcp/menu.txt) |
| picture | a picture of yours on the robot (or four numbered quarters); prints the quarter tapped | [picture.sh](../examples/curl/picture.sh) | [picture.py](../examples/python/picture.py) | [picture.mjs](../examples/js/picture.mjs) | [picture](../examples/go/picture/main.go) | [picture](../examples/mcp/picture.txt) |
| events | prints what people do at the robot; a pat makes it chirp and smile, a tag shows its text | [events.sh](../examples/curl/events.sh) | [events.py](../examples/python/events.py) | [events.mjs](../examples/js/events.mjs) | [events](../examples/go/events/main.go) | [events](../examples/mcp/events.txt) |

The MCP column is for an AI agent (section 5): what to ask it, and the tool calls it makes. Dev's
site (**Examples**) has the same examples in every language, each with a **Run** button, which
does the same from the page.

Without a clone: `go run github.com/i41-eu/q41eu-dev/examples/go/hello@latest`, or copy a Python
or JavaScript file: each one stands alone.

## 5. An AI agent

The same actions are MCP tools (`robot_show`, `robot_ask`, `robot_events`, …) at `$SDEV_URL/mcp`.
For Claude Code:

```bash
claude mcp add --transport http stackchan "$SDEV_URL/mcp" --header "Authorization: Bearer $SDEV_KEY"
```

Then ask it, for example, "show on the robot what you are working on, and ask me there before
you push". Any MCP client that speaks Streamable HTTP works the same way.

## 6. Your own program

- **The API:** every action is `POST $SDEV_URL/v1/robots/$SDEV_ROBOT/<action>` with a JSON
  object; the actions and their arguments are in the [readme](../readme.md#the-api), on Dev's
  site (**API**, with a call of each in every language), and as an OpenAPI 3.1 document at `$SDEV_URL/v1/openapi.json` (load it into any OpenAPI
  tool, or a generator for your language).
- **Go:** [`client`](../client/client.go) (`client.FromEnv()`, then `Show`, `Ask`, `Events`, …).
- **Limits:** 10 actions a second per key (bursts of 30). The robot shows one card at a time:
  a new one replaces the last. It blanks its screen after a minute without use; a card wakes it.
- **What a person did** comes as events (`tap`, `pat`, `pet`, `stroke`, `shaken`, `picked_up`,
  `near`, `away`, `tag`, `remote`, `button`, …): ask for the ones after the last `seq` you saw,
  waiting up to a minute (`?after=<seq>&seconds=30`).

## Lower level

Dev is the robot's higher-level API. Underneath, the robot speaks the device wire protocol
(every sensor raw, every command, video and audio): the [Raw data app](https://github.com/i41-eu/q41eu-raw)
shows all of it, and an app of your own can speak it directly, with Dev's
[Go kit](../readme.md#the-go-kit) (`robothub` for the connection) or on its own.
