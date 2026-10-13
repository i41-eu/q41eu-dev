# Dev's site

Dev serves its own site: a few pages built into its binary, so a Dev at home has the same docs,
offline, as dev.q41.eu. What comes next for it is planned in
[wip/design-site.md](../wip/design-site.md).

## Pages

| Path | Page | What is on it |
|---|---|---|
| `/` | Home | what Dev is, the three steps (robot, key, first program), the hello example in the languages picked with **Run**, links to the rest; the robots paired with this browser, in brief |
| `/robots` | Your robots | a card per robot paired with this browser: **Try it** (a row of controls per action), **Keys** (new, list, delete), **Events** (live); without a robot, how to pair one (signed in through the manager, or by the robot's QR code) |
| `/docs/start` | Getting started | from a robot on the desk to a program driving it (as [getting-started.md](getting-started.md)), the first program's command in each language |
| `/docs/api` | The API by part | calling an action (the helper per language, the answers and errors), then a section per part of the robot: screen and face, head, LEDs, speaker, questions, what people do (the events, by the sensor that notices them), state and readings, any robot command; each action with its arguments and one call in each language |
| `/docs/examples` | Examples | every example in every language (the files themselves, with Copy and Download), the parts of the robot it uses, **Run** |
| `/docs/case-studies` | Case studies | Focus and Pet side by side (what each is, its size, what it uses of the kit and of the robot), an app compared with a program, the kit's packages and who uses them |
| `/docs/case-studies/focus`, `/docs/case-studies/pet` | Focus, Pet | one app each, as it is at a tag of its public repository: what the robot and the phone show (its pictures), how it is built (its parts and their lines), what it sends the robot and handles with the same through Dev's actions (or "an app only"), its key code (excerpts, each with a link to its lines at the tag), what it teaches |

The header on every page: the page links (a case study marks Case studies), the search box, the account (Sign in or out through
the manager, a link to the manager), the language chips. The robot's QR code (`/pair?code=…`) pairs the browser
and goes to `/robots`.

**Sign-in** (`-manager-sign-in`): the docs pages answer every request as they are, with no
redirect (a search engine, a link preview, `curl`); the header learns whether the browser is
signed in from the page's own fetch of its state (`/api/state`). Only what needs the person's
robots goes to the manager for the silent sign-in (at most once in ten minutes, back with
`?signin=no` when nobody is signed in there): **Your robots** on load (with `-sign-in-always`,
any browser; else one the manager's hint cookie names), **Home** only for a browser the hint
cookie names, and **Run** on a page with no robot yet (`/auth/login?silent=1`, back to its
example). **Sign in** in the header signs in for real.

## Languages

- **Chips in the header:** curl, Python, JavaScript, Go and MCP (what to ask an AI agent, and the
  tool call it makes). Several can be on; **all are on by default**; turning off the last one
  turns them all on again. They are checkboxes: Tab reaches them, Space switches one.
- **Remembered per browser** in `localStorage` (`dev.langs`, a JSON list), read and written inside
  try/catch: a browser without storage (blocked, a private window) gets every language every
  time, and the chips still work on the page.
- **A link can pick:** `?lang=python,go` (also `javascript`) shows those for that page only;
  **Keep this choice** makes it the browser's (as does changing a chip).
- **Before the first paint:** a small script in `<head>` puts classes on `<html>`
  (`lang-python lang-go`), and CSS made from the list of languages hides the code of the others:
  no flash, no jump.
- **Side by side** (the default): the languages picked in a grid, two or three across on a
  laptop (whole example files two across), one under the other on a phone. **Tabs** (the button
  next to the chips, remembered as `dev.layout`): one language at a time in each group; the tab
  picked follows across the page (`dev.tab`).
- **Without JavaScript** every language is shown; the chips, Copy and Run are hidden.

Each code block has its language, how to run it (examples), **Copy**, and **Download** (example
files). **Run** runs an example on the first robot paired with this browser, through the page's
own session, and shows what it prints: a JavaScript copy of the example's steps, in `site.js`.

## Search

- **A box in the header** of every page. `/` goes to it (unless you are typing somewhere else);
  results show under it as you type, grouped (actions, arguments, events, examples, pages), the
  best first; the arrow keys move through them, Enter opens the one picked (or the first), Escape
  closes them (a second Escape clears the box). It is a combobox with a listbox for screen
  readers, and a polite count of the results.
- **The index,** `/docs/search.json` (about 20 KB), is made at start from the same data as the
  pages: an entry per action, argument (with its choices: `tada` finds `chime`), event, example,
  page and section (an `h2` with an id, with the text under it). Each has its name, kind, a short
  text, where it belongs (an argument's action, an event's sensor, a section's page), the parts of
  the robot, and its page and anchor. The page fetches it once, the first time the box is used.
- **Matching,** in `site.js`, no library: every word of the query must match (small words like
  "the" and "with" are left out); a whole name counts most, then the start of a name or of a word
  in it, then a word of the text. A small table of synonyms (`searchSynonyms` in `site.go`) helps
  people who do not know our words: "touch" also looks for `tap`, `pat`, `pet`, `stroke`;
  "light" for `leds` and `near`; "photo" for `image`; "speak" for `say` and `sound`; "colour"
  and "color"; "agent" for MCP; a synonym counts a little less than the word itself.
- Like the rest, it works offline on a Dev at home, and without JavaScript the box is hidden.

## How it is made

- **Templates:** `internal/devapp/site/`: `layout.html` (the head, the header, the footer, and the
  shared parts: a group of code blocks, an example, an action), one template per page, `site.css`,
  `site.js` (every page) and `robots.js` (Your robots). `html/template` and `embed`: no build
  step, no other dependency. Rendered once at start; `-ui-dir internal/devapp/site` renders them
  from disk on every request (edit, reload).
- **One source:** the pages are made from the same data as the API: the actions
  (`internal/devapp/actions.go`: their help and arguments, and per action an `Example` call and a
  `Prompt` for an agent, from which the API page's call in every language is made), the events
  (`gesture.Names`, what people did, and Dev's own `connected`, in `devEvents`; grouped by the
  sensor in `site.go`), and the examples' manifest
  [examples/examples.json](../examples/examples.json) (per example its title, what it shows, the
  parts it uses, and per language its file and the command that runs it). Which action belongs to
  which part of the robot is `apiParts` in `site.go`.
- **Case studies:** the apps and the tags the pages describe are `caseApps` in `cases.go`; the code
  excerpts are `site/cases/<app>.txt`, the lines as they are at the tag, each after a line
  `== <key> <path> <from>-<to>` (a page shows one with `{{template "excerpt" (excerpt "pet/menu")}}`);
  their pictures are `site/img/` (served at `/site/img/`, with their sources and licences in
  `site/img/NOTICE`). A new tag of an app: its excerpts again from there, its numbers and tables
  checked, then `TestCaseExcerpts` with `-cases-from`.
- **Adding:** a page is a template and a line in `sitePages`; a new action needs its line in
  `apiParts`, help for it and each argument, and its row in the readme's API table naming every
  argument; a new event its line in `eventGroups` and its name in the readme's events (tests fail
  until they are there); a new example is its files and its entry in `examples.json`.

## Tests

- `TestSite`: every page renders; every example's file is on the examples page in each language;
  every action, argument and event is on the API page, each action in exactly one part; every
  internal link answers and its anchor exists; no id twice on a page; the pages from disk equal
  the built-in ones.
- `TestAPIDocs`: everything `GET /v1/actions` lists is documented. Each action: its section on
  the API page with its help, a call in every language, a row per argument with its type, help and
  choices; in the search (the action and each argument); in OpenAPI; as an MCP tool with every
  argument; in the readme's API table with every argument. Each event: on the API page with what
  it means, in the search, in the readme's The API, and (a person's) in the help of `events`.
- `TestSnippets`: each action's example passes the API's own argument check, and so does each
  tool call in the MCP examples.
- `TestSearchIndex`: the index has every action, argument, event, example, page and section,
  each once and pointing at an anchor that exists; every synonym leads to a word the site has.
- `TestReadmeTable`: the readme's API table lists the actions, in order.
- `TestCaseStudies`: every excerpt is shown once, on its app's page, with a link to its lines; every
  link into an app's repository is at its tag, never a branch; the pictures are served (with an
  ETag) and nothing else under `/site/img/`. An excerpt's line count must equal its range, or the
  site does not build. `TestCaseExcerpts`, only with `-args -cases-from=<dir>` (clones of
  q41eu-focus and q41eu-pet in `<dir>`), compares every excerpt with its lines at the tag.
- `TestPagesInBrowser`, in a headless Chrome or Chromium when one is installed (`$CHROME`, or on
  `PATH`; skipped with `-short`): the language selector (all on by default; off by a click and by
  the keyboard; remembered, on another page too; the last one off turns all on; `?lang=` and
  Keep; tabs; storage blocked), the robot's card (paired by the QR code's link, Try it, a new key,
  an event, Run), the case studies' code shown whatever the languages picked, the search (`/` to the box, an action found by its name and an example by what
  it does, the arrow keys, Enter, Escape, a synonym, nothing found), and no page, nor the search's
  results, wider than a 390 px phone.
