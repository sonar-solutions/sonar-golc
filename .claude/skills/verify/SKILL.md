---
name: verify
description: Build GoLC, drive a real scan through the web UI the way a customer does, and drive the ResultsAll dashboard to observe changes at their surface. Use when verifying changes to golc.go, golc-launcher.go, ResultsAll.go, or pkg/utils report generation.
---

# Verifying GoLC

Two surfaces: the **launcher** (`golc-launcher`, config UI + scanner, writes `Results/`) and
the **ResultsAll dashboard** (HTTP + HTML, reads `Results/`). Most report changes need both
— scan once through the UI, then drive the dashboard against the output.

## Build

Three binaries from one module, selected by build tag — same commands as
`create_release_sample.sh`:

```bash
go build -tags=launcher   -o "$WS/golc-launcher" golc-launcher.go golc.go   # config UI + scanner
go build -tags=resultsall -o "$WS/ResultsAll"    ResultsAll.go              # dashboard
```

`golc.go` carries `//go:build launcher || engine`; the `engine` tag exists so the root
package's scanner tests can compile without the UI — `go test -tags=engine .` for
`golc*_test.go`, `go test -tags=resultsall .` for `ResultsAll_test.go` and
`deselection_test.go`.

Both binaries `chdir` to their own directory, so `Results/`, `Logs/` and `config.json` sit
next to the binary. Copy the binaries into an isolated workspace and work there — never
scan into the repo checkout. `./golc-launcher --version` reports the stamped release.

## Run a real scan — through the web UI, not a hand-written config.json

**This is the only route worth verifying**, because it is the only one customers use, and
because a hand-written `config.json` silently measures a *different product*.
`golc-launcher.go` ships three folder-exclusion presets **on by default** (test / vendor /
build keywords) plus `DEFAULT_FILE_PATTERNS`. A config copied from `config_sample.json`
leaves `FolderKeywords` and `FileNamePatterns` empty, so it measures GoLC with its defaults
switched off.

That cost a real false finding once: a SonarQube comparison concluded "GoLC over-counts
minified files and test code" and recommended exclusions that already existed and were on.
The bug was in the harness.

Start the launcher and read back the URL it prints — `GOLC_PORT` (legacy
`GOLC_WEBUI_PORT`, default 8091) is a *preference*, not a promise: `findFreePort` falls back
to any free port when it is taken. It also opens a browser tab at startup, which is
harmless but surprising in a scripted run.

```bash
cd "$WS" && GOLC_PORT=8199 ./golc-launcher > launcher.log 2>&1 &
until grep -q 'started on' launcher.log; do sleep 0.3; done
PORT=$(sed -n 's#.*localhost:\([0-9]*\).*#\1#p' launcher.log)
```

Then `POST /api/run` with `{"Platform":…,"Config":…}` — exactly what the page's
`gatherConfig()` sends. Send the two exclusion lists as the browser would, or you are back
to measuring defaults-off:

```jsonc
{"Platform":"Github","Config":{
  "Users":"…","AccessToken":"…","Organization":"…",
  "Repos":"repo-a, repo-b","Project":"","Branch":"","DefaultBranch":true,
  "Multithreading":true,"Workers":10,"NumberWorkerRepos":10,"WorkDir":"","CloneTimeout":15,
  "FileExclusion":"","ExtExclusion":[],"ExcludePaths":[],"Org":true,
  "ExcludeTests":true,"ExcludeVendor":true,"ExcludeBuild":true,
  "FolderKeywords":["test","tests","spec","specs","e2e","testdata","fixtures","mock",
    "mocks","integration","doc","docs","vendor","node_modules","bower_components",
    "third_party","external","dist","build","out","target","bin","coverage"],
  "FileNamePatterns":["*_test.*","test_*.*","*.test.*","*.spec.*","*_spec.*","*Test.*",
    "*Tests.*","*.min.*"],
  "ResultByFile":true,"ResultAll":true}}
```

