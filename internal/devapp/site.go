package devapp

// Dev's site: a few pages (home, your robots, getting started, the API by part, examples), made
// from the action table, the events and the examples' manifest (examples/examples.json), and
// rendered once at start (from disk on every request with Config.UIDir). Every code block is on
// the page in every language; the language selector in the header only hides some (docs/site.md).

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/i41-eu/q41eu-dev/examples"
	"github.com/i41-eu/q41eu-dev/gesture"
)

//go:embed site
var siteFiles embed.FS

// language is one of the selector's languages.
type language struct{ ID, Name string }

var languages = []language{{"curl", "curl"}, {"python", "Python"}, {"js", "JavaScript"}, {"go", "Go"}, {"mcp", "MCP"}}

// sitePage is a page of the site, in the order of the header's links. A page with a Parent has
// no link of its own there: its parent's link is marked while it is open.
type sitePage struct{ Path, Nav, Title, File, Parent string }

var sitePages = []sitePage{
	{"/", "Dev", "Stackchan Dev", "home.html", ""},
	{"/robots", "Your robots", "Your robots", "robots.html", ""},
	{"/docs/start", "Getting started", "Getting started", "start.html", ""},
	{"/docs/api", "API", "The API by part", "api.html", ""},
	{"/docs/examples", "Examples", "Examples", "examples.html", ""},
	{"/docs/case-studies", "Case studies", "Case studies", "cases.html", ""},
	{"/docs/case-studies/focus", "", "Focus, a case study", "case-focus.html", "/docs/case-studies"},
	{"/docs/case-studies/pet", "", "Pet, a case study", "case-pet.html", "/docs/case-studies"},
}

// apiPart is a part of the robot on the API page, with the actions that use it. Every action is
// in exactly one part (TestSite checks it).
type apiPart struct {
	ID, Name string
	Actions  []string
}

var apiParts = []apiPart{
	{"screen", "Screen and face", []string{"show", "say", "face"}},
	{"head", "Head", []string{"move"}},
	{"lights", "LEDs", []string{"leds"}},
	{"speaker", "Speaker", []string{"sound"}},
	{"asking", "Questions", []string{"ask"}},
	{"people", "What people do", []string{"events"}},
	{"readings", "State and readings", []string{"state"}},
	{"raw", "Any robot command", []string{"command"}},
}

// eventGroup: the events by the part that senses them. Every event is in exactly one.
type eventGroup struct {
	ID, Name string
	Events   []string
}

var eventGroups = []eventGroup{
	{"touch", "Touch screen", []string{"tap", "long_press"}},
	{"head-touch", "Head touch", []string{"pat", "pet", "stroke"}},
	{"motion", "Motion", []string{"shaken", "picked_up"}},
	{"proximity", "Light and proximity", []string{"near", "away"}},
	{"nfc", "NFC", []string{"tag", "tag_removed"}},
	{"infrared", "Infrared", []string{"remote"}},
	{"power", "Power", []string{"button", "plugged", "unplugged"}},
}

// codeBlock is one language's code in a group of code blocks.
type codeBlock struct {
	Lang, LangName string
	Code           string
	Usage          string // how to run it
	File           string // its file under /examples/, to download ("" none)
	Download       string // the name to save it as
	Missing        bool   // not in this language yet
}

type actionView struct {
	Action
	Blocks []codeBlock
}

type partView struct {
	ID, Name string
	Actions  []actionView
}

type eventView struct{ Name, Means string }

type eventGroupView struct {
	ID, Name string
	Events   []eventView
}

// exampleInfo is an example in examples/examples.json.
type exampleInfo struct {
	ID    string            `json:"id"`
	Title string            `json:"title"`
	What  string            `json:"what"`
	Tags  []string          `json:"tags"`  // the parts it uses (apiParts' ids)
	Files map[string]string `json:"files"` // language -> its file under examples/
	Run   map[string]string `json:"run"`   // language -> how to run it, in a clone of the repository
}

type exampleView struct {
	exampleInfo
	Parts  []apiPart
	Blocks []codeBlock
}

// siteData is what every page's template gets.
type siteData struct {
	Page     sitePage
	Pages    []sitePage
	Langs    []language
	LangIDs  []string
	CSS      template.CSS
	Parts    []partView
	Events   []eventGroupView
	Examples []exampleView
	Calling  []codeBlock // the helpers the API's snippets use, per language
	Start    []codeBlock // getting started: the first program, per language
	Apps     []caseApp   // the case studies' apps
}

