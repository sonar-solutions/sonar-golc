![Static Badge](https://img.shields.io/badge/Go-v1.25-blue:)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)
[![Lines of Code](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=ncloc)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)

# GoLC — Go Line Counter

![logo](imgs/Logob.png)

**GoLC** counts physical lines of source code across all programming languages supported by [SonarQube](https://www.sonarsource.com/knowledge/languages/) — without running a full Sonar analysis.

It connects to your DevOps platform, counts one branch per repository, and presents the results in an interactive web dashboard with PDF, JSON, and CSV exports.

**Supported platforms:** GitHub.com · GitHub Enterprise Server · GitHub Enterprise Cloud (including data residency) · GitLab Cloud · GitLab Self-Managed · Bitbucket Cloud · Bitbucket Data Center · Azure DevOps Services · Azure DevOps Server · Local files/directories

> Current version: **v2.1**

---

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration](#configuration)
  - [Required token permissions](#required-token-permissions)
  - [Azure DevOps Server](#azure-devops-server)
  - [Advanced options](#advanced-options)
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
| `golc-launcher` / `golc-launcher.exe` | Browser-based launcher — start here |
| `ResultsAll` / `ResultsAll.exe` | Results dashboard (launched automatically by `golc-launcher`) |

Run `golc-launcher` / `golc-launcher.exe`. The browser opens automatically to the GoLC UI (default: `http://localhost:8091`).

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
| Azure DevOps Server | Code: Read · Project and Team: Read |

#### GitHub Enterprise

The **GitHub Enterprise** card covers both self-hosted GitHub Enterprise Server and
GitHub Enterprise Cloud with data residency (a dedicated `*.ghe.com` address). Enter
your server's URL in the **Server URL** field — GoLC detects which variant it is
from the address and shows the result right under the field.

#### Azure DevOps Server

**Requires Azure DevOps Server 2019 Update 1 or later.** TFS 2018 and older are not
supported. There is no upper bound — newer versions work.

Two fields differ from Azure DevOps Services:

| Field | What to enter |
|-------|---------------|
| **Collection** | The path segment before the project in your URL — usually `DefaultCollection`. In `https://azuredevops.company.com/DefaultCollection/my-project`, that is `DefaultCollection`. |
| **Server URL** | Your server address, e.g. `https://azuredevops.company.com/`. Repositories are cloned from this host. |

#### Windows / NTLM authentication

Azure DevOps Server is often deployed behind Windows authentication, where personal access
tokens are disabled. To use it, fill in the **Username** field as `DOMAIN\username`; the
token field is then treated as that account's password.

Leave **Username** empty if your server accepts a personal access token — that is the
default behaviour and is unaffected.

---

### Advanced options

Click **Show advanced options** on the configuration screen. Everything below is optional;
the defaults are appropriate for most scans.

Fields that do not apply to the platform you picked are hidden automatically.

#### Branch

| Option | Default | What it does |
|--------|---------|--------------|
| **Analyze default branch only** | On | Counts each repository's default branch. Turn it off to count the most recently active branch instead — see [Which branch is counted](#which-branch-is-counted). |
| **Specific branch name** | empty | Counts this exact branch in every repository (e.g. `develop`). Only available when *Analyze default branch only* is off. |

#### Performance

| Option | Default | What it does |
|--------|---------|--------------|
| **Enable multithreading** | On | Analyses several repositories at once. |
| **Number of workers** | 10 | How many repositories are analysed simultaneously. Lower it if your server rate-limits you. |
| **Clone timeout (min)** | 15 | Per-repository deadline. A repository that takes longer is skipped and listed in the report rather than stalling the whole scan. `0` disables the deadline. |
| **Working directory for clones** | empty | Where repositories are cloned before counting, then deleted. Leave blank to use the system temp directory. Set it to a disk with free space if large scans fail with `no space left on device`. |

#### Scope

| Option | Shown for | What it does |
|--------|-----------|--------------|
| **Analyze as organization** | GitHub, GitHub Enterprise | On for an organization account, off for a personal account. |
| **Specific project key** | Bitbucket Cloud, Bitbucket Data Center, Azure DevOps, Azure DevOps Server | Limits the scan to one project. |
| **Specific repositories** | all except GitLab and File mode | Limits the scan to a comma-separated list. GitHub, Azure DevOps: repository names. Bitbucket: repository slugs. For GitLab, use the **Group URL slug(s)** field on the main form instead. |

#### Exclusions

Three presets add sets of folder names in one click:

| Preset | Folders excluded |
|--------|------------------|
| **Test directories** | `test`, `tests`, `spec`, `specs`, `e2e`, `testdata`, `fixtures`, `mock`, `mocks`, `integration`, `doc`, `docs` |
| **Vendor & modules** | `vendor`, `node_modules`, `bower_components`, `third_party`, `external` |
| **Build output** | `dist`, `build`, `out`, `target`, `bin`, `coverage` |

> **Test directories** matches SonarQube, which treats a file as test code — and so leaves
> it out of `ncloc` — when any folder on its path is named `doc`, `docs`, `test`, `tests`,
> `mock` or `mocks`.
>
> **Turn a preset off if your repository uses one of these names for code you ship.** A
> folder called `integration/` or `mocks/` holding production code will be skipped, and
> `integration` also matches a folder named `Integration.Something`.

Files are excluded by name too, whether or not you touch the presets:

| Default file name patterns | Excludes |
|----------------------------|----------|
| `*_test.*`, `test_*.*`, `*.test.*`, `*.spec.*`, `*_spec.*`, `*Test.*`, `*Tests.*` | test files, across every language that shares the convention |
| `*.min.*` | minified bundles |
| `*.Designer.*`, `*.g.cs`, `*.generated.*`, `*.pb.go`, `*_pb2.py`, `*.pb.cc`, `*.pb.h` | generated code |

> Generated code is recognised by name, so a generated file named like ordinary source is
> still counted. Add your own pattern under **Exclude file name patterns** if you have one.

And three fields for anything else:

| Field | What it does |
|-------|--------------|
| **Exclude folder keywords** | Skips folders whose name contains the keyword as a whole word, at any depth. Words are split on `-`, `_` and `.`, so `test` matches `integration-test/` and `test_helpers/` but not `protest/` or `latest/`. |
| **Exclude file name patterns** | Skips files whose name matches a glob, e.g. `*_test.go, *.min.js, *.spec.ts`. Matched against the file name only, not the full path. |
| **Exclude extensions** | Skips every file with these extensions regardless of language, e.g. `.css, .html`. |

> Exclusions apply while counting, so excluded files never reach the totals. To drop a
> repository *after* a scan instead, use the checkboxes on the results dashboard — see
> [Excluding repositories from the totals](#excluding-repositories-from-the-totals).

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

"Most recently active" means the branch with the most commits in a recent window — the
**last month** on every platform except **Bitbucket Data Center**, which looks back
**five months**.

> **If no branch has commits in that window, GoLC falls back to the default branch.** For
> repositories that have been quiet, turning the switch off therefore changes nothing —
> use *Specific branch name* if you need a particular branch counted regardless of activity.

---

## Reports

After each run, GoLC writes all reports to a `Results/` folder next to the binary. Click **View Results** in the browser to open the interactive dashboard, or open the folder directly.

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
| `repository_summary.pdf` | the main language |
| `GlobalReport.pdf` | the main language of each of the Top 30 repositories |

> JSON is excluded from these rankings, exactly as it is excluded from the code-line
> totals they sit beside.

### Excluding repositories from the totals

Some repositories should not count towards a sizing exercise — a repository that was
archived after the scan, a mirror, a vendored dependency dump. On the Results
dashboard, the **Lines of Code by Repository** table has a checkbox per repository.
Uncheck the ones to leave out and click **Apply selection** — the totals, language
breakdown, and chart update immediately.

This works on every platform, and **no re-scan is needed** — the repositories were
already counted, so only the totals are recomputed.

#### Original and customized reports

Once a selection exists, the **Reports** menu offers two sets:

| | Covers | Files |
|---|---|---|
| **Full scan** | every analysed repository, whatever is selected | `Results/GlobalReport.pdf`, `Results/byfile-report/repository_summary.*` |
| **Current selection** | only the selected repositories | `Results/customized/…` (same layout) |

The original is never overwritten, so it is always available for comparison. Downloads
are named `..._full-scan.pdf` and `..._selection.pdf` so the two cannot be confused
once detached from the dashboard.

Reports are generated **when you click them**, so applying a selection is instant and no
PDF is ever served stale. The first click after a change takes a moment while the report
is built.

Both PDFs and the CSV state what was excluded and what the unfiltered total was, and
the customized report's headline figures are explicitly labelled *(filtered)*, so a
filtered report can be handed to a customer without misleading them.

**It is always reversible.** **Reset to full scan** restores the original figures
exactly. The selection survives a dashboard restart, and at least one repository must
remain selected.

> A new analysis run clears the selection: it rediscovers repositories from scratch,
> so a selection made against the previous run could silently drop repositories from
> the new totals. To limit a scan *before* it runs, use **Specific repositories** or
> **Specific project key** under [Advanced options](#advanced-options).

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
C++                | .cpp, .cc, .cxx, .c++, .ipp, .ixx, ...   | //              | /* */
C++ Header         | .hh, .hpp, .hxx, .h++                    | //              | /* */
C#                 | .cs, .razor                              | //              | /* */
COBOL              | .cbl, .ccp, .cob, .cobol, .cpy           | *               |
CSS                | .css                                     |                 | /* */
Dart               | .dart                                    | //              | /* */
Docker             | Dockerfile, dockerfile                   | #               |
Flex               | .as                                      | //              | /* */
Golang             | .go                                      | //              | /* */
Gosu               | .gs, .gsx, .gsp                          | //              | /* */
Groovy             | .groovy, .gvy, .gy, .gsh, Jenkinsfile    | //              | /* */
HTML               | .html, .htm, .cshtml, .vbhtml, ...       |                 | <!-- -->
Java               | .java, .jav                              | //              | /* */
JavaScript         | .js, .jsx, .cjs, .mjs                    | //              | /* */
JCL                | .jcl, .JCL                               | //*             |
JSON               | .json                                    |                 |
JSP                | .jsp, .jspf, .jspx                       |                 | <%-- --%>, <!-- -->
Kotlin             | .kt, .kts                                | //              | /* */
Less               | .less                                    | //              | /* */
Objective-C        | .m, .mm                                  | //              | /* */
Oracle PL/SQL      | .pkb, .pks                               | --              | /* */
PHP                | .php, .php3, .php4, .php5, .phtml, .inc  | //, #           | /* */
PL/I               | .pl1, .pli                               |                 | /* */
PowerShell         | .ps1, .psm1, .psd1                       | #               | <# #>
Python             | .py                                      | #               | """ """, ''' '''
RPG                | .rpg, .rpgle, .sqlrpgle (+ uppercase)    | *               |
Ruby               | .rb                                      | #               | =begin =end
Rust               | .rs                                      | //              | /* */
Sass               | .sass                                    | //              | /* */
Scala              | .scala                                   | //              | /* */
Scss               | .scss                                    | //              | /* */
Shell              | .sh, .bash, .zsh, .ksh                   | #               |
SQL                | .sql                                     | --              | /* */
Swift              | .swift                                   | //              | /* */
Terraform          | .tf                                      | #, //           | /* */
T-SQL              | .tsql                                    | --              | /* */
Twig               | .twig                                    |                 | {# #}, <!-- -->
TypeScript         | .ts, .tsx, .cts, .mts                    | //              | /* */
VB6                | .bas, .frm, .cls, .ctl                   | '               |
Visual Basic .NET  | .vb                                      | '               |
Vue                | .vue                                     |                 | <!-- -->
XHTML              | .xhtml                                   |                 | <!-- -->
XML                | .xml, .XML, .xsd, .xsl, .config          |                 | <!-- -->
YAML               | .yaml, .yml                              | #               |
```

> **ActionScript and Flex both use `.as`**, so a report shows your `.as` files under one
> label or the other, and it may differ between runs. The line count is the same either
> way — only the name changes.

### Infrastructure-as-code, detected by content

A Kubernetes manifest, an Ansible playbook and an ordinary settings file are all `.yaml`,
so the extension alone cannot tell them apart — and the difference matters, because a
stock SonarQube counts infrastructure-as-code but not plain YAML or JSON.

GoLC recognises these from the file's content (or, for GitHub Actions, its path) and
reports each as its own language, so the breakdown lines up with SonarQube:

Language               | Recognised by
-----------------------+--------------------------------------------------------------
Kubernetes             | top-level `apiVersion:` **and** `kind:`
CloudFormation         | `AWSTemplateFormatVersion:`, or a resource with `Type: AWS::`
Ansible                | `hosts:` together with `tasks:`/`roles:`/`become:`
Azure Pipelines        | `stages:`/`jobs:`/`steps:` together with `pool:`/`trigger:`
GitHub Actions         | any file under `.github/workflows/`
Azure Resource Manager | JSON whose `$schema` names a `deploymentTemplate`

Anything unrecognised stays `YAML` or `JSON`.

### Minified JavaScript and CSS are never counted

SonarQube excludes minified files, so they never reach `ncloc`. GoLC applies the same
test — a file is treated as minified when:

- the name ends in `.min.js`, `-min.js`, `.min.css` or `-min.css`; **or**
- it is a `.js` or `.css` file whose average line length exceeds **200** characters.

The second rule catches a bundle committed under an ordinary name such as `vendor.js`. It
applies only to `.js` and `.css` — a long-lined `.ts` file is still counted, because
SonarQube counts it too.

### PHP open and close tags

A line holding only `<?php`, `<?` or `?>` is markup, not code, and is counted in no
category — matching SonarQube, which leaves such a line out of `ncloc`. A tag sharing a
line with code (`<?php $a = 1;`) is still a line of code.

---

## Execution Log

GoLC writes two logs to a `Logs/` folder next to the binary. Both are recreated on each run.

| File | Contains |
|------|----------|
| `Logs/Logs.log` | The same messages shown in the UI — authentication errors, rate limits, skipped repositories. |
| `Logs/debug.log` | Everything above plus per-repository detail. Click **Download Debug Log** on the progress screen to save it. |

Start with `Logs.log`. Send `debug.log` if you report a problem.

---

## Troubleshooting

### "Apple could not verify" message
When starting the app on MacOS you may get `Apple could not verify golc-launcher is free of malware that may harm your Mac or compromise your privacy."` message. To go around it, run:

```
xattr -cr /path/to/golc-launcher
xattr -cr /path/to/ResultsAll
```
