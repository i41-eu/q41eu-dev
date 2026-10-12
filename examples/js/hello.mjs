#!/usr/bin/env node
// hello: a card on the robot's screen, its LEDs, a nod and a chime.
//
//   export SDEV_URL=… SDEV_ROBOT=… SDEV_KEY=…   (Dev's page shows them with a new key)
//   node hello.mjs
// Node 18 or newer, nothing to install: only fetch.
const { SDEV_URL, SDEV_ROBOT, SDEV_KEY } = process.env;
if (!SDEV_URL || !SDEV_ROBOT || !SDEV_KEY) {
  console.error("set SDEV_URL, SDEV_ROBOT and SDEV_KEY (Dev's page shows them with a new key)");
  process.exit(2);
}

// act: one action on the robot: POST /v1/robots/<robot>/<action> with the arguments as JSON.
async function act(action, args = {}) {
  const res = await fetch(`${SDEV_URL.replace(/\/$/, "")}/v1/robots/${SDEV_ROBOT}/${action}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${SDEV_KEY}`, "Content-Type": "application/json" },
    body: JSON.stringify(args),
  });
  const out = await res.json().catch(() => ({ ok: false, error: res.statusText }));
  if (!res.ok || out.ok === false) throw new Error(`${action}: ${out.error || res.status}`);
  return out;
}

await act("show", { title: "Hello", text: "from JavaScript", note: "q41eu-dev examples", color: "green" });
await act("leds", { color: "green" });
await act("move", { gesture: "nod" });
await act("sound", { chime: "tada" });
console.log("done: look at the robot");