// site is the rendered site.
type site struct {
	pages    map[string][]byte // path -> the page
	files    fs.FS             // its scripts and pictures
	search   []byte            // /docs/search.json
	excerpts map[string]excerpt
}

// buildSite renders every page from the templates in files.
func buildSite(files fs.FS) (*site, error) {
	css, err := fs.ReadFile(files, "site.css")
	if err != nil {
		return nil, err
	}
	d := siteData{Pages: sitePages, Langs: languages, CSS: template.CSS(string(css) + langCSS())}
	for _, l := range languages {
		d.LangIDs = append(d.LangIDs, l.ID)
	}
	if d.Parts, err = partViews(); err != nil {
		return nil, err
	}
	if d.Events, err = eventViews(); err != nil {
		return nil, err
	}
	if d.Examples, err = exampleViews(); err != nil {
		return nil, err
	}
	if d.Calling, err = callingBlocks(); err != nil {
		return nil, err
	}
	d.Start = startBlocks(d.Examples)
	d.Apps = caseApps
	excerpts, err := loadExcerpts(files)
	if err != nil {
		return nil, err
	}
	funcs := template.FuncMap{
		"join": strings.Join,
		"part": func(id string) (partView, error) {
			for _, p := range d.Parts {
				if p.ID == id {
					return p, nil
				}
			}
			return partView{}, fmt.Errorf("no part %q", id)
		},
		"example": func(id string) (exampleView, error) {
			for _, e := range d.Examples {
				if e.ID == id {
					return e, nil
				}
			}
			return exampleView{}, fmt.Errorf("no example %q", id)
		},
		"excerpt": func(key string) (excerpt, error) {
			e, ok := excerpts[key]
			if !ok {
				return excerpt{}, fmt.Errorf("no excerpt %q in cases/", key)
			}
			return e, nil
		},
		"app": func(id string) (caseApp, error) {
			for _, c := range caseApps {
				if c.ID == id {
					return c, nil
				}
			}
			return caseApp{}, fmt.Errorf("no case study app %q", id)
		},
	}
	s := &site{pages: map[string][]byte{}, files: files, excerpts: excerpts}
	for _, p := range sitePages {
		t, err := template.New(p.File).Funcs(funcs).ParseFS(files, "layout.html", p.File)
		if err != nil {
			return nil, err
		}
		d.Page = p
		var b bytes.Buffer
		if err := t.ExecuteTemplate(&b, "layout", d); err != nil {
			return nil, fmt.Errorf("%s: %w", p.File, err)
		}
		s.pages[p.Path] = b.Bytes()
	}
	idx := searchIndex(d, s.pages)
	if s.search, err = json.Marshal(idx); err != nil {
		return nil, err
	}
	return s, nil
}

// langCSS hides the code of the languages not picked (the classes on <html>, set before the first
// paint), and in tabs shows one language at a time.
func langCSS() string {
	var b strings.Builder
	for _, l := range languages {
		fmt.Fprintf(&b, "html.langsel:not(.lang-%[1]s) [data-lang=%[1]s]{display:none!important}\n", l.ID)
		fmt.Fprintf(&b, "html.tabs.tab-%[1]s .langs>.code[data-lang=%[1]s]{display:flex}\n", l.ID)
		fmt.Fprintf(&b, "html.tabs.tab-%[1]s .tab[data-lang=%[1]s]{background:var(--accent);color:var(--accent-text)}\n", l.ID)
	}
	return b.String()
}

func partViews() ([]partView, error) {
	var out []partView
	for _, p := range apiParts {
		v := partView{ID: p.ID, Name: p.Name}
		for _, name := range p.Actions {
			i := slices.IndexFunc(Actions, func(a Action) bool { return a.Name == name })
			if i < 0 {
				return nil, fmt.Errorf("part %s: no action %q", p.ID, name)
			}
			blocks, err := snippets(Actions[i])
			if err != nil {
				return nil, err
			}
			v.Actions = append(v.Actions, actionView{Action: Actions[i], Blocks: blocks})
		}
		out = append(out, v)
	}
	return out, nil
}

func eventViews() ([]eventGroupView, error) {
	var out []eventGroupView
	for _, g := range eventGroups {
		v := eventGroupView{ID: g.ID, Name: g.Name}
		for _, name := range g.Events {
			i := slices.IndexFunc(gesture.Names, func(n struct{ Name, Means string }) bool { return n.Name == name })
			if i < 0 {
				return nil, fmt.Errorf("events %s: no event %q", g.ID, name)
			}
			v.Events = append(v.Events, eventView{Name: name, Means: gesture.Names[i].Means})
		}
		out = append(out, v)
	}
	return out, nil
}