Copy both lists from `PRESET_*_KEYWORDS` / `DEFAULT_FILE_PATTERNS` in `golc-launcher.go`
rather than from here, and say in any write-up which configuration produced a figure.
`ExcludeTests` / `ExcludeVendor` / `ExcludeBuild` are only persisted checkbox state — the
engine reads `FolderKeywords`, `FileNamePatterns`, `ExtExclusion` and `ExcludePaths`.

Then poll until it finishes:

```bash
until curl -s localhost:$PORT/api/status | grep -q '"running":false'; do sleep 3; done
```

`/api/status` returns `{running, phase, current, total, pct, error}`, with `phase` walking
`idle → identifying → analyzing → reporting → complete` (or `error`). `/api/events` is the
SSE stream the page consumes and replays its buffer to a late subscriber, so it is the
better surface when the *progress reporting itself* is what changed.

What the route does that a script would forget: `/api/run` merges your `Config` over
`platformDefaults` and **writes `config.json`** (so inspect that file when a run
misbehaves — and `chmod 600` it, it holds a live token), then deletes `Results/` before
spawning the scanner. Other answers: 400 unknown platform, 409 already running. `POST
/api/stop` kills the child.

`--internal-run <PlatformKey>` is how the launcher spawns its own analysis subprocess. Reach
for it directly only to reproduce an engine bug without HTTP in the way; it reads the
`config.json` the UI last wrote (or `$GOLC_CONFIG_FILE`).

### Platforms

| Key | Notes |
|---|---|
| `Github`, `GithubEnterprise` | GHE also needs `Url`; `Baseapi` is derived from it |
| `Gitlab` | `Organization` is a comma-separated list of group slugs, blank to auto-discover |
| `BitBucket`, `BitBucketSRV` | Cloud takes `Workspace`; DC takes `Url` + `Protocol` |
| `Azure`, `AzureServer` | both accept `Project` and `Repos` |
| `File` | scans `Directory` locally — no credentials, the fastest way to check report plumbing |

Dispatch inside `golc.go` is on the config's `DevOps` field, not the platform key, so
`AzureServer` (`DevOps: "azure"`) runs the same connector as the hosted service. Its
defaults are derived from `Azure`'s at init minus `Url`, so **`Url` must be supplied** and
the two cannot drift apart.

Two things are specific to Azure DevOps Server: `Organization` is the *collection* (the path
segment before the project, usually `DefaultCollection`), and a non-empty `Users` switches
the connector to `user:token` basic auth and turns on the NTLM negotiator for
Windows-authenticated servers. Leave `Users` empty when authenticating with a PAT. NTLM
authenticates a connection rather than a request, so each request gets its own connection
pool — if you touch `pkg/devops/getazure/ntlm.go`, verify under concurrent workers, since
the failure it fixes only appears there (a shared pool lost 2 of 30 repos).

Credentials for existing test orgs live outside the repo — ask the user; e.g.
`~/golc-env/*.env`, which must be **sourced**, not parsed: some tokens contain characters
the shell expands, so a regex yields a different string from the one git sees.

**Keep scans small and fast.** `Repos` is a comma-separated include-list — pick the smallest
repos so cloning takes seconds:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://api.github.com/orgs/$ORG/repos?per_page=100&type=all&sort=size&direction=asc" \
  | python3 -c "import json,sys; d=json.load(sys.stdin); \
      print(', '.join(r['name'] for r in sorted([x for x in d if not x['archived'] and x['size']>0], \
      key=lambda r:r['size'])[:35]))"
