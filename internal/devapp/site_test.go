package devapp

import (
	"bufio"
	"encoding/json"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/i41-eu/q41eu-dev/examples"
	"github.com/i41-eu/q41eu-dev/gesture"
)

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

var (
	idAttr   = regexp.MustCompile(`\sid="([^"]+)"`)
	linkAttr = regexp.MustCompile(`\s(?:href|src)="([^"]+)"`)
)

// The site: every page renders, every example is there in each of its languages, every internal
// link answers and its anchor exists.
func TestSite(t *testing.T) {
	a := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PublicURL: "https://dev.example", Version: "test"})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	pages := map[string]string{}
	for _, p := range sitePages {
		code, body := get(t, ts.URL+p.Path)
		if code != http.StatusOK || !strings.Contains(body, "<h1>") {
			t.Fatalf("%s: %d", p.Path, code)
		}
		pages[p.Path] = body
		seen := map[string]bool{}
		for _, m := range idAttr.FindAllStringSubmatch(body, -1) {
			if seen[m[1]] {
				t.Errorf("%s: the id %q twice", p.Path, m[1])
			}
			seen[m[1]] = true
		}
		for _, l := range languages {
			if !strings.Contains(body, `<input type="checkbox" name="lang" value="`+l.ID+`" checked>`) {
				t.Errorf("%s: no chip for %s", p.Path, l.Name)
			}
		}
	}

	// Every example in each of its languages, whole, on the examples page.
	infos, err := loadExamples()
	if err != nil {
		t.Fatal(err)
	}
	exPage := html.UnescapeString(pages["/docs/examples"])
	for _, ex := range infos {
		if !strings.Contains(pages["/docs/examples"], `id="ex-`+ex.ID+`"`) {
			t.Errorf("example %s: not on the page", ex.ID)
		}
		for _, l := range languages {
			file := ex.Files[l.ID]
			if file == "" {
				t.Errorf("example %s: not in %s", ex.ID, l.Name)
				continue
			}
			b, err := fs.ReadFile(examples.FS, file)
			if err != nil {
				t.Fatalf("example %s: %v", ex.ID, err)
			}
			if !strings.Contains(exPage, string(b)) {
				t.Errorf("example %s: its %s code is not on the page", ex.ID, l.Name)
			}
			if run := ex.Run[l.ID]; l.ID != "mcp" && !strings.Contains(run, strings.TrimSuffix(strings.TrimSuffix(file, "/main.go"), ".go")) {
				t.Errorf("example %s: %q does not run %s", ex.ID, run, file)
			}
		}
	}
	for _, l := range languages {
		if n := strings.Count(pages["/docs/examples"], `<figure class="code" data-lang="`+l.ID+`">`); n != len(infos) {
			t.Errorf("%s: %d code blocks on the examples page, want %d", l.Name, n, len(infos))
		}
	}

	// Every action and every event on the API page, each in exactly one part.
	api := pages["/docs/api"]
	for _, act := range Actions {
		if !strings.Contains(api, `<h3 id="`+act.Name+`">`) {
			t.Errorf("action %s: not on the API page", act.Name)
		}
		n := 0
		for _, p := range apiParts {
			n += strings.Count(strings.Join(p.Actions, " ")+" ", act.Name+" ")
		}
		if n != 1 {
			t.Errorf("action %s: in %d parts, want 1", act.Name, n)
		}
		for _, p := range act.Params {
			if !strings.Contains(api, `id="`+act.Name+"-"+p.Name+`"`) {
				t.Errorf("argument %s %s: not on the API page", act.Name, p.Name)
			}
		}
	}
	for _, e := range gesture.Names {
		if !strings.Contains(api, `id="ev-`+e.Name+`"`) {
			t.Errorf("event %s: not on the API page", e.Name)
		}
	}
	for _, p := range apiParts {
		if !strings.Contains(api, `<h2 id="`+p.ID+`">`) {
			t.Errorf("part %s: not on the API page", p.ID)
		}
	}

	// Every internal link answers, and its anchor is on its page.
	checked := map[string]bool{}
	for from, body := range pages {
		for _, m := range linkAttr.FindAllStringSubmatch(body, -1) {
			link := html.UnescapeString(m[1])
			if strings.HasPrefix(link, "#") {
				link = from + link
			}
			if !strings.HasPrefix(link, "/") || strings.HasPrefix(link, "//") || strings.HasPrefix(link, "/auth/") || checked[link] {
				continue
			}
			checked[link] = true
			path, frag, _ := strings.Cut(link, "#")
			if code, _ := get(t, ts.URL+path); code != http.StatusOK {
				t.Errorf("%s links to %s: %d", from, link, code)
			}
			if frag != "" && !strings.Contains(pages[path], ` id="`+frag+`"`) {
				t.Errorf("%s links to %s: no such anchor", from, link)
			}
		}
	}
	if len(checked) < 20 {
		t.Errorf("only %d internal links", len(checked))
	}

	// The pages from disk (-ui-dir) are the same.
	dev := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PublicURL: "https://dev.example", Version: "test", UIDir: "site"})
	ds := httptest.NewServer(dev.Handler())
	defer ds.Close()
	for _, p := range sitePages {
		if _, body := get(t, ds.URL+p.Path); body != pages[p.Path] {
			t.Errorf("%s from disk differs", p.Path)
		}
	}
	if code, js := get(t, ts.URL+"/site/site.js"); code != http.StatusOK || !strings.Contains(js, "const Dev") {
		t.Errorf("site.js: %d", code)
	}
	if code, _ := get(t, ts.URL+"/site/site.css"); code != http.StatusNotFound {
		t.Errorf("only the scripts under /site/: %d", code)
	}
}