// loadExamples reads the examples' manifest.
func loadExamples() ([]exampleInfo, error) {
	b, err := fs.ReadFile(examples.FS, "examples.json")
	if err != nil {
		return nil, err
	}
	var m struct {
		Examples []exampleInfo `json:"examples"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("examples.json: %w", err)
	}
	return m.Examples, nil
}

func exampleViews() ([]exampleView, error) {
	infos, err := loadExamples()
	if err != nil {
		return nil, err
	}
	var out []exampleView
	for _, ex := range infos {
		v := exampleView{exampleInfo: ex}
		for _, tag := range ex.Tags {
			i := slices.IndexFunc(apiParts, func(p apiPart) bool { return p.ID == tag })
			if i < 0 {
				return nil, fmt.Errorf("example %s: no part %q", ex.ID, tag)
			}
			v.Parts = append(v.Parts, apiParts[i])
		}
		for _, l := range languages {
			c := codeBlock{Lang: l.ID, LangName: l.Name, Usage: ex.Run[l.ID]}
			file := ex.Files[l.ID]
			if file == "" {
				c.Missing = true
				v.Blocks = append(v.Blocks, c)
				continue
			}
			b, err := fs.ReadFile(examples.FS, file)
			if err != nil {
				return nil, fmt.Errorf("example %s: %w", ex.ID, err)
			}
			c.Code = string(b)
			if l.ID != "mcp" { // the agent's part is words, not a program to save
				c.File, c.Download = file, path.Base(file)
				if c.Download == "main.go" {
					c.Download = ex.ID + ".go"
				}
			}
			v.Blocks = append(v.Blocks, c)
		}
		out = append(out, v)
	}
	return out, nil
}

// startBlocks: getting started's first program, the hello example, run in each language.
func startBlocks(exs []exampleView) []codeBlock {
	var out []codeBlock
	for _, l := range languages {
		c := codeBlock{Lang: l.ID, LangName: l.Name}
		for _, ex := range exs {
			if ex.ID == "hello" {
				c.Code = ex.Run[l.ID]
			}
		}
		if l.ID == "mcp" {
			c.Code = mcpAdd + "\n\n# then ask the agent:\n#   Say hello on the robot: a green card \"Hello\", green LEDs, a nod and the tada chime."
		}
		c.Missing = c.Code == ""
		out = append(out, c)
	}
	return out
}

const mcpAdd = `claude mcp add --transport http stackchan "$SDEV_URL/mcp" --header "Authorization: Bearer $SDEV_KEY"`

// callingBlocks are the helpers the API page's snippets use: for Python and JavaScript, the act
// function of the hello example itself.
func callingBlocks() ([]codeBlock, error) {
	py, err := exampleLines("python/hello.py", "import ", "act(")
	if err != nil {
		return nil, err
	}
	js, err := exampleLines("js/hello.mjs", "const { SDEV_URL", "await act(")
	if err != nil {
		return nil, err
	}
	code := map[string]string{
		"curl": "# every action: POST $SDEV_URL/v1/robots/$SDEV_ROBOT/<action> with its arguments as JSON\n" +
			"curl -sS -X POST \"$SDEV_URL/v1/robots/$SDEV_ROBOT/show\" \\\n" +
			"  -H \"Authorization: Bearer $SDEV_KEY\" -H \"Content-Type: application/json\" \\\n" +
			"  -d '{\"text\":\"Hello\"}'",
		"python": "# Python 3, the standard library only (as in examples/python)\n" + py,
		"js":     "// Node 18 or newer, only fetch (as in examples/js)\n" + js,
		"go": `// go get github.com/i41-eu/q41eu-dev/client
robot, err := client.FromEnv() // reads SDEV_URL, SDEV_ROBOT, SDEV_KEY
if err != nil {
	log.Fatal(err)
}
ctx := context.Background()
// any action: Do(ctx, action, its arguments, &the answer); or the named methods:
// Show, Clear, Say, Face, Gesture, Look, LEDs, LEDBar, Chime, Notes, Ask, Events, State`,
		"mcp": "# give an AI agent the robot (Claude Code here; any MCP client with Streamable HTTP)\n" + mcpAdd +
			"\n\n# every action is the tool robot_<action>, with the same arguments",
	}
	var out []codeBlock
	for _, l := range languages {
		out = append(out, codeBlock{Lang: l.ID, LangName: l.Name, Code: code[l.ID]})
	}
	return out, nil
}

