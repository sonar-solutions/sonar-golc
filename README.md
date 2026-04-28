![Static Badge](https://img.shields.io/badge/Go-v1.25-blue:)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=alert_status&token=8ec4d9fa8caaec10baf81b14f9411528c569312d)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)[![Lines of Code](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=ncloc&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Reliability Issues](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_reliability_issues&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Maintainability Rating](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_maintainability_rating&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)

# GoLC — Go Line Counter

![logo](imgs/Logob.png)

**GoLC** counts physical lines of source code across all programming languages supported by [SonarQube](https://www.sonarsource.com/knowledge/languages/) — without running a full Sonar analysis.

It connects to your DevOps platform, identifies the largest branch of each repository, counts lines of code per language, and presents the results in an interactive web dashboard with PDF, JSON, and CSV exports.

**Supported platforms:** GitHub.com · GitHub Enterprise Server · GitLab Cloud · GitLab Self-Managed · Bitbucket Cloud · Bitbucket Data Center · Azure DevOps Services · Local files/directories

> Current version: **v2.0** — [`ver2.0`](https://github.com/sonar-solutions/sonar-golc/tree/ver2.0) branch.

---

## Table of Contents

- [Quick Start](#quick-start)
- [Installation](#installation)
- [Docker](#docker)
- [Configuration](#configuration)
  - [Optional Parameters](#optional-parameters)
- [Reports](#reports)
- [Supported Languages](#supported-languages)
- [Execution Log](#execution-log)
- [Creating a Release](#creating-a-release)

---

## Quick Start

```bash
./webui
```

Open the URL printed in the terminal (default: `http://localhost:8091`), then:

1. **Choose your platform** — click the platform card.
2. **Enter credentials** — token, organization, and any other required fields. Previous settings are pre-filled automatically.
3. **Run Analysis** — click "Run Analysis" and watch the live progress bar.
4. **View Results** — click "View Results" when complete. The results dashboard opens automatically.

> **Ports are managed automatically.** If the default port is in use, GoLC picks the next free one. The actual URL is always printed on startup.

> **GitLab tip:** Leave the Organization field blank to auto-discover all groups your token has access to.

---

## Installation

### Option 1 — Pre-built binaries

Download the latest release from the [Releases page](https://github.com/sonar-solutions/sonar-golc/releases).

Each archive contains two binaries:

| Binary | Purpose |
|--------|---------|
| `webui` / `webui.exe` | Browser-based launcher — start here |
| `ResultsAll` / `ResultsAll.exe` | Results dashboard (launched automatically by `webui`) |

### Option 2 — Build from source

Requires [Go 1.25+](https://go.dev/).

```bash
git clone https://github.com/sonar-solutions/sonar-golc.git
cd sonar-golc

go build -tags webui -o webui webui.go golc.go
go build -tags resultsall -o ResultsAll ResultsAll.go
```

---

## Docker

Docker gives you the same browser UI as the native binary — no config files to edit.

```bash
docker run -p 8091:8091 -p 8090:8090 -v "$(pwd)/data:/data" fabiogos846/sonar-golc
```

Open `http://localhost:8091`, configure your platform in the browser, and click **Run Analysis**. Results open at `http://localhost:8090` when complete. Everything is saved to the mounted `data/` directory and persists across restarts.

**Docker Compose:**

```bash
docker compose up
```

The included `docker-compose.yml` mounts `./data` automatically.

| Variable | Default | Description |
|----------|---------|-------------|
| `GOLC_WEBUI_PORT` | `8091` | Browser UI port |
| `GOLC_RESULTS_PORT` | `8090` | Results dashboard port |

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

### Platform notes

**GitLab** — The Organization field expects the **URL slug** (e.g. `my-group` from `https://gitlab.com/my-group`), not the display name. Use a comma-separated list for multiple groups, or leave it blank to auto-discover all groups your token has access to.

**Bitbucket Cloud** — Use an **API Token** (not an App Password) from your Atlassian account settings. The Username field must be your email address.

**File Mode** — To analyze multiple directories, create a `.cloc_file_load` file with one path per line. It takes precedence over the Directory field in the UI.

---

### Optional Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `DefaultBranch` | bool | `true` = default branch only (faster). `false` = scan all branches and pick the largest. |
| `Branch` | string | Analyze a specific branch name across all repos. Leave blank for auto. |
| `Multithreading` | bool | Enable parallel analysis. Default: `true`. |
| `Workers` | int | Concurrent workers. Default: `10`. |
| `ExcludePaths` | array | Directories to exclude per repo (e.g. `["test", "vendor"]`). |
| `ExtExclusion` | array | File extensions to exclude (e.g. `[".css", ".min.js"]`). |
| `FileExclusion` | string | Path to a file listing repos to skip (e.g. `.cloc_github_ignore`). |
| `ResultByFile` | bool | Generate per-file breakdowns in addition to per-language summaries. |
| `Project` | string | Limit to a specific project key (Bitbucket, Azure DevOps). |
| `Repos` | string | Limit to specific repo slugs (comma-separated). |
| `Org` | bool | `true` = organization, `false` = personal account (GitHub only). |

**Exclusion file format** (`.cloc_<platform>_ignore`):

```
# GitHub / GitLab — one repo slug per line
repo-slug-1
repo-slug-2

# Bitbucket / Azure DevOps
PROJECT_KEY
PROJECT_KEY/REPO_SLUG
```

---

## Reports

Reports are written to the `Results` directory after each run:

```
Results/
├── bylanguage-report/
│   └── Result_<repo>.json
├── byfile-report/
│   ├── csv-report/
│   │   └── Result_<repo>_byfile.csv
│   ├── pdf-report/
│   │   └── Result_<repo>_byfile.pdf
│   └── Result_<repo>_byfile.json
├── GlobalReport.json
├── GlobalReport.pdf
└── GlobalReport.txt
```

![report](imgs/report.png)

![report](imgs/reportbyfiles.png)

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

GoLC writes a detailed log to `Logs/Logs.log` in the working directory. The file is recreated on each run. Use it to troubleshoot authentication errors, rate limits, or unexpected results.

---

## Creating a Release

The `create_release_sample.sh` script builds release archives for all platforms and optionally produces a multi-arch Docker image.

**Prerequisites:** `bash`, `go`, `zip`, `git` · Docker with `buildx` for the Docker step.

### 1. Configure

```bash
export TAG="v2.0"                              # Version tag embedded in the binaries
export Release1="2.0"                          # Used in archive file names
export buildpath="/tmp/golc-releases/"         # Output directory
DOCKER_IMAGE="your-dockerhub-user/sonar-golc"  # Docker Hub image name
```

### 2. Run

```bash
chmod +x create_release_sample.sh
./create_release_sample.sh
```

The script builds `webui` and `ResultsAll` for all 6 platform combinations (arm64/amd64 × macOS/Linux/Windows), packages each into a ZIP archive, and creates `source.zip` / `source.tar.gz` from the current git HEAD.

The Docker build section is commented out by default. Uncomment it to build a multi-arch image. To push to Docker Hub, run the command printed at the end of the script.
