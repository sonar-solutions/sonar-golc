![Static Badge](https://img.shields.io/badge/Go-v1.25-blue:)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)
[![Lines of Code](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=ncloc)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)

# GoLC — Go Line Counter

![logo](imgs/Logob.png)

**GoLC** counts physical lines of source code across all programming languages supported by [SonarQube](https://www.sonarsource.com/knowledge/languages/) — without running a full Sonar analysis.

It connects to your DevOps platform, counts one branch per repository, and presents the results in an interactive web dashboard with PDF, JSON, and CSV exports.

**Supported platforms:** GitHub.com · GitHub Enterprise Server · GitLab Cloud · GitLab Self-Managed · Bitbucket Cloud · Bitbucket Data Center · Azure DevOps Services · Local files/directories

> Current version: **v2.1**

---

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration](#configuration)
  - [Optional Parameters](#optional-parameters)
  - [Which branch is counted](#which-branch-is-counted)
- [Reports](#reports)
  - [Languages per repository](#languages-per-repository)
  - [Excluding repositories from the totals](#excluding-repositories-from-the-totals)
- [Supported Languages](#supported-languages)
- [Execution Log](#execution-log)
- [Troubleshooting](#troubleshooting)

---

## Quick Start

Download the latest release from the [Releases page](https://github.com/sonar-solutions/sonar-golc/releases).

Each archive contains two binaries:

| Binary | Purpose |
|--------|---------|
| `webui` / `webui.exe` | Browser-based launcher — start here |
| `ResultsAll` / `ResultsAll.exe` | Results dashboard (launched automatically by `webui`) |

Run `webui` / `webui.exe`. The browser opens automatically to the GoLC UI (default: `http://localhost:8091`).

If it doesn't open, copy the URL printed in the terminal. Then:

1. **Choose your platform** — click the platform card.
2. **Enter credentials** — token, organization, and any other required fields. Previous settings are pre-filled automatically.
3. **Run Analysis** — click "Run Analysis" and watch the live progress bar.
4. **View Results** — click "View Results" when complete. The results dashboard opens automatically.

> **Ports are managed automatically.** If the default port is in use, GoLC picks the next free one. The actual URL is always printed on startup.

---

## Configuration

Credentials are entered in the browser and saved automatically — no config file needed for normal use.

### Required token permissions

| Platform | Required scopes |
|----------|----------------|
| GitHub / GitHub Enterprise | `repo` |
| GitLab | `read_repository`, `read_api` |
| Bitbucket Cloud | Repositories: Read · Projects: Read · Account: Read |
| Bitbucket Data Center | Repo read, pull |
| Azure DevOps | Code: Read · Project and Team: Read |

---

### Optional Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `DefaultBranch` | bool | `true` (default) = count each repository's default branch. `false` = count its most recently active branch instead. See [Which branch is counted](#which-branch-is-counted). |
| `Branch` | string | Count this branch name in every repository instead. Requires `DefaultBranch: false`. |
| `Period` | int | How far back to look when deciding which branch is most active, in months (e.g. `-1` = the last month). Only used when `DefaultBranch: false` and `Branch` is empty. Default `-1`; Bitbucket Data Center `-5`. |
| `Multithreading` | bool | Enable parallel analysis. Default: `true`. |
| `Workers` | int | Concurrent workers. Default: `10`. |
| `FolderKeywords` | array | Exclude folders whose name contains the keyword as a whole word at any depth. Word boundaries are delimiters `-`, `_`, `.` — so `"test"` matches `integration-test/` and `test_helpers/` but not `protest/` or `latest/`. |
| `FileNamePatterns` | array | Exclude files whose name matches a glob pattern (e.g. `["*_test.go", "*.min.js", "*.spec.ts"]`). The `*` wildcard is matched against the file name only, not the full path. |
| `ExtExclusion` | array | Exclude all files with these extensions, regardless of language (e.g. `[".css", ".html"]`). |
| `ExcludeTests` | bool | Shortcut that adds common test-directory keywords to `FolderKeywords`: `test`, `tests`, `spec`, `specs`, `e2e`, `testdata`, `fixtures`, `mocks`, `integration`. |
| `ExcludeVendor` | bool | Shortcut that adds common vendor-directory keywords to `FolderKeywords`: `vendor`, `node_modules`, `bower_components`, `third_party`, `external`. |
| `Project` | string | Limit to a specific project key (Bitbucket, Azure DevOps). |
| `Repos` | string | Limit to specific repositories (comma-separated). GitHub/GHE: repository name. Bitbucket: repository slug. Azure DevOps: repository name. Not applicable for GitLab — use the Group URL slug field instead. |
| `Org` | bool | `true` = analyze an organization account, `false` = analyze a personal account. GitHub and GitHub Enterprise only. Default: `true`. |
| `WorkDir` | string | Base directory where repositories are cloned before counting, then deleted. Leave blank to use the system temp directory (the default). Set this to a path on a disk with enough free space when `/tmp` is small or RAM-backed and large/many repos fail with `no space left on device`. The directory is created if missing and must be writable. Can also be set globally with the `GOLC_WORKDIR` environment variable; the per-platform `WorkDir` value takes precedence over the environment variable. |


---

### Which branch is counted

**GoLC counts exactly one branch per repository** — never several, and never all of them
added together. This keeps the total comparable to what SonarQube would report.

Which one depends on the **Analyze default branch only** switch in the UI:

| Setting | Branch counted | Use it when |
|---------|----------------|-------------|
| **On** (default) | the repository's **default** branch | Almost always. Fastest, and matches what SonarQube analyses. |
| **Off** | the **most recently active** branch | Your main line of work is not the default branch. |
| **Off** + *Specific branch name* | that **exact branch**, in every repository | You size a named branch such as `develop` or `release/2025`. |

"Most recently active" means the branch with the most commits in the last `Period`
months (`-1` by default, so the last month).

> **If no branch has commits in that window, GoLC falls back to the default branch.** For
> repositories that have been quiet, turning the switch off therefore changes nothing.
> Widen `Period` (for example `-12`) if you want a longer history taken into account.

---

## Reports

After each run, GoLC writes all reports to a `Results/` folder next to the binary. This is always the case regardless of how the binary was launched (double-click, terminal, or PATH). Click **View Results** in the browser to open the interactive dashboard, or navigate to the folder directly.

### File tree view

GoLC maps the complete file hierarchy of each scanned repository. In the Results dashboard, click any repository to explore an interactive tree — folders are collapsible, files show their code line count, and a search box filters the tree in real time.

```
my-org / my-service  (branch: main)
│
├── src/
│   ├── main/
│   │   ├── java/
│   │   │   ├── App.java                          1 204 lines
│   │   │   └── service/
│   │   │       ├── UserService.java                892 lines
│   │   │       └── OrderService.java               741 lines
│   │   └── resources/
│   │       └── application.properties               38 lines
│   └── test/
│       └── java/
│           └── AppTest.java                        310 lines
├── pom.xml                                          64 lines
└── Dockerfile                                       18 lines
```

The same data is available as exportable files in the `Results/` folder next to the binary:

| Output | Location |
|--------|----------|
| Interactive dashboard | Click **View Results** → select a repository |
| Per-repo file breakdown (JSON) | `Results/byfile-report/Result_<org>_<repo>_<branch>_byfile.json` |
| Per-repo file breakdown (CSV) | `Results/byfile-report/csv-report/Result_<org>_<repo>_<branch>_byfile.csv` |
| Per-repo file breakdown (PDF) | `Results/byfile-report/pdf-report/Result_<org>_<repo>_<branch>_byfile.pdf` |
| Cross-repo summary | `Results/byfile-report/repository_summary.{json,csv,pdf}` |
| Per-repo language breakdown | `Results/bylanguage-report/Result_<org>_<repo>_<branch>.json` |
| Organisation-wide totals | `Results/GlobalReport.{pdf,json}`, `Results/code_lines_by_language.json` |

> File names follow the pattern `Result_<organisation>_<repository>_<branch>` so results from multiple organisations or runs can coexist in the same folder.

### What each report contains

| Report | Contents |
|--------|----------|
| `GlobalReport.pdf / .json` | Organisation-wide totals: lines of code per language, largest repository, total repository and branch counts, and a **Top 30 Repositories** table (branch, main language, LOC, share of total) |
| `byfile-report/repository_summary.*` | Cross-repository file summary — lists every analysed repository with its total lines, blank lines, comments, and code lines, plus its largest languages |
| `byfile-report/*_byfile.*` | Per-repository file tree — one row per source file with individual line counts |
| `bylanguage-report/*.json` | Per-repository language breakdown — one row per detected language with line counts |

### Languages per repository

The **Lines of Code by Repository** table shows each repository's three largest
languages with their code lines (`Python 54.9K · C# 50.0K · YAML 32.0K`), sortable by
the primary language. The full breakdown for a repository is one click away on its
detail page.

The same information reaches the reports:

| Report | Languages shown |
|--------|-----------------|
| `repository_summary.csv` / `.json` | all three, in fixed columns so a spreadsheet can sort or pivot on them |
| `repository_summary.pdf` | the main language (the table has no room for three) |
| `GlobalReport.pdf` | the main language of each of the Top 30 repositories |

> JSON is excluded from these rankings, exactly as it is excluded from the code-line
> totals they sit beside. A repository whose by-language results are missing shows `—`.

### Excluding repositories from the totals

Some repositories should not count towards a sizing exercise — a repository that was
archived after the scan, a mirror, a vendored dependency dump. On the Results
dashboard, the **Lines of Code by Repository** table has a checkbox per repository.
Uncheck the ones to leave out and click **Apply selection** — the totals, language
breakdown, and chart update immediately.

This works on every platform (GitHub, GitHub Enterprise, GitLab, Bitbucket
Cloud/Data Center, Azure DevOps, and local directories), because it operates on the
analysis results rather than on any platform API. **No re-scan is needed** — the
repositories were already counted, and only the totals are recomputed.

#### Original and customized reports

Once a selection exists, the **Reports** menu offers two sets:

| | Covers | Files |
|---|---|---|
| **Full scan** | every analysed repository, whatever is selected | `Results/GlobalReport.pdf`, `Results/byfile-report/repository_summary.*` |
| **Current selection** | only the selected repositories | `Results/customized/…` (same layout) |

The original is never overwritten, so it is always available for comparison. Downloads
are named `..._full-scan.pdf` and `..._selection.pdf` so the two cannot be confused
once detached from the dashboard.

Reports are generated **when you click them**, not when you change the selection, so
applying a selection is instant and no PDF is ever served stale. The first click after
a change takes a moment while the report is built. (GoLC also rebuilds them in the
background after a change, so anything reading `Results/` directly — a script, a CI
job — finds current files too.)

Both PDFs and the CSV state what was excluded and what the unfiltered total was, and
the customized report's headline figures are explicitly labelled *(filtered)*, so a
filtered report can be handed to a customer without misleading them.

**It is always reversible.** The per-repository result files are never modified and
`GlobalReport.json` keeps the figures as scanned, so **Reset to full scan** restores
the original figures exactly. The selection is stored in
`Results/config/deselected_repos.json` and survives a dashboard restart. At least one
repository must remain selected.

> A new analysis run clears the selection: it rediscovers repositories from scratch,
> so a selection made against the previous run could silently drop repositories from
> the new totals. To exclude repositories *before* a scan instead, so they are never
> cloned, use the platform's `FileExclusion` file in `config.json`.

---

## Supported Languages

```
Language           | Extensions                               | Single Comments | Multi-line Comments
-------------------+------------------------------------------+-----------------+--------------------
Abap               | .abap, .ab4, .flow, .asprog              | *, "            |
ActionScript       | .as                                      | //              | /* */
Apex               | .cls, .trigger                           | //              | /* */
C                  | .c                                       | //              | /* */
C Header           | .h                                       | //              | /* */
C++                | .cpp, .cc                                | //              | /* */
C++ Header         | .hh, .hpp                                | //              | /* */
C#                 | .cs                                      | //              | /* */
COBOL              | .cbl, .ccp, .cob, .cobol, .cpy           | *               |
CSS                | .css                                     |                 | /* */
Dart               | .dart                                    | //              | /* */
Docker             | Dockerfile, dockerfile                   | #               |
Golang             | .go                                      | //              | /* */
Gosu               | .gs, .gsx, .gsp                          | //              | /* */
Groovy             | .groovy, .gvy, .gy, .gsh, Jenkinsfile    | //              | /* */
HTML               | .html, .htm, .cshtml, .vbhtml, ...       |                 | <!-- -->
Java               | .java, .jav                              | //              | /* */
JavaScript         | .js, .jsx                                | //              | /* */
JCL                | .jcl, .JCL                               | //*             |
JSON               | .json                                    |                 |
JSP                | .jsp, .jspf, .jspx                       |                 | <%-- --%>, <!-- -->
Kotlin             | .kt, .kts                                | //              | /* */
Objective-C        | .m, .mm                                  | //              | /* */
Oracle PL/SQL      | .pkb                                     | --              | /* */
PHP                | .php, .php3, .php4, .php5, .phtml, .inc  | //, #           | /* */
PL/I               | .pl1, .pli                               |                 | /* */
PowerShell         | .ps1, .psm1, .psd1                       | #               | <# #>
Python             | .py                                      | #               | """ """, ''' '''
RPG                | .rpg                                     | *               |
Ruby               | .rb                                      | #               | =begin =end
Rust               | .rs                                      | //              | /* */
Scala              | .scala                                   | //              | /* */
Scss               | .scss                                    | //              | /* */
Shell              | .sh, .bash, .zsh, .ksh                   | #               |
SQL                | .sql                                     | --              | /* */
Swift              | .swift                                   | //              | /* */
Terraform          | .tf                                      | #, //           | /* */
T-SQL              | .tsql                                    | --              | /* */
TypeScript         | .ts, .tsx                                | //              | /* */
VB6                | .bas, .frm, .cls                         | '               |
Visual Basic .NET  | .vb                                      | '               |
Vue                | .vue                                     |                 | <!-- -->
XML                | .xml, .XML                               |                 | <!-- -->
XHTML              | .xhtml                                   |                 | <!-- -->
YAML               | .yaml, .yml                              | #               |
```

---

## Execution Log

GoLC writes a detailed log to `Logs/Logs.log` next to the binary. The file is recreated on each run. Use it to troubleshoot authentication errors, rate limits, or unexpected results.

---

## Troubleshooting

### "Apple could not verify" message
When starting the app on MacOS you may get `Apple could not verify webui is free of malware that may harm your Mac or compromise your privacy."` message. To go around it, run:

```
xattr -cr /path/to/webui
xattr -cr /path/to/ResultsAll
```
