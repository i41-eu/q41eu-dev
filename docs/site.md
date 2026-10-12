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

The header on every page: the page links, the search box, the account (Sign in or out through
the manager, a link to the manager), the language chips. The robot's QR code (`/pair?code=…`) pairs the browser
and goes to `/robots`. With `-manager-sign-in`, every page first tries the silent sign-in.

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
  (`gesture.Names`, grouped by the sensor in `site.go`), and the examples' manifest
  [examples/examples.json](../examples/examples.json) (per example its title, what it shows, the
  parts it uses, and per language its file and the command that runs it). Which action belongs to
  which part of the robot is `apiParts` in `site.go`.
- **Adding:** a page is a template and a line in `sitePages`; a new action needs its line in
  `apiParts` (a test fails until it has one); a new example is its files and its entry in
  `examples.json`.

## Tests

- `TestSite`: every page renders; every example's file is on the examples page in each language;
  every action, argument and event is on the API page, each action in exactly one part; every
  internal link answers and its anchor exists; no id twice on a page; the pages from disk equal
  the built-in ones.
- `TestSnippets`: each action's example passes the API's own argument check, and so does each
  tool call in the MCP examples.
- `TestSearchIndex`: the index has every action, argument, event, example, page and section,
  each once and pointing at an anchor that exists; every synonym leads to a word the site has.
- `TestReadmeTable`: the readme's API table lists the actions, in order.
- `TestPagesInBrowser`, in a headless Chrome or Chromium when one is installed (`$CHROME`, or on
  `PATH`; skipped with `-short`): the language selector (all on by default; off by a click and by
  the keyboard; remembered, on another page too; the last one off turns all on; `?lang=` and
  Keep; tabs; storage blocked), the robot's card (paired by the QR code's link, Try it, a new key,
  an event, Run), the search (`/` to the box, an action found by its name and an example by what
  it does, the arrow keys, Enter, Escape, a synonym, nothing found), and no page, nor the search's
  results, wider than a 390 px phone.
