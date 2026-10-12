package devapp

// The site in a real browser: a headless Chrome (or Chromium; $CHROME names another), driven over
// the DevTools protocol with the websocket package Dev already uses. Without a browser, or with
// -short, the test skips.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func findChrome() string {
	if c := os.Getenv("CHROME"); c != "" {
		return c
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// browser is one tab of a headless Chrome.
type browser struct {
	t       *testing.T
	ws      *websocket.Conn
	wmu     sync.Mutex
	mu      sync.Mutex
	next    int
	waiting map[int]chan cdpMessage
	session string
	errs    []string // exceptions and console errors on the page
}

type cdpMessage struct {
	ID        int             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct{ Message string }
	SessionID string `json:"sessionId,omitempty"`
}

func startBrowser(t *testing.T, chrome string) *browser {
	profile, err := os.MkdirTemp("", "devapp-chrome-")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(chrome, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile,
		"--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-background-networking",
		"--disable-sync", "--disable-gpu", "--mute-audio", "--window-size=1280,900", "about:blank")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(profile)
		t.Skipf("no browser: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	var b *browser
	t.Cleanup(func() { // the browser closes itself and its other processes; killed if it does not
		if b != nil {
			b.wmu.Lock()
			b.ws.WriteJSON(map[string]any{"id": 1 << 30, "method": "Browser.close"})
			b.wmu.Unlock()
		}
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			<-exited
		}
		os.RemoveAll(profile)
	})
	found := make(chan string, 1)
	go func() { // "DevTools listening on ws://127.0.0.1:<port>/devtools/browser/<id>"
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if _, url, ok := strings.Cut(sc.Text(), "DevTools listening on "); ok {
				found <- strings.TrimSpace(url)
				break
			}
		}
		io.Copy(io.Discard, stderr)
	}()
	var url string
	select {
	case url = <-found:
	case <-time.After(30 * time.Second):
		t.Fatal("the browser did not start")
	}
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	b = &browser{t: t, ws: ws, waiting: map[int]chan cdpMessage{}}
	go b.read()
	var target struct {
		TargetID string `json:"targetId"`
	}
	b.callIn("", "Target.createTarget", map[string]any{"url": "about:blank"}, &target)
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	b.callIn("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, &attached)
	b.session = attached.SessionID
	b.call("Runtime.enable", nil, nil)
	b.call("Page.enable", nil, nil)
	return b
}

func (b *browser) read() {
	for {
		var m cdpMessage
		if err := b.ws.ReadJSON(&m); err != nil {
			b.mu.Lock()
			for id, ch := range b.waiting {
				close(ch)
				delete(b.waiting, id)
			}
			b.mu.Unlock()
			return
		}
		b.mu.Lock()
		if ch := b.waiting[m.ID]; m.ID != 0 && ch != nil {
			ch <- m
			delete(b.waiting, m.ID)
		}
		switch m.Method {
		case "Runtime.exceptionThrown":
			var p struct {
				ExceptionDetails struct {
					Text      string
					Exception *struct{ Description string }
				}
			}
			json.Unmarshal(m.Params, &p)
			msg := p.ExceptionDetails.Text
			if e := p.ExceptionDetails.Exception; e != nil {
				msg += " " + e.Description
			}
			b.errs = append(b.errs, msg)
		case "Runtime.consoleAPICalled":
			var p struct {
				Type string
				Args []struct {
					Value       any
					Description string
				}
			}
			json.Unmarshal(m.Params, &p)
			if p.Type == "error" {
				b.errs = append(b.errs, fmt.Sprintf("console.error %v", p.Args))
			}
		}
		b.mu.Unlock()
	}
}

func (b *browser) callIn(session, method string, params, out any) {
	b.t.Helper()
	b.mu.Lock()
	b.next++
	id := b.next
	ch := make(chan cdpMessage, 1)
	b.waiting[id] = ch
	b.mu.Unlock()
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if session != "" {
		msg["sessionId"] = session
	}
	b.wmu.Lock()
	err := b.ws.WriteJSON(msg)
	b.wmu.Unlock()
	if err != nil {
		b.t.Fatalf("%s: %v", method, err)
	}
	select {
	case m, ok := <-ch:
		if !ok {
			b.t.Fatalf("%s: the browser went away", method)
		}
		if m.Error != nil {
			b.t.Fatalf("%s: %s", method, m.Error.Message)
		}
		if out != nil {
			json.Unmarshal(m.Result, out)
		}
	case <-time.After(30 * time.Second):
		b.t.Fatalf("%s: no answer", method)
	}
}

func (b *browser) call(method string, params, out any) {
	b.t.Helper()
	b.callIn(b.session, method, params, out)
}

// eval runs JavaScript in the page (awaiting a promise) and decodes its value into out.
func (b *browser) eval(js string, out any) {
	b.t.Helper()
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string
			Exception *struct{ Description string }
		} `json:"exceptionDetails"`
	}
	b.call("Runtime.evaluate", map[string]any{"expression": js, "returnByValue": true, "awaitPromise": true}, &r)
	if e := r.ExceptionDetails; e != nil {
		desc := e.Text
		if e.Exception != nil {
			desc = e.Exception.Description
		}
		b.t.Fatalf("%s: %s", js, desc)
	}
	if out != nil && len(r.Result.Value) > 0 {
		if err := json.Unmarshal(r.Result.Value, out); err != nil {
			b.t.Fatalf("%s: %s: %v", js, r.Result.Value, err)
		}
	}
}

