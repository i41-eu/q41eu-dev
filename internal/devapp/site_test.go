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

// The search's index has every action, argument, event, example, page and section, each pointing
// at its anchor; its synonyms lead only to words the site has.
func TestSearchIndex(t *testing.T) {
	a := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PublicURL: "https://dev.example", Version: "test"})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	res, err := http.Get(ts.URL + "/docs/search.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/json" || len(raw) > 96<<10 {
		t.Fatalf("search.json: %d %s, %d bytes", res.StatusCode, res.Header.Get("Content-Type"), len(raw))
	}
	var idx struct {
		Kinds    []string
		Entries  []searchEntry
		Synonyms map[string][]string
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, p := range sitePages {
		_, pages[p.Path] = get(t, ts.URL+p.Path)
	}
	byURL := map[string]searchEntry{}
	words := map[string]bool{}
	split := regexp.MustCompile(`[^a-z0-9_]+`)
	for _, e := range idx.Entries {
		if _, dup := byURL[e.URL]; dup {
			t.Errorf("%s twice", e.URL)
		}
		byURL[e.URL] = e
		if e.Name == "" || !slices.Contains(idx.Kinds, e.Kind) {
			t.Errorf("%+v", e)
		}
		path, frag, _ := strings.Cut(e.URL, "#")
		if pages[path] == "" || (frag != "" && !strings.Contains(pages[path], ` id="`+frag+`"`)) {
			t.Errorf("%s (%s): no such page or anchor", e.URL, e.Name)
		}
		for _, w := range split.Split(strings.ToLower(strings.Join(append([]string{e.Name, e.Text, e.In}, e.Parts...), " ")), -1) {
			words[w] = true
		}
	}
	want := func(url, kind, name string) {
		t.Helper()
		if e, ok := byURL[url]; !ok || e.Kind != kind || e.Name != name {
			t.Errorf("%s: %+v, want %s %s", url, e, kind, name)
		}
	}
	for _, act := range Actions {
		want("/docs/api#"+act.Name, "Actions", act.Name)
		for _, p := range act.Params {
			want("/docs/api#"+act.Name+"-"+p.Name, "Arguments", p.Name)
		}
	}
	for _, e := range gesture.Names {
		want("/docs/api#ev-"+e.Name, "Events", e.Name)
	}
	infos, _ := loadExamples()
	for _, ex := range infos {
		want("/docs/examples#ex-"+ex.ID, "Examples", ex.Title)
	}
	h2 := regexp.MustCompile(`<h2 id="([^"]+)">`)
	for _, p := range sitePages {
		want(p.Path, "Pages", p.Title)
		for _, m := range h2.FindAllStringSubmatch(pages[p.Path], -1) {
			if _, ok := byURL[p.Path+"#"+m[1]]; !ok {
				t.Errorf("the section %s#%s is not in the index", p.Path, m[1])
			}
		}
	}
	if e := byURL["/docs/start#agent"]; e.Name != "5. An AI agent" || e.In != "Getting started" || !strings.Contains(e.Text, "MCP tools") {
		t.Errorf("a section: %+v", e)
	}
	for w, syns := range idx.Synonyms {
		for _, s := range syns {
			if !words[s] {
				t.Errorf("synonym %s -> %s: not a word of the site", w, s)
			}
		}
	}
}

// Everything GET /v1/actions lists is documented: each action with its help, its arguments (each
// with its help and choices) and a call in every language on the API page, in the search, in
// OpenAPI and MCP, and in the readme's table; each event with what it means on the API page, in
// the search and in the readme. A new action or event without its docs fails here.
func TestAPIDocs(t *testing.T) {
	a := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PublicURL: "https://dev.example", Version: "test"})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	var list struct {
		Actions []struct {
			Name, Help string
			Params     []struct {
				Name, Type, Help string
				Enum             []string
			}
		}
		Events []string
	}
	_, raw := get(t, ts.URL+"/v1/actions")
	if err := json.Unmarshal([]byte(raw), &list); err != nil || len(list.Actions) == 0 || len(list.Events) == 0 {
		t.Fatalf("/v1/actions: %v %s", err, raw)
	}
	_, api := get(t, ts.URL+"/docs/api")
	_, idxRaw := get(t, ts.URL+"/docs/search.json")
	var idx struct{ Entries []searchEntry }
	json.Unmarshal([]byte(idxRaw), &idx)
	found := map[string]searchEntry{}
	for _, e := range idx.Entries {
		found[e.URL] = e
	}
	paths := a.openAPI()["paths"].(map[string]any)
	tools, _ := a.mcpCall(nil, access{}, rpcRequest{Method: "tools/list"})
	toolSchemas := map[string]map[string]any{}
	for _, tool := range tools.(map[string]any)["tools"].([]map[string]any) {
		toolSchemas[tool["name"].(string)] = tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
	}
	readme, err := os.ReadFile("../../readme.md")
	if err != nil {
		t.Fatal(err)
	}
	apiSection := string(readme)[strings.Index(string(readme), "## The API"):]
	apiSection = apiSection[:strings.Index(apiSection, "\n## ")]

	article := func(name string) string {
		return regexp.MustCompile(`(?s)<article class="action">\s*<h3 id="` + regexp.QuoteMeta(name) + `">.*?</article>`).FindString(api)
	}
	for _, act := range list.Actions {
		art := article(act.Name)
		switch {
		case art == "":
			t.Errorf("%s: no section on the API page", act.Name)
			continue
		case len(act.Help) < 20 || !strings.Contains(html.UnescapeString(art), act.Help):
			t.Errorf("%s: its help %q is not on the API page", act.Name, act.Help)
		}
		for _, l := range languages {
			block := regexp.MustCompile(`(?s)<figure class="code" data-lang="` + l.ID + `">.*?</figure>`).FindString(art)
			if block == "" || strings.Contains(block, `class="missing`) || !strings.Contains(block, act.Name) {
				t.Errorf("%s: no call in %s on the API page", act.Name, l.Name)
			}
		}
		if e := found["/docs/api#"+act.Name]; e.Kind != "Actions" || e.Text == "" {
			t.Errorf("%s: not in the search", act.Name)
		}
		p := "/v1/robots/{robot}/" + act.Name
		if act.Name == "state" {
			p = "/v1/robots/{robot}"
		}
		if paths[p] == nil {
			t.Errorf("%s: not in OpenAPI", act.Name)
		}
		schema, isTool := toolSchemas["robot_"+act.Name]
		if !isTool || len(schema) != len(act.Params) {
			t.Errorf("%s: MCP's tool robot_%s has %d arguments, want %d", act.Name, act.Name, len(schema), len(act.Params))
		}
		row := regexp.MustCompile("(?m)^\\| `" + regexp.QuoteMeta(act.Name) + "` \\|.*$").FindString(apiSection)
		if row == "" {
			t.Errorf("%s: not in the readme's API table", act.Name)
		}
		for _, prm := range act.Params {
			tr := regexp.MustCompile(`(?s)<tr id="` + regexp.QuoteMeta(act.Name+"-"+prm.Name) + `">.*?</tr>`).FindString(art)
			plain := html.UnescapeString(tr)
			if tr == "" || len(prm.Help) < 3 || !strings.Contains(plain, prm.Help) || !strings.Contains(plain, prm.Type) {
				t.Errorf("%s %s: no row with its type and help %q on the API page", act.Name, prm.Name, prm.Help)
			}
			for _, choice := range prm.Enum {
				if !strings.Contains(plain, choice) {
					t.Errorf("%s %s: its choice %q is not on the API page", act.Name, prm.Name, choice)
				}
			}
			if e := found["/docs/api#"+act.Name+"-"+prm.Name]; e.Kind != "Arguments" || e.In != act.Name {
				t.Errorf("%s %s: not in the search", act.Name, prm.Name)
			}
			if _, ok := schema[prm.Name]; !ok {
				t.Errorf("%s %s: not in MCP's tool", act.Name, prm.Name)
			}
			if !strings.Contains(row, "`"+prm.Name+"`") {
				t.Errorf("%s %s: not in the readme's API table: %s", act.Name, prm.Name, row)
			}
		}
	}
	var eventsHelp string
	for _, act := range list.Actions {
		if act.Name == "events" {
			eventsHelp = act.Help
		}
	}
	for _, name := range list.Events {
		means := regexp.MustCompile(`(?s)<dt id="ev-` + regexp.QuoteMeta(name) + `"><code>[^<]*</code></dt><dd>([^<]+)</dd>`).FindStringSubmatch(api)
		if means == nil || len(strings.TrimSpace(means[1])) < 5 {
			t.Errorf("event %s: not on the API page with what it means", name)
		}
		if e := found["/docs/api#ev-"+name]; e.Kind != "Events" || e.In == "" || e.Text == "" {
			t.Errorf("event %s: not in the search", name)
		}
		if !strings.Contains(apiSection, "`"+name+"`") {
			t.Errorf("event %s: not in the readme's The API", name)
		}
		if slices.ContainsFunc(gesture.Names, func(n struct{ Name, Means string }) bool { return n.Name == name }) &&
			!regexp.MustCompile(`\b`+name+`\b`).MatchString(eventsHelp) {
			t.Errorf("event %s: not in the events action's help", name)
		}
	}
	if !slices.Contains(list.Events, "connected") {
		t.Error("/v1/actions does not list connected")
	}
}
