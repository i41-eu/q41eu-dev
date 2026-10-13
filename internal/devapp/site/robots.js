"use strict";
// Your robots: a card per robot paired with this browser (Try it, Keys, Events), kept live.
(() => {
  const { el, call } = Dev;
  const cards = new Map(); // robot id -> {root, update, addEvent, renderKeys, fillChimes}
  let info = { actions: [], public_url: location.origin, chimes: [] };
  let streaming = false;

  function base() { return (info.public_url || location.origin).replace(/\/$/, ""); }

  function makeCard(r) {
    const c = { id: r.id };
    const dot = el("span", { className: "dot" }), meta = el("span", { className: "meta" });
    const out = el("div", { className: "out", role: "status" });
    const run = (action, body, label) => {
      out.textContent = "→ " + (label || action) + "…";
      return call(c.id, action, body).then((j) => {
        const { ok, ...rest } = j;
        out.textContent = "✓ " + (label || action) + (Object.keys(rest).length ? " " + JSON.stringify(rest) : "");
        return j;
      }).catch((e) => { out.textContent = "✗ " + e.message; });
    };

    // Try it: each control is one action, as a program would send it.
    const text = el("input", { type: "text", placeholder: "Text for the screen", value: "Hello from Dev", ariaLabel: "Text for the screen" });
    const color = el("input", { type: "color", value: "#7d8cff", title: "Colour for cards and LEDs", ariaLabel: "Colour for cards and LEDs" });
    const say = el("input", { type: "text", placeholder: "Speech bubble", value: "Hi!", ariaLabel: "Speech bubble" });
    const bar = el("input", { type: "range", min: 0, max: 100, value: 50, ariaLabel: "LED bar" });
    const notes = el("input", { type: "text", value: "C5:120 E5:120 G5:300", ariaLabel: "Notes to play" });
    const chime = el("select", { ariaLabel: "Chime" }, []);
    const question = el("input", { type: "text", value: "Coffee or tea?", ariaLabel: "Question" });
    const tiles = el("input", { type: "text", value: "Tea, Coffee, Juice, Water", ariaLabel: "Menu items, separated by commas" });
    const picture = el("input", { type: "file", accept: "image/*", ariaLabel: "A picture to show on the robot" });
    // A picture from this phone or computer: made 320x240 here, so only that reaches the robot
    // (nothing else of the photo, such as where it was taken).
    const showPicture = async () => {
      const f = picture.files[0];
      if (!f) { out.textContent = "✗ pick a picture first"; return; }
      try {
        const bmp = await createImageBitmap(f);
        const cv = el("canvas", { width: 320, height: 240 });
        const k = Math.max(320 / bmp.width, 240 / bmp.height), w = bmp.width * k, h = bmp.height * k;
        cv.getContext("2d").drawImage(bmp, (320 - w) / 2, (240 - h) / 2, w, h);
        await run("show", { image: cv.toDataURL("image/jpeg", 0.9).split(",")[1] }, "show picture");
      } catch (e) { out.textContent = "✗ that picture: " + e.message; }
    };
    const answers = el("input", { type: "text", value: "Coffee, Tea", ariaLabel: "Answers, separated by commas" });
    const tryit = el("div", { className: "box" }, [
      el("h2", { textContent: "Try it" }),
      el("div", { className: "row" }, [text, color, el("button", { textContent: "Show", onclick: () => run("show", { text: text.value, color: color.value }, "show") }),
        el("button", { className: "soft", textContent: "Clear", onclick: () => run("show", { clear: true }, "clear") })]),
      el("div", { className: "row" }, [say, el("button", { textContent: "Say", onclick: () => run("say", { text: say.value }) })]),
      el("div", { className: "row" }, [...["happy", "sad", "doubt", "sleepy", "neutral"].map((e) => el("button", { className: "soft", textContent: e, onclick: () => run("face", { emotion: e }, e) })),
        el("button", { className: "soft", textContent: "readings", title: "The robot's own sensors on its screen", onclick: () => run("face", { view: "readings" }, "readings") })]),
      el("div", { className: "row" }, ["nod", "shake", "home"].map((g) => el("button", { className: "soft", textContent: g, onclick: () => run("move", { gesture: g }, g) }))),
      el("div", { className: "row" }, [el("span", { className: "muted", textContent: "LEDs" }), bar,
        el("button", { className: "soft", textContent: "Bar", onclick: () => run("leds", { bar: bar.value / 100, color: color.value }, "LED bar") }),
        el("button", { className: "soft", textContent: "Rainbow", onclick: () => run("leds", { effect: "rainbow", seconds: 5 }, "rainbow") }),
        el("button", { className: "soft", textContent: "Off", onclick: () => run("leds", { off: true }, "LEDs off") })]),
      el("div", { className: "row" }, [chime, el("button", { className: "soft", textContent: "Chime", onclick: () => run("sound", { chime: chime.value }, chime.value) })]),
      el("div", { className: "row" }, [notes, el("button", { className: "soft", textContent: "Play", onclick: () => run("sound", { notes: notes.value }, "notes") })]),
      el("div", { className: "row" }, [question, answers, el("button", { textContent: "Ask", onclick: () => run("ask", { question: question.value,
        answers: answers.value.split(",").map((s) => s.trim()).filter(Boolean), timeout_s: 30 }, "ask (tap an answer on the robot)") })]),
      el("div", { className: "row" }, [tiles, el("button", { textContent: "Menu", onclick: () => run("menu", {
        items: tiles.value.split(",").map((s) => s.trim()).filter(Boolean), timeout_s: 30 }, "menu (tap a tile on the robot)") })]),
      el("div", { className: "row" }, [picture, el("button", { textContent: "Show picture", title: "Made 320x240 in this browser first: nothing else of the photo leaves it", onclick: showPicture })]),
      out,
    ]);
    c.fillChimes = () => { if (!chime.options.length) for (const n of info.chimes || []) chime.append(el("option", { value: n, textContent: n })); };

    // Keys
    const keys = el("div"), fresh = el("div");
    const keyName = el("input", { type: "text", placeholder: "Key name, e.g. laptop or claude", value: "", ariaLabel: "Key name" });
    const keyBox = el("div", { className: "box" }, [el("h2", { textContent: "Keys" }),
      el("p", { className: "muted", textContent: "A key lets a program or an AI agent use this robot through the API. Anyone with it can show things, move the head and hear what people do at the robot: keep it like a password. It is shown once." }),
      el("div", { className: "row" }, [keyName, el("button", { textContent: "New key", onclick: async () => {
        try {
          const res = await fetch("/api/keys", { method: "POST", headers: { "Content-Type": "application/json", "X-Requested-With": "dev" }, body: JSON.stringify({ robot: c.id, name: keyName.value }) });
          const j = await res.json();
          if (!j.ok) throw new Error(j.error);
          keyName.value = "";
          showKey(j.key);
          Dev.load();
        } catch (e) { fresh.textContent = e.message; }
      } })]), fresh, keys]);
    const showKey = (key) => {
      const u = base(), id = c.id;
      fresh.replaceChildren(el("div", { className: "key" }, [
        el("div", { className: "muted", textContent: "Your new key (copy it now, it is not shown again):" }),
        el("pre", { textContent: key }),
        el("div", { className: "muted", textContent: "For your shell (the examples read these):" }),
        el("pre", { textContent: `export SDEV_URL=${u}\nexport SDEV_ROBOT=${id}\nexport SDEV_KEY=${key}` }),
        el("div", { className: "muted", textContent: "Then, in a clone of github.com/i41-eu/q41eu-dev:" }),
        el("pre", { textContent: "sh examples/curl/hello.sh\npython3 examples/python/hello.py\nnode examples/js/hello.mjs\ngo run ./examples/go/hello" }),
        el("div", { className: "muted", textContent: "Give it to an AI agent (MCP), e.g. Claude Code:" }),
        el("pre", { textContent: `claude mcp add --transport http stackchan ${u}/mcp --header "Authorization: Bearer ${key}"` }),
      ]));
    };
    c.renderKeys = (list) => keys.replaceChildren(...list.filter((k) => k.robot === c.id).map((k) => el("div", { className: "row" }, [
      el("span", { textContent: k.name }), el("code", { textContent: k.id }),
      el("span", { className: "muted", textContent: "made " + new Date(k.created).toLocaleDateString() + (k.last_used ? " · used " + new Date(k.last_used).toLocaleString() : " · not used yet") }),
      el("button", { className: "soft", textContent: "Delete", ariaLabel: "Delete the key " + k.name, onclick: async () => {
        if (!confirm(`Delete the key ${k.name}? Programs using it stop working.`)) return;
        await fetch("/api/keys/" + k.id, { method: "DELETE", headers: { "X-Requested-With": "dev" } });
        Dev.load();
      } }),
    ])));

    // Events: what people do at the robot, and what programs ask of it
    const events = el("ul", { className: "events" });
    c.addEvent = (text, act) => {
      events.prepend(el("li", { className: act ? "act" : "", textContent: new Date().toLocaleTimeString() + "  " + text }));
      while (events.children.length > 60) events.lastChild.remove();
    };
    const evBox = el("div", { className: "box" }, [el("h2", { textContent: "Events" }),
      el("p", { className: "muted", textContent: "Tap the robot's screen, touch its head, hold a tag near it. Grey: what programs asked." }), events]);

    c.root = el("section", { className: "card", ariaLabel: "Robot " + r.id }, [el("div", { className: "head" }, [dot, el("span", { className: "name", textContent: r.id }), meta]),
      el("div", { className: "grid" }, [tryit, keyBox, evBox])]);
    c.update = (r) => {
      dot.className = "dot" + (r.online ? " on" : "");
      meta.textContent = r.online ? `online · ${r.model || ""} · firmware ${r.firmware || "?"}` : "offline";
      tryit.querySelectorAll("button").forEach((b) => { b.disabled = !r.online; });
    };
    return c;
  }

  function upsert(r) {
    let c = cards.get(r.id);
    if (!c) {
      c = makeCard(r);
      cards.set(r.id, c);
      document.getElementById("robots").append(c.root);
    }
    c.fillChimes();
    c.update(r);
  }

  function render(j) {
    info = j;
    info.chimes = (j.actions.find((a) => a.name === "sound")?.params.find((p) => p.name === "chime")?.enum) || [];
    for (const r of j.robots) upsert(r);
    for (const c of cards.values()) c.renderKeys(j.keys || []);
    document.getElementById("empty").hidden = j.robots.length > 0;
    const me = j.me || {};
    if (me.manager_url) document.getElementById("mgrLink2").href = me.manager_url + "/";
    document.getElementById("mgrHint").hidden = !me.manager_url;
    document.getElementById("signinHint").hidden = !me.sign_in || !!me.signed_in;
    if (cards.size && !streaming) {
      streaming = true;
      connect();
    }
  }

  function connect() {
    const es = new EventSource("/api/events");
    es.addEventListener("robot", (ev) => { const r = JSON.parse(ev.data); if (cards.has(r.id)) upsert(r); });
    es.addEventListener("event", (ev) => {
      const e = JSON.parse(ev.data), c = cards.get(e.robot);
      if (c) c.addEvent(e.event.name + (e.event.data ? " " + JSON.stringify(e.event.data) : "") + `  #${e.event.seq}`);
    });
    es.addEventListener("action", (ev) => {
      const e = JSON.parse(ev.data), c = cards.get(e.robot);
      if (c) c.addEvent(`→ ${e.action} ${JSON.stringify(e.args)} (${e.by})`, true);
    });
    es.onerror = () => { es.close(); setTimeout(connect, 3000); };
  }

  Dev.onState(render);
  setInterval(() => Dev.load().catch(() => {}), 15000);
})();