// exampleLines are the lines of an example's file from the first that starts with from up to the
// next that starts with to.
func exampleLines(file, from, to string) (string, error) {
	b, err := fs.ReadFile(examples.FS, file)
	if err != nil {
		return "", err
	}
	var out []string
	in := false
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case !in && strings.HasPrefix(line, from):
			in = true
		case in && strings.HasPrefix(line, to):
			return strings.TrimRight(strings.Join(out, "\n"), "\n"), nil
		}
		if in {
			out = append(out, line)
		}
	}
	return "", fmt.Errorf("%s: no lines from %q to %q", file, from, to)
}

// snippets are one call of the action, with its example arguments, in every language.
func snippets(act Action) ([]codeBlock, error) {
	v, err := orderedJSON(act.Example)
	if err != nil || !v.obj {
		return nil, fmt.Errorf("action %s: its example is not a JSON object: %v", act.Name, err)
	}
	var compact bytes.Buffer
	json.Compact(&compact, []byte(act.Example))
	pyArgs, jsArgs := "", ""
	if len(v.keys) > 0 {
		var kw []string
		for i, k := range v.keys {
			kw = append(kw, k+"="+v.vals[i].lit("python"))
		}
		pyArgs, jsArgs = ", "+strings.Join(kw, ", "), ", "+v.lit("js")
	}
	code := map[string]string{
		"curl": "curl -sS -X POST \"$SDEV_URL/v1/robots/$SDEV_ROBOT/" + act.Name + "\" \\\n" +
			"  -H \"Authorization: Bearer $SDEV_KEY\" -H \"Content-Type: application/json\" \\\n" +
			"  -d " + shellQuote(compact.String()),
		"python": "print(act(\"" + act.Name + "\"" + pyArgs + "))",
		"js":     "console.log(await act(\"" + act.Name + "\"" + jsArgs + "));",
		"go":     "var out map[string]any\nerr := robot.Do(ctx, \"" + act.Name + "\", " + v.lit("go") + ", &out)",
		"mcp":    "# ask your agent:\n#   " + act.Prompt + "\nrobot_" + act.Name + " " + v.lit("json"),
	}
	var out []codeBlock
	for _, l := range languages {
		out = append(out, codeBlock{Lang: l.ID, LangName: l.Name, Code: code[l.ID]})
	}
	return out, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// jsonValue is decoded JSON that keeps an object's keys in their order, so a snippet lists the
// arguments as the action table's example does.
type jsonValue struct {
	obj, arr bool
	keys     []string    // an object's
	vals     []jsonValue // an object's values, an array's items
	scalar   any         // string, json.Number, bool or nil
}

func orderedJSON(s string) (jsonValue, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	v, err := decodeOrdered(dec)
	if err == nil && dec.More() {
		err = fmt.Errorf("more after the value")
	}
	return v, err
}

func decodeOrdered(dec *json.Decoder) (jsonValue, error) {
	t, err := dec.Token()
	if err != nil {
		return jsonValue{}, err
	}
	d, isDelim := t.(json.Delim)
	if !isDelim {
		return jsonValue{scalar: t}, nil
	}
	v := jsonValue{obj: d == '{', arr: d == '['}
	for dec.More() {
		if v.obj {
			k, err := dec.Token()
			if err != nil {
				return v, err
			}
			v.keys = append(v.keys, k.(string))
		}
		item, err := decodeOrdered(dec)
		if err != nil {
			return v, err
		}
		v.vals = append(v.vals, item)
	}
	_, err = dec.Token() // the closing delimiter
	return v, err
}

var identifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// lit is the value as a literal of lang: json, python, js or go.
func (v jsonValue) lit(lang string) string {
	var items []string
	for i, item := range v.vals {
		s := item.lit(lang)
		if v.obj {
			k := jsonString(v.keys[i])
			switch {
			case lang == "go":
				k = strconv.Quote(v.keys[i])
			case lang == "js" && identifier.MatchString(v.keys[i]):
				k = v.keys[i]
			}
			s = k + ": " + s
		}
		items = append(items, s)
	}
	switch {
	case v.obj && lang == "go":
		return "map[string]any{" + strings.Join(items, ", ") + "}"
	case v.obj && lang == "js" && len(items) > 0:
		return "{ " + strings.Join(items, ", ") + " }"
	case v.obj:
		return "{" + strings.Join(items, ", ") + "}"
	case v.arr && lang == "go":
		kind := "string"
		for _, item := range v.vals {
			if _, isStr := item.scalar.(string); !isStr || item.obj || item.arr {
				kind = "any"
			}
		}
		return "[]" + kind + "{" + strings.Join(items, ", ") + "}"
	case v.arr:
		return "[" + strings.Join(items, ", ") + "]"
	}
	switch x := v.scalar.(type) {
	case string:
		if lang == "go" {
			return strconv.Quote(x)
		}
		return jsonString(x)
	case json.Number:
		return x.String()
	case bool:
		if lang == "python" {
			return map[bool]string{true: "True", false: "False"}[x]
		}
		return strconv.FormatBool(x)
	}
	switch lang {
	case "python":
		return "None"
	case "go":
		return "nil"
	}
	return "null"
}

func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

/* --------------------------------- search --------------------------------- */

// searchEntry is one thing the header's search finds.
type searchEntry struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`            // one of searchKinds
	In    string   `json:"in,omitempty"`    // where it belongs: an argument's action, an event's sensor, a heading's page
	Text  string   `json:"text,omitempty"`  // what it is, in a few words
	URL   string   `json:"url"`             // its page and anchor
	Parts []string `json:"parts,omitempty"` // the parts of the robot it is about
}

var searchKinds = []string{"Actions", "Arguments", "Events", "Examples", "Pages"}

// searchSynonyms help people who do not know our words: a word of the query also matches these.
var searchSynonyms = map[string][]string{
	"light": {"leds", "near", "light_lux"}, "lights": {"leds"}, "led": {"leds"}, "lamp": {"leds"},
	"colour": {"color"}, "color": {"colour"},
	"photo": {"image", "picture"}, "picture": {"image"}, "image": {"picture"}, "jpeg": {"image"}, "png": {"image"},
	"touch": {"tap", "long_press", "pat", "pet", "stroke"}, "press": {"tap", "long_press", "button"},
	"speak": {"say", "sound"}, "talk": {"say"}, "speech": {"say"}, "bubble": {"say"}, "voice": {"say", "sound"},
	"music": {"sound", "notes"}, "melody": {"notes"}, "beep": {"chime"}, "play": {"sound"}, "volume": {"sound"},
	"question": {"ask"}, "choice": {"ask"}, "menu": {"ask"}, "confirm": {"ask"},
	"nfc": {"tag"}, "card": {"show", "tag"}, "sticker": {"command"},
	"turn": {"move", "yaw"}, "look": {"move", "yaw", "pitch"}, "servo": {"move"}, "neck": {"move"},
	"text": {"show", "say"}, "display": {"show", "screen"}, "screen": {"show"}, "clear": {"show"},
	"emotion": {"face"}, "smile": {"face", "happy"}, "mood": {"face", "emotion"},
	"listen": {"events"}, "wait": {"events"}, "event": {"events"}, "input": {"events"},
	"battery": {"state", "battery_pct"}, "sensor": {"state", "readings"}, "sensors": {"state", "readings"}, "reading": {"readings", "state"},
	"agent": {"mcp"}, "ai": {"mcp", "agent"}, "claude": {"mcp", "agent"},
	"token": {"key"}, "password": {"key"},
	"ir": {"remote", "infrared"}, "infrared": {"remote"}, "tv": {"remote"},
	"usb": {"plugged", "unplugged"}, "power": {"button", "plugged"}, "charging": {"plugged", "state"},
	"shake": {"shaken"}, "pick": {"picked_up"}, "lift": {"picked_up"},
	"raw": {"command"}, "protocol": {"command"},
}

// searchIndex lists every action, argument, event, example, page and section of the site.
func searchIndex(d siteData, pages map[string][]byte) map[string]any {
	var out []searchEntry
	have := map[string]bool{}
	add := func(e searchEntry) {
		if !have[e.URL] {
			have[e.URL] = true
			out = append(out, e)
		}
	}
	for _, p := range d.Parts {
		for _, a := range p.Actions {
			add(searchEntry{Name: a.Name, Kind: "Actions", Text: a.Help, URL: "/docs/api#" + a.Name, Parts: []string{p.Name}})
		}
	}
	for _, p := range d.Parts {
		for _, a := range p.Actions {
			for _, prm := range a.Params {
				text := prm.Help
				if len(prm.Enum) > 0 {
					text += ": " + strings.Join(prm.Enum, ", ")
				}
				add(searchEntry{Name: prm.Name, Kind: "Arguments", In: a.Name, Text: text, URL: "/docs/api#" + a.Name + "-" + prm.Name, Parts: []string{p.Name}})
			}
		}
	}
	for _, g := range d.Events {
		for _, e := range g.Events {
			add(searchEntry{Name: e.Name, Kind: "Events", In: g.Name, Text: e.Means, URL: "/docs/api#ev-" + e.Name, Parts: []string{g.Name}})
		}
	}
	for _, ex := range d.Examples {
		var parts []string
		for _, p := range ex.Parts {
			parts = append(parts, p.Name)
		}
		add(searchEntry{Name: ex.Title, Kind: "Examples", Text: ex.What, URL: "/docs/examples#ex-" + ex.ID, Parts: parts})
	}
	for _, p := range sitePages {
		pageEntries(p, string(pages[p.Path]), add)
	}
	return map[string]any{"kinds": searchKinds, "entries": out, "synonyms": searchSynonyms}
}

var (
	headingRe     = regexp.MustCompile(`(?s)<h([12])(?: id="([^"]*)")?>(.*?)</h[12]>`)
	nextHeadRe    = regexp.MustCompile(`<h[1-6][ >]`)
	noscriptRe    = regexp.MustCompile(`(?s)<noscript>.*?</noscript>`)
	tagRe         = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceBeforeRe = regexp.MustCompile(` ([,.;:!?)])`)
)

