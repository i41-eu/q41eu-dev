#!/usr/bin/env node
// ask: a question on the robot's screen; the person at the robot taps an answer.
//
//   node ask.mjs "Coffee or tea?" Coffee Tea
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

const [question, ...answers] = process.argv.length > 4 ? process.argv.slice(2) : ["Coffee or tea?", "Coffee", "Tea"];
const out = await act("ask", { question, answers });
console.log(out.answer || "no answer within 30 s");
