#!/usr/bin/env node
// picture: a picture of yours on the robot's screen (or four numbered quarters), then the quarter
// the person taps. Dev fits the picture to the screen; up to about 1.5 MB.
//
//   node picture.mjs [photo.jpg]
// Node 18 or newer, nothing to install: only fetch.
import { readFile } from "node:fs/promises";

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

const image = (await readFile(process.argv[2] || new URL("../img/quarters.png", import.meta.url))).toString("base64");
const regions = [0, 1, 2, 3].map((i) => ({ id: String(i + 1), rect: [160 * (i % 2), 120 * Math.floor(i / 2), 160, 120] }));
const out = await act("clickmap", { image, regions });
console.log(out.region ? `quarter ${out.region} (at ${out.x}, ${out.y})` : "no tap within 30 s");