// pageEntries are a page's title and its sections (h2 with an id), each with the text under it.
func pageEntries(p sitePage, page string, add func(searchEntry)) {
	start, end := strings.Index(page, `<main id="main">`), strings.Index(page, "</main>")
	if start < 0 || end < start {
		return
	}
	main := noscriptRe.ReplaceAllString(page[start:end], "")
	for _, m := range headingRe.FindAllStringSubmatchIndex(main, -1) {
		level, id := main[m[2]:m[3]], ""
		if m[4] >= 0 {
			id = main[m[4]:m[5]]
		}
		e := searchEntry{Name: plainText(main[m[6]:m[7]]), Kind: "Pages", URL: p.Path}
		switch {
		case level == "1":
			e.Name = p.Title
		case id == "":
			continue
		default:
			e.In, e.URL = p.Title, p.Path+"#"+id
		}
		rest := main[m[1]:]
		if i := nextHeadRe.FindStringIndex(rest); i != nil {
			rest = rest[:i[0]]
		}
		e.Text = shorten(plainText(rest), 240)
		add(e)
	}
}

// plainText is HTML as text: tags gone, spaces collapsed, none left before punctuation.
func plainText(h string) string {
	s := strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(h, " "))), " ")
	return spaceBeforeRe.ReplaceAllString(strings.ReplaceAll(s, "( ", "("), "$1")
}

