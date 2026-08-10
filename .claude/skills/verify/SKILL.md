---
name: verify
description: Build GoLC, run a real scan against a DevOps platform, and drive the ResultsAll dashboard to observe changes at their surface. Use when verifying changes to golc.go, golc-launcher.go, ResultsAll.go, or pkg/utils report generation.
---

# Verifying GoLC

Two surfaces: the **CLI scanner** (writes `Results/`) and the **ResultsAll dashboard**
(HTTP + HTML, reads `Results/`). Most report changes need both — scan once, then drive
the dashboard against the output.

## Build

Three binaries from one module, selected by build tag:

```bash
go build -tags=launcher   -o "$WS/golc-launcher" golc-launcher.go golc.go   # scanner + config UI
go build -tags=resultsall -o "$WS/ResultsAll" .                  # dashboard
```

The scanner has no CLI flags — the analysis entrypoint is a hidden subcommand:

```bash
./golc-launcher --internal-run Github     # platform key from config.json: Github, GithubEnterprise,
                                 # Gitlab, Azure, BitBucket, BitBucketSRV, File
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

The UI applies defaults a hand-written config does not. `golc-launcher.go` ships three exclusion
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
directly, copy those two lists from `golc-launcher.go` first and say so in the write-up.

## The optional OSS test bed

A local toolset may exist outside the repo that mirrors ~30 real open-source projects onto
six platforms — Azure DevOps cloud and Server, Bitbucket Cloud, GitHub Enterprise, and
GitLab cloud and self-managed (ask the user for the location; it is deliberately not
version-controlled because it embeds their platform identifiers). It replaced a synthetic
corpus, because generated code does not parse and SonarQube therefore reported 0 ncloc for
whole languages — noise that reads as catastrophic GoLC defects.

**Azure DevOps Server is populated but cannot be scanned: GoLC has no connector for it**, so
there is no `config.json` platform key to point at it. The mirrors are there for whenever
one is written; until then, verify Azure against the hosted service.

| Script | Question |
|---|---|
| `mirror.py` | Clone the sources and re-originate them into `build/golc-*` |
| `publish.py` | Push them to one platform |
| `sqscan.py` + `sqcompare.py` | Does GoLC predict SonarQube's `ncloc`? |
| `baseline.py` | Did any count move since last time? |

**There is no oracle.** Nobody knows the true line count of real code, so nothing mirrors
GoLC's logic in Python and nothing has to be kept in step with a scanner change. Use
`baseline.py --save` before a change and `--check` after; it reports only what moved.

Three things worth knowing:

- **GoLC's default exclusions and a stock SonarQube are far apart.** Measured over 30 real
  repos: GoLC with its UI defaults reported 1.47M lines against SonarQube's 3.63M — a 2.5x
  under-count, entirely from the test/vendor/build folder presets. With exclusions off the
  two agree to 3.8%. Neither number is wrong; they answer different questions. Say which
  configuration produced any figure you report.
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
  `golc.go` and `golc-launcher.go` from coverage, so code reachable only from those files
  contributes nothing to the new-code coverage gate. Test report logic in `pkg/utils`.
- The `sonar` CLI and the SonarQube MCP server authenticate separately; the CLI may
  report "Project not found" while MCP works (different org).
