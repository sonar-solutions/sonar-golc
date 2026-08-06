---
name: verify
description: Build GoLC, run a real scan against a DevOps platform, and drive the ResultsAll dashboard to observe changes at their surface. Use when verifying changes to golc.go, webui.go, ResultsAll.go, or pkg/utils report generation.
---

# Verifying GoLC

Two surfaces: the **CLI scanner** (writes `Results/`) and the **ResultsAll dashboard**
(HTTP + HTML, reads `Results/`). Most report changes need both — scan once, then drive
the dashboard against the output.

## Build

Three binaries from one module, selected by build tag:

```bash
go build -tags=webui      -o "$WS/golc"       webui.go golc.go   # scanner + config UI
go build -tags=resultsall -o "$WS/ResultsAll" .                  # dashboard
```

The scanner has no CLI flags — the analysis entrypoint is a hidden subcommand:

```bash
./golc --internal-run Github     # platform key from config.json: Github, Gitlab, Azure, BitBucket, BitBucketSRV, File
```

Both binaries `chdir` to their own directory, so `Results/` and `config.json` must sit
next to the binary. Copy the binaries into an isolated workspace and work there — never
scan into the repo checkout.

## Run a real scan

`config.json` next to the binary, mirroring `config_sample.json` with one platform
filled in. Credentials for existing test orgs live outside the repo (ask the user; e.g.
`~/golc-env/*.env` exporting `GOLC_<PLATFORM>_TOKEN` / `_ORG` / `_USERNAME`).

**Keep scans small and fast.** `Repos` is a comma-separated include-list — pick the
smallest repos so cloning takes seconds, not minutes:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://api.github.com/orgs/$ORG/repos?per_page=100&type=all&sort=size&direction=asc" \
  | python3 -c "import json,sys; d=json.load(sys.stdin); \
      print(', '.join(r['name'] for r in sorted([x for x in d if not x['archived'] and x['size']>0], \
      key=lambda r:r['size'])[:35]))"
```

35 small repos ≈ 35 seconds and exercises the >30 truncation in the global report's
top-repositories table. `chmod 600 config.json` — it holds a live token. Delete the
workspace afterwards.

## Configure through the web UI, not a hand-written config.json

The UI applies defaults a hand-written config does not. `webui.go` ships three exclusion
presets **on by default** (test / vendor / build folder keywords) plus
`DEFAULT_FILE_PATTERNS` (`*_test.*`, `*.spec.*`, `*.min.*`, …). A `config.json` copied from
`config_sample.json` has these empty, so it measures GoLC **with its defaults switched
off** — and the UI is the channel customers actually use.

This caused a real false finding: a SonarQube comparison concluded "GoLC over-counts
minified files and test code" and recommended exclusions that already existed and were on.
The bug was in the harness.

Drive the same path the browser does — `POST /api/run` with `{"Platform":…,"Config":…}`,
including `FolderKeywords` and `FileNamePatterns` as the page would send them — then poll
`/api/status` until `running` is false. If a scripted run must write `config.json`
directly, copy those two lists from `webui.go` first and say so in the write-up.

## The optional test corpus

A generator for a synthetic multi-repo corpus may exist outside the repo (ask the user for
the location; it is deliberately not version-controlled because it embeds their platform
identifiers). It answers three different questions — do not confuse them:

| Script | Question | Use when |
|---|---|---|
| `verify.py` | Did GoLC count what was generated? | Changing the scanner, the language map, or exclusions |
| `sqscan.py` + `sqcompare.py` | Does GoLC predict SonarQube's `ncloc`? | Changing anything that affects the estimate customers act on |
| the feature probes | Does the tool behave on awkward repos? | Branch selection, exclusion decoys, archived/empty repos, Top-30 truncation |

**Its oracle mirrors GoLC's logic in Python, so a GoLC counting change must be mirrored
there or `verify.py` reports false failures.** Currently mirrored: the scanner's line
classification, `NonCodeLines`, `looksMinified`, and `RefineLanguage`. After touching
`assets/languages.go`, regenerate the corpus's language snapshot (`extract_languages.py`)
— a stale snapshot silently marks new languages as uncounted.

Two traps:

- **`verify.py` needs exclusions OFF.** The oracle models the scanner and analyzer but not
  config-driven exclusions, so scan with empty `FolderKeywords`/`FileNamePatterns`. That is
  the one place the web-UI-defaults rule above does not apply. Use the UI defaults for the
  SonarQube comparison and for anything customer-representative.
- **`sonar-scanner` writes a 40 MB+ `.scannerwork/` into the directory it scans**, mutating
  the fixtures. Force `sonar.working.directory` outside the corpus and check
  `find <build> -name .scannerwork` is empty afterwards.

For SonarQube parity, three analyzers count nothing under a stock configuration:
`sonar.yaml.activate` and `sonar.json.activate` default to **false**, and
`sonar.cobol.file.suffixes` defaults to **empty**. C# and VB.NET need SonarScanner for
.NET, which builds the project, so they can only be compared against source that compiles.

## Drive the dashboard

```bash
GOLC_RESULTS_PORT=8087 ./ResultsAll > results.log 2>&1 &   # never the default 8090
```

Port 8090 is frequently taken (Docker binds it here), and `handleServerStartup` then
prompts interactively and fails in a non-TTY. Always set an explicit free port — this
also makes `TestServerFunctions/handleServerStartup_function` pass locally.

Endpoints worth driving:

| Surface | Call |
|---|---|
| Page | `curl -s http://localhost:PORT/` |
| Repo table JSON | `/api/repositories`, `/api/deselected`, `/api/global-info`, `/api/scan-summary` |
| Reports (generated on demand) | `/reports/global-report.pdf`, `-customized.pdf`, `/reports/repository-summary.{pdf,csv}` |
| Everything zipped | `/download` |
| Change selection | `POST /api/deselected` with `{"Keys":["<org>__<repo>__<branch>"]}`; `{"Keys":[]}` resets |

Deselection keys are the result-file stem: `<org-or-projectkey>__<repo>__<branch>`, with
`/` in any component replaced by `_`.

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
body = urllib.request.urlopen("http://localhost:8086/").read().decode()
body = body.replace('src="/dist/', 'src="http://localhost:8086/dist/')
body = body.replace('href="/dist/', 'href="http://localhost:8086/dist/')
body = body.replace('</body>', PROBE + '</body>')   # PROBE dispatches clicks, writes results into <pre id="probe">
```

Then `chrome --headless=new --virtual-time-budget=8000 --dump-dom http://localhost:PROXY/`
and read the `<pre>` back. Dispatch clicks with
`el.dispatchEvent(new MouseEvent('click',{bubbles:true}))`.

## Reading PDFs

Assert on content, not just file size. Streams are zlib-deflated, and PDF **escapes
parentheses** — a naive `\((.*?)\)` stops at `\)` and silently truncates labels like
`Total LOC (filtered)`:

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

- A stale `ResultsAll` keeps the port and silently serves the **old binary** — a rebuild
  appears to have no effect. `pkill -f ResultsAll` between runs and confirm the port is
  free.
- Reports are generated lazily on first request and cached against a stamp covering the
  selection *and* the scan identity. To prove regeneration, delete the artifact and
  request it.
- `sonar.coverage.exclusions` in `sonar-project.properties` excludes `ResultsAll.go`,
  `golc.go` and `webui.go` from coverage, so code reachable only from those files
  contributes nothing to the new-code coverage gate. Test report logic in `pkg/utils`.
- The `sonar` CLI and the SonarQube MCP server authenticate separately; the CLI may
  report "Project not found" while MCP works (different org).
