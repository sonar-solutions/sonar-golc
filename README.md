![Static Badge](https://img.shields.io/badge/Go-v1.25-blue:)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=alert_status&token=8ec4d9fa8caaec10baf81b14f9411528c569312d)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)[![Lines of Code](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=ncloc&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Reliability Issues](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_reliability_issues&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Maintainability Rating](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_maintainability_rating&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)

# GoLC — Go Line Counter

![logo](imgs/Logob.png)

**GoLC** counts physical lines of source code across all programming languages supported by [SonarQube](https://www.sonarsource.com/knowledge/languages/) — without running a full Sonar analysis.

It connects to your DevOps platform, identifies the largest branch of each repository, counts lines of code per language, and presents the results in an interactive web dashboard with PDF, JSON, and CSV exports.

**Supported platforms:** GitHub.com · GitHub Enterprise Server · GitLab Cloud · GitLab Self-Managed · Bitbucket Cloud · Bitbucket Data Center · Azure DevOps Services · Local files/directories

> Current version: **v2.0**

---

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration](#configuration)
  - [Optional Parameters](#optional-parameters)
- [Reports](#reports)
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
| `DefaultBranch` | bool | `true` = default branch only (faster). `false` = scan all branches and pick the largest. |
| `Branch` | string | Analyze a specific branch name across all repos. Leave blank for auto. |
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
| `GlobalReport.pdf / .json` | Organisation-wide totals: lines of code per language, largest repository, total repository and branch counts |
| `byfile-report/repository_summary.*` | Cross-repository file summary — lists every analysed repository with its total lines, blank lines, comments, and code lines |
| `byfile-report/*_byfile.*` | Per-repository file tree — one row per source file with individual line counts |
| `bylanguage-report/*.json` | Per-repository language breakdown — one row per detected language with line counts |

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
HTML               | .html, .htm, .cshtml, .vbhtml, ...       |                 | <!-- -->
Java               | .java, .jav                              | //              | /* */
JavaScript         | .js, .jsx, .jsp, .jspf                   | //              | /* */
JCL                | .jcl, .JCL                               | //*             |
JSON               | .json                                    |                 |
Kotlin             | .kt, .kts                                | //              | /* */
Objective-C        | .m, .mm                                  | //              | /* */
Oracle PL/SQL      | .pkb                                     | --              | /* */
PHP                | .php, .php3, .php4, .php5, .phtml, .inc  | //, #           | /* */
PL/I               | .pl1, .pli                               |                 | /* */
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