```

35 small repos ≈ 35 seconds and exercises the >30 truncation in the global report's
top-repositories table. Delete the workspace afterwards — it holds a token in
`config.json`.

## The optional OSS test bed

A local toolset may exist outside the repo that mirrors ~30 real open-source projects onto
six platforms — Azure DevOps cloud and Server, Bitbucket Cloud, GitHub Enterprise, and
GitLab cloud and self-managed (ask the user for the location; it is deliberately not
version-controlled because it embeds their platform identifiers). It replaced a synthetic
corpus, because generated code does not parse and SonarQube therefore reported 0 ncloc for
whole languages — noise that reads as catastrophic GoLC defects.

| Script | Question |
|---|---|
| `mirror.py` | Clone the sources and re-originate them into `build/golc-*` |
| `publish.py` | Push them to one platform |
| `platforms.py` | Per-platform adapters, including Azure DevOps Server |
| `sqscan.py` + `sqcompare.py` | Does GoLC predict SonarQube's `ncloc`? |
| `baseline.py` | Did any count move since last time? |
| `teardown.py` | Remove the mirrors — the `golc-` name prefix is its only safety filter |

Its README may still say Azure DevOps Server cannot be scanned because GoLC has no
connector. That is out of date — `AzureServer` is a supported platform key.

**There is no oracle.** Nobody knows the true line count of real code, so nothing mirrors
GoLC's logic in Python and nothing has to be kept in step with a scanner change. Use
`baseline.py --save` before a change and `--check` after; it reports only what moved.

Three things worth knowing:

- **GoLC's default exclusions and a stock SonarQube are far apart.** Measured over 30 real
  repos: GoLC with its UI defaults reported 1.47M lines against SonarQube's 3.63M — a 2.5x
  under-count, entirely from the test/vendor/build folder presets. With exclusions off the
  two agree to 3.8%. Neither number is wrong; they answer different questions.
- **On content SonarQube can parse, GoLC is accurate.** Objective-C, Swift and Terraform
  matched exactly; ABAP, C, C++, Dart, Java, Ruby, Rust, HTML and XML within 1%.
- **`sonar-scanner` writes a 40 MB+ `.scannerwork/` into the directory it scans.** Force
  `sonar.working.directory` elsewhere and check `find <build> -name .scannerwork` is empty.

Two SonarQube-side gotchas when comparing: `sonar.yaml.activate` and `sonar.json.activate`
default to **false** and `sonar.cobol.file.suffixes` defaults to **empty**, so those
analyzers count nothing until switched on (`sqscan.py --activate-optin`). C# and VB.NET need
SonarScanner for .NET, which builds the project. A mirrored project may also ship its own
`sonar-project.properties`, which the scanner will read and which can gut the scan —
`sqscan.py` neutralises it.

## Drive the dashboard

Ask the launcher for it, the way the "open results" button does:

```bash
curl -s -X POST localhost:$PORT/api/open-results   # → {"url":"http://localhost:8090"}
```

This route is worth preferring: it **kills any previous ResultsAll** before starting a new
one (the dashboard caches `Results/` at startup, so a survivor would serve the old scan),
picks a free port starting from `GOLC_RESULTS_PORT` (default 8090), and returns the URL it
actually bound. It needs the `ResultsAll` binary beside `golc-launcher` or in the cwd —
otherwise it answers with `"note":"ResultsAll binary not found"` and a URL that serves
nothing.

By hand, always set an explicit free port:

```bash
GOLC_RESULTS_PORT=8087 ./ResultsAll > results.log 2>&1 &   # never the default 8090
```

8090 is frequently taken (Docker binds it here), and `handleServerStartup` then prompts
interactively — which exits 1 in a non-TTY. An explicit port also makes
`TestServerFunctions/handleServerStartup_function` pass locally.

| Surface | Call |
|---|---|
| Page | `curl -s http://localhost:PORT/` |
| Totals and tables | `/api/global-info`, `/api/repositories`, `/api/languages`, `/api/scan-summary` |
| What did not get counted | `/api/skipped-repositories`, `/api/deselected` |
| One repository's detail page | `/repository/<key>` |
| Reports (generated on demand) | `/reports/{global-report,repository-summary}.pdf`, `/reports/repository-summary.csv`, and a `-customized` variant of each |
| Everything zipped | `/download` |
| Change selection | `POST /api/deselected` with `{"Keys":["<org>__<repo>__<branch>"]}`; `{"Keys":[]}` resets |

