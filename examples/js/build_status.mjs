#!/usr/bin/env node
// build_status: runs a command and shows on the robot how it went: blue while it runs, then a
// green card and a chime when it passed, a red card and a shake when it failed. Its exit code is
// the command's.
//
//   node build_status.mjs make test
// Node 18 or newer, nothing to install: only fetch.
import { spawnSync } from "node:child_process";

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

const cmd = process.argv.length > 2 ? process.argv.slice(2) : ["make", "test"];
const name = cmd.join(" ");
await act("show", { title: "Running", text: name, color: "blue" });
await act("leds", { effect: "breathe", color: "blue" });
const start = Date.now();
const run = spawnSync(cmd[0], cmd.slice(1), { stdio: "inherit" });
const code = run.status ?? 1;
const took = `${name} · ${Math.round((Date.now() - start) / 1000)} s`;
if (code === 0) {
  await act("show", { title: "Passed", text: "All good", note: took, color: "green", progress: 1 });
  await act("leds", { color: "green" });
  await act("sound", { chime: "tada" });
  await act("move", { gesture: "nod" });
} else {
  await act("show", { title: "Failed", text: "Have a look", note: took, color: "red" });
  await act("leds", { color: "red" });
  await act("sound", { chime: "error" });
  await act("move", { gesture: "shake" });
}
process.exit(code);
