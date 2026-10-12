# Extensions: devices beside the robot, through Dev

Design notes, not features yet. The question from the owner (2026-10-12): how does Dev (and its
kit) take in robot extensions and connected devices (the TPBot car, Sbot's devices, a lamp) so
that programs and agents use them the way they use the robot?

## What exists to build on

- The robot already carries one extension: the car over BLE (`car_enable`, then `car_*`
  commands and telemetry, re-registered capabilities; wire protocol §7 "Extensions use a prefix
  and an `*_enable` command").
- i41eu-arch [device API](https://github.com/i41-eu/i41eu-arch/blob/main/docs/device-api.md):
  devices list their **parts** (named, with attributes and units) in capability descriptors,
  one command per kind of part (`led`, `servo`, …), safety commands stay on the device.
- Sbot (q41eu-sbot) is the home node seed: devices beside the robot, an event hub.

## Proposal

1. **A device is a target, the robot one of them.** `/v1/robots/<robot>/…` becomes
   `/v1/devices/<device>/…` (the robot keeps its id; the car is `car` under it:
   `<robot>.car`, or its own id when it has its own connection). A key may cover a robot and
   the devices attached to it.
2. **Actions come from parts.** Today's actions are written for the Stackchan. With the
   device API's descriptors, Dev offers generic actions for kinds of parts (`led`, `servo`,
   `display`, `speaker`, `drive` with its watchdog) on any device that lists them, next to the
   higher-level ones a device family adds (the robot's `ask`, `show`). The action table
   (`internal/devapp/actions.go`) becomes per device kind; REST, MCP, OpenAPI and the page keep
   coming from it.
3. **Events from every device** in one stream with the device's name (`car.bumper`,
   `robot.pat`); `gesture`-like packages per device family make the meaning.
4. **Safety stays on the device:** a moving thing stops when commands stop (the car's
   watchdog); Dev's `drive` sends with a lease (`seconds`), never "forever"; a person's Yes on
   the robot for anything that moves the robot itself (rotation) stays.
5. **Sbot** keeps the home's own loops; Dev is the door for programs and agents. Sbot may use
   Dev's kit for its robot connection (as Focus does), and later register its devices with Dev.

## Order

1. The device API's descriptors in the firmware (parts list) and in `wire`.
2. Dev reads them; generic part actions next to the robot's.
3. The car through Dev (`drive` with a lease, its sonar and line sensors as readings).
4. Sbot onto the kit; its devices as Dev devices.
