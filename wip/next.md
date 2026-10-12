# Dev: what comes next

Plans, not features yet (docs only describe what exists).

1. **Apps on the kit.** Focus moves its robot plumbing to `robothub` and its chimes to `sound`
   (branch `dev-kit` in q41eu-focus, waiting for a tagged q41eu-dev). Then the pet (its
   `robotconn.go`, `ledColor`, `sound`, and its touch rules into `gesture`), sbot, and raw (raw
   keeps its relay and media parts; its hub part is the same).
2. **Sign-in through the manager** (`sso`), so the owner's browsers get their robots without the
   QR code (as Focus and the pet).
3. **A log of what each key did**, on the page; per-account tiers for the limits, as raw's.
4. **More actions**, each asked for by a use case: `snapshot` (a camera still for an agent, only
   while the robot's own camera setting allows it, and shown on the robot as LIVE), `listen`
   (a few seconds of the microphone), menus from sprites (pictures stored on the robot), `wait_for`
   a reading crossing a value (light, proximity) for scripts.
5. **Device API names** (i41eu-arch docs/device-api.md): when the robot lists its parts, `state`
   gives them, and `leds` / `move` take part names.
