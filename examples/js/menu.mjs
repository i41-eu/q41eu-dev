#!/usr/bin/env node
// menu: a snack menu of four tiles on the robot's screen; prints the one tapped and the robot
// says it.
//
//   node menu.mjs
// Node 18 or newer, nothing to install: only fetch.
const { SDEV_URL, SDEV_ROBOT, SDEV_KEY } = process.env;
if (!SDEV_URL || !SDEV_ROBOT || !SDEV_KEY) {
  console.error("set SDEV_URL, SDEV_ROBOT and SDEV_KEY (Dev's page shows them with a new key)");
  process.exit(2);
}

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

const out = await act("menu", { title: "Snack?", items: ["Apple", "Cake", "Carrot", "Milk"] });
if (!out.item) {
  console.log("no choice within 30 s");
  process.exit(1);
}
console.log(out.item);
await act("show", { clear: true }); // the speech bubble is on the face
await act("say", { text: `${out.item}, yum!` });