// shorten cuts s to about n bytes at a space.
func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if i := strings.LastIndexByte(s[:n], ' '); i > 0 {
		return s[:i] + " …"
	}
	return s[:n]
}

/* --------------------------------- serving -------------------------------- */

// siteNow is the rendered site; from disk, rendered now, with Config.UIDir.
func (a *App) siteNow() (*site, error) {
	if a.cfg.UIDir != "" {
		return buildSite(os.DirFS(a.cfg.UIDir))
	}
	return a.site, a.siteErr
}

// handlePage serves one of the site's pages.
func (a *App) handlePage(w http.ResponseWriter, r *http.Request) {
	s, err := a.siteNow()
	if err != nil {
		http.Error(w, "the site: "+err.Error(), http.StatusInternalServerError)
		return
	}
	b, ok := s.pages[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}

// handleSearch serves the search's index: every action, argument, event, example and section.
func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	s, err := a.siteNow()
	if err != nil {
		http.Error(w, "the site: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(s.search)
}

// sitePictures are the pictures under site/img/, by extension.
var sitePictures = map[string]string{".png": "image/png", ".jpg": "image/jpeg"}

// handleSitePicture serves a picture of the pages (site/img/).
func (a *App) handleSitePicture(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	kind, ok := sitePictures[path.Ext(name)]
	s, err := a.siteNow()
	if !ok || err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(s.files, "img/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sum := sha256.Sum256(b)
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:8])+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

// handleSiteFile serves the site's scripts.
func (a *App) handleSiteFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if path.Ext(name) != ".js" {
		http.NotFound(w, r)
		return
	}
	s, err := a.siteNow()
	if err != nil {
		http.Error(w, "the site: "+err.Error(), http.StatusInternalServerError)
		return
	}
	b, err := fs.ReadFile(s.files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}
