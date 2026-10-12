#!/usr/bin/env node
// events: prints what people do at the robot and answers some of it: a pat on the head makes it
// chirp and smile, a tag held near shows what the tag says. Ctrl+C ends it.
//
//   node events.mjs
// Node 18 or newer, nothing to install: only fetch.
const { SDEV_URL, SDEV_ROBOT, SDEV_KEY } = process.env;
if (!SDEV_URL || !SDEV_ROBOT || !SDEV_KEY) {
  console.error("set SDEV_URL, SDEV_ROBOT and SDEV_KEY (Dev's page shows them with a new key)");
  process.exit(2);
}
const base = `${SDEV_URL.replace(/\/$/, "")}/v1/robots/${SDEV_ROBOT}`;
const headers = { Authorization: `Bearer ${SDEV_KEY}`, "Content-Type": "application/json" };

async function act(action, args = {}) {
  const res = await fetch(`${base}/${action}`, { method: "POST", headers, body: JSON.stringify(args) });
  const out = await res.json().catch(() => ({ ok: false, error: res.statusText }));
  if (!res.ok || out.ok === false) throw new Error(`${action}: ${out.error || res.status}`);
  return out;
}

// events: the events after a sequence number; waits up to seconds for the next one.
async function events(after, seconds) {
  const res = await fetch(`${base}/events?after=${after}&seconds=${seconds}`, { headers });
  const out = await res.json();
  if (!res.ok) throw new Error(`events: ${out.error || res.status}`);
  return out;
}

let last = (await events(0, 0)).last_seq; // start from now
console.log("touch the robot: tap its screen, pat its head, hold a tag near it…");
for (;;) {
  let got;
  try {
    got = await events(last, 30);
  } catch (err) {
    console.log(err.message);
    await new Promise((ok) => setTimeout(ok, 3000));
    continue;
  }
  last = got.last_seq;
  for (const e of got.events) {
    const data = e.data || {};
    console.log(e.name, data);
    if (["pat", "pet", "stroke"].includes(e.name)) {
      await act("face", { emotion: "happy" });
      await act("sound", { chime: "chirp" });
    } else if (e.name === "tag") {
      await act("show", { title: "A tag", text: data.text || "(no text)", note: data.uid || "" });
    } else if (e.name === "tap") {
      await act("show", { clear: true });
    }
  }
}