func (b *browser) is(js string) bool {
	b.t.Helper()
	var ok bool
	b.eval(js, &ok)
	return ok
}

// waitFor waits until js is true in the page.
func (b *browser) waitFor(what, js string) {
	b.t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(30 * time.Millisecond) {
		if b.is(js) {
			return
		}
	}
	b.t.Fatalf("waited in vain for %s (%s)", what, js)
}

// open loads a page and waits until its scripts ran and its state arrived.
func (b *browser) open(url string) {
	b.t.Helper()
	b.eval(`window.__stale = true`, nil)
	b.call("Page.navigate", map[string]any{"url": url}, nil)
	b.waitFor(url, `!window.__stale && document.readyState === "complete" && typeof Dev === "object" && !!document.querySelector("#langs")`)
}

func (b *browser) reload() {
	b.t.Helper()
	var href string
	b.eval(`location.href`, &href)
	b.open(href)
}

// press presses a key in the page, as on a keyboard.
func (b *browser) press(key string) {
	b.t.Helper()
	codes := map[string]struct {
		code string
		vk   int
		text string
	}{
		" ": {"Space", 32, " "}, "/": {"Slash", 191, "/"}, "Enter": {"Enter", 13, "\r"}, "Escape": {"Escape", 27, ""},
		"ArrowDown": {"ArrowDown", 40, ""}, "ArrowUp": {"ArrowUp", 38, ""}, "Tab": {"Tab", 9, ""},
	}
	k, ok := codes[key]
	if !ok {
		b.t.Fatalf("press: no key %q", key)
	}
	down := map[string]any{"type": "keyDown", "key": key, "code": k.code, "windowsVirtualKeyCode": k.vk}
	if k.text != "" {
		down["text"] = k.text
	}
	b.call("Input.dispatchKeyEvent", down, nil)
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": key, "code": k.code, "windowsVirtualKeyCode": k.vk}, nil)
}

// noErrors: nothing went wrong in the page's scripts so far.
func (b *browser) noErrors(where string) {
	b.t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.errs {
		b.t.Errorf("%s: %s", where, e)
	}
	b.errs = nil
}

// visible counts the code blocks shown for each language.
const visibleJS = `(() => { const n = {}; for (const f of document.querySelectorAll("figure.code")) if (f.checkVisibility()) n[f.dataset.lang] = (n[f.dataset.lang] || 0) + 1; return n; })()`

func TestPagesInBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: no browser")
	}
	chrome := findChrome()
	if chrome == "" {
		t.Skip("no Chrome or Chromium on PATH (or $CHROME)")
	}
	a := New(Config{RobotToken: "tok", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "test"})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	a.cfg.PublicURL = ts.URL
	b := startBrowser(t, chrome)
	infos, err := loadExamples()
	if err != nil {
		t.Fatal(err)
	}
	all := func() map[string]int {
		n := map[string]int{}
		for _, l := range languages {
			n[l.ID] = len(infos)
		}
		return n
	}
	shown := func(want map[string]int) {
		t.Helper()
		var got map[string]int
		b.eval(visibleJS, &got)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("code blocks shown %v, want %v", got, want)
		}
	}
	saved := func() string {
		var s string
		b.eval(`localStorage.getItem("dev.langs") || ""`, &s)
		return s
	}

	// The language selector: every language by default.
	b.open(ts.URL + "/docs/examples")
	shown(all())
	if !b.is(`[...document.querySelectorAll("#langs input[name=lang]")].every((c) => c.checked)`) {
		t.Fatal("not every chip is on")
	}
	// Off with a click (Python), and with the keyboard (Go: focus, space).
	b.eval(`document.querySelector("#langs input[value=python]").click()`, nil)
	b.eval(`document.querySelector("#langs input[value=go]").focus()`, nil)
	b.press(" ")
	want := all()
	delete(want, "python")
	delete(want, "go")
	shown(want)
	if s := saved(); s != `["curl","js","mcp"]` {
		t.Fatalf("remembered %q", s)
	}
	// Remembered: on the next visit, and on another page.
	b.reload()
	shown(want)
	b.open(ts.URL + "/docs/api")
	if b.is(`document.querySelector("figure.code[data-lang=python]").checkVisibility()`) || !b.is(`document.querySelector("figure.code[data-lang=js]").checkVisibility()`) {
		t.Fatal("the API page does not follow the choice")
	}
	var broken []string // an argument's type of one word ("boolean") on one line, not split
	b.eval(`[...document.querySelectorAll("tr[id] td:nth-child(2)")].filter((td) => !td.textContent.includes(" ")).filter((td) => {
		const r = document.createRange(); r.selectNodeContents(td); return r.getClientRects().length > 1; }).map((td) => td.closest("tr").id)`, &broken)
	if len(broken) > 0 {
		t.Fatalf("argument types split over lines: %v", broken)
	}
	// On again; turning off the last one turns them all on.
	b.open(ts.URL + "/docs/examples")
	b.eval(`document.querySelector("#langs input[value=python]").click()`, nil)
	want["python"] = len(infos)
	shown(want)
	b.eval(`for (const c of document.querySelectorAll("#langs input[name=lang]:checked")) c.click()`, nil)
	shown(all())
	if s := saved(); s != `["curl","python","js","go","mcp"]` {
		t.Fatalf("remembered %q after all off", s)
	}

	// ?lang= picks for this page only, until "keep".
	b.open(ts.URL + "/docs/examples?lang=go,javascript")
	shown(map[string]int{"go": len(infos), "js": len(infos)})
	if !b.is(`!document.getElementById("keepBtn").hidden`) || saved() != `["curl","python","js","go","mcp"]` {
		t.Fatal("a link's languages are kept without asking")
	}
	b.eval(`document.getElementById("keepBtn").click()`, nil)
	if s := saved(); s != `["js","go"]` || b.is(`location.search.includes("lang")`) {
		t.Fatalf("keep: %q, %v", s, b.is(`location.search`))
	}
	b.open(ts.URL + "/docs/examples")
	shown(map[string]int{"go": len(infos), "js": len(infos)})

	// Tabs: one language at a time, the same across the page, remembered, as wide as the group.
	b.eval(`document.getElementById("tabsBtn").click()`, nil)
	shown(map[string]int{"js": len(infos)})
	if !b.is(`[...document.querySelectorAll(".langs")].every((g) => { const f = [...g.querySelectorAll(".code")].find((x) => x.checkVisibility()); return f && Math.abs(f.getBoundingClientRect().width - g.getBoundingClientRect().width) < 2; })`) {
		t.Fatal("in tabs, a code block does not take the group's width")
	}
	b.eval(`document.querySelector(".tab[data-lang=go]").click()`, nil)
	shown(map[string]int{"go": len(infos)})
	b.reload()
	shown(map[string]int{"go": len(infos)})
	b.eval(`document.getElementById("tabsBtn").click()`, nil)
	b.eval(`for (const c of document.querySelectorAll("#langs input[name=lang]:not(:checked)")) c.click()`, nil)
	shown(all())
	b.noErrors("the language selector")

	// No storage (blocked, or a private window): every language, and the chips still work here.
	var script struct {
		Identifier string `json:"identifier"`
	}
	b.call("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": `Object.defineProperty(window, "localStorage", { get() { throw new DOMException("blocked", "SecurityError"); } });`}, &script)
	b.eval(`localStorage.setItem("dev.langs", JSON.stringify(["go"]))`, nil)
	b.open(ts.URL + "/docs/examples")
	if !b.is(`(() => { try { localStorage; return false; } catch (e) { return true; } })()`) {
		t.Fatal("the storage is not blocked")
	}
	shown(all())
	b.eval(`document.querySelector("#langs input[value=curl]").click()`, nil)
	want = all()
	delete(want, "curl")
	shown(want)
	b.noErrors("without storage")
	b.call("Page.removeScriptToEvaluateOnNewDocument", map[string]any{"identifier": script.Identifier}, nil)
	b.open(ts.URL + "/docs/examples")
	b.eval(`localStorage.clear()`, nil)

	// Today's robot card still works: pairing by the QR code's link, Try it, a key, events, Run.
	robot := connectRobot(t, ts.URL)
	robot.waitFor("pair code", func() bool { return robot.code != "" })
	b.eval(`window.__stale = true`, nil)
	b.call("Page.navigate", map[string]any{"url": ts.URL + "/pair?code=" + robot.code}, nil)
	b.waitFor("the robot's card", `!window.__stale && location.pathname === "/robots" && !!document.querySelector("#robots .card .dot.on")`)
	b.eval(`[...document.querySelectorAll("#robots button")].find((x) => x.textContent === "nod").click()`, nil)
	robot.waitFor("nod from Try it", func() bool { return len(robot.cmds) > 0 && robot.cmds[len(robot.cmds)-1].Command == "nod" })
	b.eval(`[...document.querySelectorAll("#robots button")].find((x) => x.textContent === "New key").click()`, nil)
	b.waitFor("the new key", `[...document.querySelectorAll("#robots .key pre")].some((p) => p.textContent.startsWith("sdev_"))`)
	b.waitFor("the key listed", `[...document.querySelectorAll("#robots button")].some((x) => x.textContent === "Delete")`)
	patSeen := `[...document.querySelectorAll("#robots .events li")].some((li) => li.textContent.includes("pat"))`
	for i := 0; i < 50 && !b.is(patSeen); i++ { // the page's event stream may still be connecting
		robot.event("head_release", map[string]any{"ms": 300.0})
		time.Sleep(100 * time.Millisecond)
	}
	b.waitFor("the pat in Events", patSeen)
	b.open(ts.URL + "/docs/examples")
	b.waitFor("Run's robot", `document.getElementById("exRobot").textContent.includes("chan-1")`)
	robot.mu.Lock()
	n := len(robot.cmds)
	robot.mu.Unlock()
	b.eval(`document.querySelector(".run[data-run=hello]").click()`, nil)
	b.waitFor("hello's run", `document.getElementById("ex-hello").closest("section").querySelector(".out").textContent.includes("done")`)
	robot.mu.Lock()
	ran := robot.cmds[n:]
	robot.mu.Unlock()
	if len(ran) != 2 || ran[0].Command != "leds" || ran[1].Command != "nod" {
		t.Fatalf("Run hello: %+v", ran)
	}
	b.noErrors("the robot's card")

	// Phone width: no page scrolls sideways.
	b.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 390, "height": 844, "deviceScaleFactor": 2, "mobile": true}, nil)
	for _, p := range sitePages {
		b.open(ts.URL + p.Path)
		var w []int
		b.eval(`[document.documentElement.scrollWidth, window.innerWidth]`, &w)
		if w[0] > w[1] {
			var wide []string
			b.eval(`[...document.querySelectorAll("body *")].filter((e) => e.getBoundingClientRect().right > window.innerWidth + 1).slice(0, 5).map((e) => e.tagName + "." + e.className)`, &wide)
			t.Errorf("%s at phone width: %d px wide in %d (%v)", p.Path, w[0], w[1], wide)
		}
	}
	b.noErrors("phone width")
}