Deselection keys are the result-file stem: `<org-or-projectkey>__<repo>__<branch>`, with `/`
in any component replaced by `_` — except the `File` platform, whose results carry no org or
branch, so the key is the bare repo name. `POST` answers with
`{DeselectedCount, CountedRepositories, TotalLinesOfCode, RawTotalLinesOfCode, Ignored}`,
and refuses to deselect everything with 422. While nothing is deselected the `-customized`
routes deliberately serve the full-scan report rather than 404, so **compare bytes, not
status codes**, when checking that a selection reached the PDF.

## Screenshots

No Playwright installed; headless Chrome works. The page is long, so render tall and
crop with `sips`:

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless=new --disable-gpu --hide-scrollbars --no-sandbox \
  --window-size=1500,5200 --screenshot=shot.png --virtual-time-budget=7000 \
  http://localhost:8087/
sips --cropOffset 1290 0 -c 300 1500 shot.png --out bar.png   # selection bar + banner
```

## Driving the page's JavaScript (sorting, checkboxes)

Headless Chrome can't click, and **saving the page and opening it over `file://` does not
work**: the inline script builds the Chart.js pie chart before it attaches the sort and
checkbox handlers, so with `/dist/` vendor scripts unreachable it throws early and no
handler is ever bound. The page looks fine but nothing responds — easy to misread as a
broken feature.

Tell them apart by checking whether the page's own JS ran: `#selectionSummary` is empty
and `#totalCodeLines` still shows the server-rendered `-` when it didn't.

Instead, proxy the real page and inject a probe, rewriting `/dist/` to absolute URLs on
the real server so vendor scripts load and the inline script runs normally:

```python
body = urllib.request.urlopen("http://localhost:8087/").read().decode()
body = body.replace('src="/dist/', 'src="http://localhost:8087/dist/')
body = body.replace('href="/dist/', 'href="http://localhost:8087/dist/')
body = body.replace('</body>', PROBE + '</body>')   # PROBE dispatches clicks, writes results into <pre id="probe">
```

Then `chrome --headless=new --virtual-time-budget=8000 --dump-dom http://localhost:PROXY/`
and read the `<pre>` back. Dispatch clicks with
`el.dispatchEvent(new MouseEvent('click',{bubbles:true}))`.

## Reading PDFs

Assert on content, not just file size. Streams are zlib-deflated, and PDF **escapes
parentheses** — a naive `\((.*?)\)` stops at `\)` and silently truncates labels like
`JSON (excl.)`:

```python
import re, zlib
raw = open(path,'rb').read(); out = ""
for m in re.finditer(rb'stream\r?\n(.*?)endstream', raw, re.S):
    try: out += zlib.decompress(m.group(1)).decode('latin-1')
    except Exception: pass
text = "".join(re.findall(r'\(((?:\\.|[^\\()])*)\)', out))
text = text.replace('\\(','(').replace('\\)',')')
```

## Gotchas

- A stale `ResultsAll` started by hand keeps the port and silently serves the **old
  binary** — a rebuild appears to have no effect. `pkill -f ResultsAll` between runs and
  confirm the port is free. Going through `/api/open-results` avoids this.
- Reports are generated lazily on first request and cached against a stamp covering the
  selection *and* the scan identity. To prove regeneration, delete the artifact and
  request it.
- `Results/config/` holds the scan's own account of itself — the platform inventory,
  `analysis_summary.json` (scanned / analyzed / archived / empty / excluded / skipped) and
  `analysis_skipped.json`. Read it before believing a repository "went missing".
- `sonar.coverage.exclusions` in `sonar-project.properties` excludes `ResultsAll.go`,
  `golc.go` and `golc-launcher.go` from coverage, so code reachable only from those files
  contributes nothing to the new-code coverage gate. Test report logic in `pkg/utils`.
- The `sonar` CLI and the SonarQube MCP server authenticate separately; the CLI may
  report "Project not found" while MCP works (different org).
