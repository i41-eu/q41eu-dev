package devapp

import (
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"testing"
)

var casesFrom = flag.String("cases-from", "", "a directory with clones of the case studies' apps (q41eu-focus, q41eu-pet): TestCaseExcerpts checks the excerpts against their tags")

// The case studies: every excerpt is shown once, on its app's page, with a link to its lines at the
// app's tag; the pages link to the apps at those tags, never to a branch; the pictures are served.
func TestCaseStudies(t *testing.T) {
	a := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PublicURL: "https://dev.example", Version: "test"})
	if a.siteErr != nil {
		t.Fatal(a.siteErr)
	}
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	pages := map[string]string{}
	for _, p := range sitePages {
		_, pages[p.Path] = get(t, ts.URL+p.Path)
	}
	shown := regexp.MustCompile(`data-excerpt="([^"]+)"`)
	seen := map[string]int{}
	for _, app := range caseApps {
		page := pages["/docs/case-studies/"+app.ID]
		if page == "" {
			t.Fatalf("no case study page for %s", app.ID)
		}
		for _, m := range shown.FindAllStringSubmatch(page, -1) {
			if !strings.HasPrefix(m[1], app.ID+"/") {
				t.Errorf("%s's page shows %s", app.ID, m[1])
			}
			seen[m[1]]++
		}
		// Links into the app's repository: at its tag only.
		for _, m := range regexp.MustCompile(`href="(https://github\.com/`+regexp.QuoteMeta(app.Repo)+`[^"]*)"`).FindAllStringSubmatch(page, -1) {
			if !strings.Contains(m[1], "/tree/"+app.Tag) && !strings.Contains(m[1], "/blob/"+app.Tag+"/") {
				t.Errorf("%s's page links to %s, not at %s", app.ID, m[1], app.Tag)
			}
		}
		if !strings.Contains(page, `href="`+app.URL()+`"`) {
			t.Errorf("%s's page does not link to its repository at %s", app.ID, app.Tag)
		}
		if !strings.Contains(page, `<a href="/docs/case-studies" aria-current="true">`) {
			t.Errorf("%s's page: the header does not mark Case studies", app.ID)
		}
	}
	for key, e := range a.site.excerpts {
		if seen[key] != 1 {
			t.Errorf("excerpt %s shown %d times, want once", key, seen[key])
		}
		if !strings.Contains(pages["/docs/case-studies/"+e.App.ID], `href="`+e.URL()+`"`) {
			t.Errorf("excerpt %s: no link to %s", key, e.URL())
		}
	}
	if len(a.site.excerpts) < 10 {
		t.Errorf("only %d excerpts", len(a.site.excerpts))
	}
	// Sub-pages have no link of their own in the header.
	nav := regexp.MustCompile(`(?s)<nav class="pages".*?</nav>`).FindString(pages["/"])
	if strings.Count(nav, `href="/docs/case-studies`) != 1 || !strings.Contains(nav, `<a href="/docs/case-studies">Case studies</a>`) {
		t.Errorf("the header links the case studies' index, and only it: %s", nav)
	}

	// The pictures: their type, and not again when the browser has them.
	for _, m := range regexp.MustCompile(`src="(/site/img/[^"]+)"`).FindAllStringSubmatch(pages["/docs/case-studies"]+pages["/docs/case-studies/pet"], -1) {
		res, err := http.Get(ts.URL + m[1])
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != sitePictures[path.Ext(m[1])] || res.Header.Get("ETag") == "" {
			t.Fatalf("%s: %d %s", m[1], res.StatusCode, res.Header.Get("Content-Type"))
		}
		req, _ := http.NewRequest(http.MethodGet, ts.URL+m[1], nil)
		req.Header.Set("If-None-Match", res.Header.Get("ETag"))
		if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusNotModified {
			t.Fatalf("%s again: %v %v", m[1], err, res.Status)
		}
	}
	for _, p := range []string{"/site/img/NOTICE", "/site/img/../site.css", "/site/img/nothing.png"} {
		if code, _ := get(t, ts.URL+p); code != http.StatusNotFound {
			t.Errorf("%s: %d", p, code)
		}
	}
}

// The excerpts are the apps' code at their tags (run with -cases-from=<dir with the apps' clones>).
func TestCaseExcerpts(t *testing.T) {
	if *casesFrom == "" {
		t.Skip("-cases-from not given")
	}
	a := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if a.siteErr != nil {
		t.Fatal(a.siteErr)
	}
	for key, e := range a.site.excerpts {
		dir := *casesFrom + "/" + path.Base(e.App.Repo)
		out, err := exec.Command("git", "-C", dir, "show", e.App.Tag+":"+e.Path).Output()
		if err != nil {
			t.Fatalf("%s: git show %s:%s in %s: %v", key, e.App.Tag, e.Path, dir, err)
		}
		lines := strings.Split(string(out), "\n")
		if e.To > len(lines) {
			t.Errorf("%s: %s has %d lines, the excerpt ends at %d", key, e.Path, len(lines), e.To)
			continue
		}
		if want := strings.Join(lines[e.From-1:e.To], "\n"); e.Code != want {
			t.Errorf("%s: not lines %d-%d of %s at %s:\n%s\nwant\n%s", key, e.From, e.To, e.Path, e.App.Tag, e.Code, want)
		}
	}
}
