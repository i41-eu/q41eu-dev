package devapp

// The case studies (docs/site.md): apps built on the kit, as they are at a tag of their public
// repositories. Their code excerpts are in site/cases/<app>.txt, the lines as they are at the tag,
// each after a line "== <key> <path> <from>-<to>". The pages show them with a link to those lines;
// `go test ./internal/devapp -run TestCaseExcerpts -args -cases-from=<dir>`, with clones of the
// apps in <dir>, checks them against the tags.

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// caseApp is an app of the case studies: its public repository and the tag the page describes.
type caseApp struct {
	ID, Name, Repo, Tag string
}

var caseApps = []caseApp{
	{"focus", "Focus", "i41-eu/q41eu-focus", "v0.5.0"},
	{"pet", "Pet", "i41-eu/q41eu-pet", "v0.14.1"},
}

// URL is the app's repository at its tag; with a path, that file there.
func (c caseApp) URL(path ...string) string {
	if len(path) == 0 {
		return "https://github.com/" + c.Repo + "/tree/" + c.Tag
	}
	return "https://github.com/" + c.Repo + "/blob/" + c.Tag + "/" + strings.Join(path, "/")
}

// excerpt is a few lines of an app's code at its tag.
type excerpt struct {
	App      caseApp
	Key      string // "focus/hub"
	Path     string // in the app's repository
	From, To int    // the lines
	Code     string
}

// URL links to the excerpt's lines in the app's repository, at its tag.
func (e excerpt) URL() string { return fmt.Sprintf("%s#L%d-L%d", e.App.URL(e.Path), e.From, e.To) }

// loadExcerpts reads every app's excerpts (site/cases/<app>.txt), by key ("focus/hub").
func loadExcerpts(files fs.FS) (map[string]excerpt, error) {
	out := map[string]excerpt{}
	for _, app := range caseApps {
		b, err := fs.ReadFile(files, "cases/"+app.ID+".txt")
		if err != nil {
			return nil, err
		}
		var cur *excerpt
		var lines []string
		done := func() error {
			if cur == nil {
				return nil
			}
			if n := cur.To - cur.From + 1; len(lines) != n {
				return fmt.Errorf("cases/%s.txt: %s has %d lines, its range %d-%d wants %d", app.ID, cur.Key, len(lines), cur.From, cur.To, n)
			}
			cur.Code = strings.Join(lines, "\n")
			out[cur.Key] = *cur
			return nil
		}
		for i, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			head, isHead := strings.CutPrefix(line, "== ")
			if !isHead {
				if cur == nil {
					return nil, fmt.Errorf("cases/%s.txt:%d: code before the first \"== \" line", app.ID, i+1)
				}
				lines = append(lines, line)
				continue
			}
			if err := done(); err != nil {
				return nil, err
			}
			f := strings.Fields(head)
			from, to, ok := strings.Cut(f[len(f)-1], "-")
			a, errA := strconv.Atoi(from)
			z, errZ := strconv.Atoi(to)
			if len(f) != 3 || !ok || errA != nil || errZ != nil || a < 1 || z < a {
				return nil, fmt.Errorf("cases/%s.txt:%d: want \"== <key> <path> <from>-<to>\", not %q", app.ID, i+1, line)
			}
			key := app.ID + "/" + f[0]
			if _, dup := out[key]; dup {
				return nil, fmt.Errorf("cases/%s.txt:%d: %s twice", app.ID, i+1, key)
			}
			cur, lines = &excerpt{App: app, Key: key, Path: f[1], From: a, To: z}, nil
		}
		if err := done(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
