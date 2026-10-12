"use strict";
// Every page of the site: the languages picked, Copy, Run for the examples, and the account in
// the header. The page works without it (every language shown); this only hides and helps.
const Dev = (() => {
  const el = (tag, props = {}, kids = []) => {
    const e = Object.assign(document.createElement(tag), props);
    for (const k of kids) if (k != null && k !== false) e.append(k);
    return e;
  };
  const root = document.documentElement;
  // Remembered per browser; a browser without storage (or with it blocked) gets the defaults.
  const store = (k, v) => { try { localStorage.setItem(k, v); } catch (e) { /* this page only */ } };

  // An action through the page's own session (its cookie and this header, which other sites
  // cannot send), as a program does with its key.
  async function call(id, action, body) {
    const res = await fetch(`/v1/robots/${encodeURIComponent(id)}/${action}`, {
      method: "POST", headers: { "Content-Type": "application/json", "X-Requested-With": "dev" }, body: JSON.stringify(body || {}),
    });
    const j = await res.json().catch(() => ({ ok: false, error: res.statusText }));
    if (!res.ok || j.ok === false) throw new Error(j.error || res.statusText);
    return j;
  }

  /* ------------------------------ the languages ------------------------------ */
  // The head's script set the classes before the first paint (window.devLangs); here the chips
  // change them. All are on by default; turning off the last one turns all on again.
  const L = window.devLangs || { all: [], langs: [], fromURL: false, tabs: false, tab: "" };
  const chips = [...document.querySelectorAll("#langs input[name=lang]")];
  const tabsBtn = document.getElementById("tabsBtn"), keepBtn = document.getElementById("keepBtn");
  function applyLangs() {
    if (!L.langs.includes(L.tab)) L.tab = L.langs[0];
    for (const l of L.all) {
      root.classList.toggle("lang-" + l, L.langs.includes(l));
      root.classList.toggle("tab-" + l, l === L.tab);
    }
    root.classList.toggle("tabs", L.tabs);
    for (const c of chips) c.checked = L.langs.includes(c.value);
    for (const t of document.querySelectorAll(".tab")) t.setAttribute("aria-pressed", String(t.dataset.lang === L.tab));
    if (tabsBtn) tabsBtn.setAttribute("aria-pressed", String(L.tabs));
    if (keepBtn) keepBtn.hidden = !L.fromURL;
  }
  // keep: the languages shown now are this browser's choice (also those of a ?lang= link).
  function keep() {
    L.fromURL = false;
    store("dev.langs", JSON.stringify(L.langs));
    const u = new URL(location.href);
    if (u.searchParams.has("lang")) {
      u.searchParams.delete("lang");
      history.replaceState(null, "", u);
    }
    applyLangs();
  }
  for (const c of chips) {
    c.addEventListener("change", () => {
      L.langs = chips.filter((x) => x.checked).map((x) => x.value);
      if (!L.langs.length) L.langs = L.all.slice();
      keep();
    });
  }
  if (tabsBtn) tabsBtn.addEventListener("click", () => { L.tabs = !L.tabs; store("dev.layout", L.tabs ? "tabs" : "side"); applyLangs(); });
  if (keepBtn) keepBtn.addEventListener("click", keep);
  document.addEventListener("click", (ev) => {
    const t = ev.target.closest(".tab");
    if (!t) return;
    // The tab picked follows across the page; the one clicked stays where it was on the screen.
    const before = t.getBoundingClientRect().top;
    L.tab = t.dataset.lang;
    store("dev.tab", L.tab);
    applyLangs();
    window.scrollBy(0, t.getBoundingClientRect().top - before);
  });
  applyLangs();

  /* ---------------------------------- Copy ---------------------------------- */
  document.addEventListener("click", async (ev) => {
    const b = ev.target.closest(".copy");
    if (!b) return;
    const text = b.closest(".code").querySelector("pre").textContent;
    let ok = false;
    try {
      await navigator.clipboard.writeText(text);
      ok = true;
    } catch (e) { // no clipboard API (a page over plain HTTP on the LAN): the old way
      const t = el("textarea", { value: text, readOnly: true });
      t.style.position = "fixed";
      t.style.opacity = "0";
      document.body.append(t);
      t.select();
      try { ok = document.execCommand("copy"); } catch (e2) { ok = false; }
      t.remove();
      b.focus();
    }
    b.textContent = ok ? "Copied" : "Select it to copy";
    setTimeout(() => { b.textContent = "Copy"; }, 1500);
  });

  /* ------------------------------- the account ------------------------------- */
  let state = null;
  const waiting = [];
  function renderAccount(me) {
    const acct = document.getElementById("acct");
    if (!acct) return;
    const kids = [];
    if (me.manager_url) kids.push(el("a", { href: me.manager_url + "/", textContent: "Manager", target: "_blank", rel: "noopener" }));
    if (me.sign_in && me.signed_in) {
      kids.push(el("span", { textContent: me.name || "signed in" }), el("a", { href: "#", textContent: "Sign out", onclick: async (e) => {
        e.preventDefault();
        await fetch("/auth/logout", { method: "POST", headers: { "X-Requested-With": "dev" } });
        location.href = location.pathname + "?signin=no";
      } }));
    } else if (me.sign_in) {
      kids.push(el("a", { href: "/auth/login?next=" + encodeURIComponent(location.pathname), textContent: "Sign in" }));
    }
    acct.replaceChildren(...kids);
  }
  // Home: the robots in brief.
  function renderBrief(robots) {
    const box = document.getElementById("brief");
    if (!box) return;
    box.hidden = !robots.length;
    document.getElementById("briefList").replaceChildren(...robots.map((r) => el("a", { href: "/robots" }, [
      el("span", { className: "dot" + (r.online ? " on" : "") }), el("span", { className: "name", textContent: r.id }),
      el("span", { className: "muted", textContent: r.online ? "online" : "offline" })])));
  }
  async function load() {
    const j = await fetch("/api/state").then((r) => r.json());
    state = j;
    renderAccount(j.me || {});
    renderBrief(j.robots || []);
    const ex = document.getElementById("exRobot");
    if (ex) ex.textContent = j.robots.length ? "Run uses " + j.robots[0].id + "." : "Pair a robot on Your robots to run them here.";
    for (const f of waiting) f(j);
    return j;
  }
  function onState(f) {
    waiting.push(f);
    if (state) f(state);
  }

  /* ------------------------------- Run (examples) ------------------------------- */
  // Run does here, on the first robot paired with this browser, what the example's program does
  // on a computer (call = the same action through this page).
  const RUNS = {
    hello: async (call) => {
      await call("show", { title: "Hello", text: "from the page", note: "q41eu-dev examples", color: "green" });
      await call("leds", { color: "green" });
      await call("move", { gesture: "nod" });
      await call("sound", { chime: "tada" });
      return "done: look at the robot";
    },
    ask: async (call, log) => {
      log("waiting up to 30 s for a tap on the robot…");
      const r = await call("ask", { question: "Coffee or tea?", answers: ["Coffee", "Tea"] });
      return r.answer || "no answer within 30 s";
    },
    build: async (call, log) => {
      await call("show", { title: "Running", text: "make test", color: "blue" });
      await call("leds", { effect: "breathe", color: "blue" });
      log("here a 3 s pause stands in for your command…");
      await new Promise((ok) => setTimeout(ok, 3000));
      await call("show", { title: "Passed", text: "All good", note: "make test · 3 s", color: "green", progress: 1 });
      await call("leds", { color: "green" });
      await call("sound", { chime: "tada" });
      await call("move", { gesture: "nod" });
      return "passed";
    },
    events: async (call, log, events) => {
      let last = (await events(0, 0)).last_seq;
      log("for 30 s, touch the robot: tap its screen, pat its head, hold a tag near it…");
      // Listens the whole time, as the programs do: a hand bringing a tag brushes the head
      // (stroke) before the tag is read, and the tag must still count.
      const end = Date.now() + 30000, seen = [];
      while (Date.now() < end) {
        const got = await events(last, Math.max(1, Math.round((end - Date.now()) / 1000)));
        if (got.last_seq === undefined) throw new Error(got.error || "events failed");
        last = got.last_seq;
        for (const e of got.events) {
          seen.push(e.name);
          log(e.name + " " + JSON.stringify(e.data || {}));
          if (["pat", "pet", "stroke"].includes(e.name)) { await call("face", { emotion: "happy" }); await call("sound", { chime: "chirp" }); }
          if (e.name === "tag") await call("show", { title: "A tag", text: (e.data || {}).text || "(no text)", note: (e.data || {}).uid || "" });
          if (e.name === "tap") await call("show", { clear: true });
        }
      }
      return seen.length ? seen.length + " event(s): " + [...new Set(seen)].join(", ") : "nothing within 30 s";
    },
  };
  document.addEventListener("click", async (ev) => {
    const b = ev.target.closest(".run");
    if (!b || b.disabled) return;
    const out = b.parentElement.nextElementSibling;
    const lines = [];
    const log = (t) => { lines.push(t); out.textContent = lines.join("\n"); out.hidden = false; };
    const run = RUNS[b.dataset.run];
    const id = state && state.robots.length ? state.robots[0].id : "";
    if (!run) { log("✗ not runnable here"); return; }
    if (!id) { log("✗ pair a robot first: Your robots"); return; }
    b.disabled = true;
    const events = (after, seconds) => fetch(`/v1/robots/${encodeURIComponent(id)}/events?after=${after}&seconds=${seconds}`,
      { headers: { "X-Requested-With": "dev" } }).then((r) => r.json());
    try {
      log("✓ " + await run((action, body) => call(id, action, body), log, events));
    } catch (e) {
      log("✗ " + e.message);
    }
    b.disabled = false;
  });

  /* --------------------------------- search --------------------------------- */
  // The box in the header: the index (/docs/search.json, every action, argument, event,
  // example and section) is fetched once and matched here as you type. "/" comes back to it.
  const q = document.getElementById("q"), list = document.getElementById("q-results"), status = document.getElementById("q-status");
  const STOP = new Set(["a", "an", "the", "of", "to", "in", "on", "at", "for", "and", "or", "with", "is", "it", "its", "my", "me", "i", "how", "do", "does", "what", "can"]);
  const words = (s) => s.toLowerCase().split(/[^a-z0-9_]+/).filter(Boolean);
  let index = null, shown = [], active = -1, pending = Promise.resolve();
  function loadIndex() {
    if (!index) {
      index = fetch("/docs/search.json").then((r) => r.json()).then((j) => {
        for (const e of j.entries) {
          e.n = e.name.toLowerCase();
          e.nw = words(e.name).concat(e.n.split(/[_\s]+/));
          e.tw = words([e.text || "", e.in || "", ...(e.parts || [])].join(" "));
        }
        return j;
      });
      index.catch(() => { index = null; });
    }
    return index;
  }
  // score: every word of the query (or one of its synonyms) must match; names weigh more than
  // texts, a whole name more than its start.
  function score(e, query) {
    let total = 0;
    for (const alts of query) {
      let best = 0;
      alts.forEach((w, i) => {
        let s = 0;
        if (e.n === w) s = 100;
        else if (e.n.startsWith(w)) s = 60;
        else if (e.nw.some((x) => x.startsWith(w))) s = 40;
        else if (e.tw.includes(w)) s = 12;
        else if (w.length > 2 && e.tw.some((x) => x.startsWith(w))) s = 8;
        best = Math.max(best, i ? s * 0.7 : s); // a synonym counts a little less
      });
      if (!best) return 0;
      total += best;
    }
    return total;
  }
  function close() {
    list.hidden = true;
    q.setAttribute("aria-expanded", "false");
    q.removeAttribute("aria-activedescendant");
    active = -1;
  }
  async function search() {
    const text = q.value.trim();
    if (!text) { close(); status.textContent = ""; return; }
    const j = await loadIndex();
    if (q.value.trim() !== text) return; // typed on meanwhile
    let ws = words(text);
    if (ws.some((w) => !STOP.has(w))) ws = ws.filter((w) => !STOP.has(w));
    const query = ws.map((w) => [w, ...(Object.hasOwn(j.synonyms, w) ? j.synonyms[w] : [])]);
    const hits = j.entries.map((e) => ({ e, s: score(e, query) })).filter((h) => h.s > 0)
      .map((h) => ({ e: h.e, s: h.s - j.kinds.indexOf(h.e.kind) * 0.5 })) // a tie: actions first, pages last
      .sort((a, b) => b.s - a.s).slice(0, 12);
    const groups = []; // by kind, in the order of each kind's best result
    for (const h of hits) {
      let g = groups.find((x) => x.kind === h.e.kind);
      if (!g) groups.push(g = { kind: h.e.kind, items: [] });
      g.items.push(h.e);
    }
    shown = groups.flatMap((g) => g.items);
    active = -1;
    let n = 0;
    list.replaceChildren(...groups.map((g, gi) => {
      const group = el("div", {}, [el("div", { className: "group", id: "q-g" + gi, textContent: g.kind })]);
      group.setAttribute("role", "group");
      group.setAttribute("aria-labelledby", "q-g" + gi);
      for (const e of g.items) {
        const o = el("div", { id: "q-o" + n++ }, [el("a", { href: e.url, tabIndex: -1 }, [el("b", { textContent: e.name }),
          e.in ? el("span", { className: "in", textContent: " · " + e.in }) : null, e.text ? el("span", { className: "text", textContent: e.text }) : null])]);
        o.setAttribute("role", "option");
        o.setAttribute("aria-selected", "false");
        group.append(o);
      }
      return group;
    }));
    if (!shown.length) list.replaceChildren(el("div", { className: "none", textContent: `Nothing found for “${text}”.` }));
    list.hidden = false;
    q.setAttribute("aria-expanded", "true");
    q.removeAttribute("aria-activedescendant");
    status.textContent = shown.length ? `${shown.length} result${shown.length > 1 ? "s" : ""}` : "nothing found";
  }
  function move(d) {
    if (!shown.length) return;
    active = active < 0 ? (d > 0 ? 0 : shown.length - 1) : (active + d + shown.length) % shown.length;
    for (const o of list.querySelectorAll("[role=option]")) {
      const on = o.id === "q-o" + active;
      o.classList.toggle("active", on);
      o.setAttribute("aria-selected", String(on));
      if (on) o.scrollIntoView({ block: "nearest" });
    }
    q.setAttribute("aria-activedescendant", "q-o" + active);
  }
  function go(i) {
    const e = shown[i];
    if (!e) return;
    close();
    const u = new URL(e.url, location.href);
    if (u.pathname !== location.pathname) { location.href = u.href; return; }
    location.hash = u.hash; // on this page: there, and the keyboard's focus with it
    const target = u.hash && document.getElementById(decodeURIComponent(u.hash.slice(1)));
    if (target) {
      if (!target.hasAttribute("tabindex")) target.setAttribute("tabindex", "-1");
      target.focus();
    }
  }
  if (q) {
    q.addEventListener("input", () => { pending = search().catch(() => {}); });
    q.addEventListener("focus", () => { loadIndex().catch(() => {}); });
    q.addEventListener("keydown", (ev) => {
      if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
        ev.preventDefault();
        if (list.hidden) pending = search().catch(() => {});
        else move(ev.key === "ArrowDown" ? 1 : -1);
      } else if (ev.key === "Enter") {
        ev.preventDefault();
        pending.then(() => go(active >= 0 ? active : 0));
      } else if (ev.key === "Escape") { // closes the results; a second Escape clears the box
        ev.preventDefault();
        if (!list.hidden) close();
        else { q.value = ""; status.textContent = ""; }
      }
    });
    document.addEventListener("keydown", (ev) => {
      if (ev.key !== "/" || ev.ctrlKey || ev.metaKey || ev.altKey || ev.target.closest?.("input, textarea, select, [contenteditable]")) return;
      ev.preventDefault();
      q.focus();
      q.select();
    });
    document.addEventListener("click", (ev) => { if (!ev.target.closest(".search")) close(); });
  }

  load().catch(() => {});
  return { el, call, load, onState };
})();