// Each action's example is a valid call (the API's own check) with a sentence for an agent; the
// MCP examples' tool calls are valid calls too.
func TestSnippets(t *testing.T) {
	for _, act := range Actions {
		var in args
		if err := json.Unmarshal([]byte(act.Example), &in); err != nil {
			t.Fatalf("%s: example: %v", act.Name, err)
		}
		if err := in.check(act.Params); err != nil {
			t.Errorf("%s: example %s: %v", act.Name, act.Example, err)
		}
		if act.Prompt == "" {
			t.Errorf("%s: no prompt", act.Name)
		}
		blocks, err := snippets(act)
		if err != nil || len(blocks) != len(languages) {
			t.Fatalf("%s: %v", act.Name, err)
		}
	}
	ask := Actions[slices.IndexFunc(Actions, func(a Action) bool { return a.Name == "ask" })]
	blocks, _ := snippets(ask)
	want := map[string]string{
		"python": `print(act("ask", question="Deploy now?", answers=["Yes", "Not yet"]))`,
		"js":     `console.log(await act("ask", { question: "Deploy now?", answers: ["Yes", "Not yet"] }));`,
		"go":     "var out map[string]any\nerr := robot.Do(ctx, \"ask\", map[string]any{\"question\": \"Deploy now?\", \"answers\": []string{\"Yes\", \"Not yet\"}}, &out)",
	}
	for _, b := range blocks {
		if w, ok := want[b.Lang]; ok && b.Code != w {
			t.Errorf("%s:\n%s\nwant\n%s", b.Lang, b.Code, w)
		}
		if b.Lang == "curl" && !strings.Contains(b.Code, `-d '{"question":"Deploy now?","answers":["Yes","Not yet"]}'`) {
			t.Errorf("curl: %s", b.Code)
		}
	}

	files, _ := fs.Glob(examples.FS, "mcp/*.txt")
	calls := 0
	for _, f := range files {
		b, _ := fs.ReadFile(examples.FS, f)
		for line := range strings.Lines(string(b)) {
			tool, argsJSON, ok := strings.Cut(strings.TrimSpace(line), " ")
			name, isTool := strings.CutPrefix(tool, "robot_")
			if !ok || !isTool {
				continue
			}
			i := slices.IndexFunc(Actions, func(a Action) bool { return a.Name == name })
			if i < 0 {
				t.Errorf("%s: no tool %s", f, tool)
				continue
			}
			var in args
			if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
				t.Errorf("%s: %s: %v", f, tool, err)
			} else if err := in.check(Actions[i].Params); err != nil {
				t.Errorf("%s: %s: %v", f, tool, err)
			}
			calls++
		}
	}
	if len(files) < 4 || calls < 10 {
		t.Errorf("MCP examples: %d files, %d tool calls", len(files), calls)
	}
}

// The readme's API table lists the actions, in the action table's order.
func TestReadmeTable(t *testing.T) {
	f, err := os.Open("../../readme.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	row := regexp.MustCompile("^\\| `([a-z_]+)` \\|")
	var got []string
	inAPI := false
	for sc := bufio.NewScanner(f); sc.Scan(); {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			inAPI = line == "## The API"
		}
		if m := row.FindStringSubmatch(line); inAPI && m != nil {
			got = append(got, m[1])
		}
	}
	var want []string
	for _, a := range Actions {
		want = append(want, a.Name)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("readme's API table: %v\nthe actions: %v", got, want)
	}
}
