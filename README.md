![Static Badge](https://img.shields.io/badge/Go-v1.25-blue:)

[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=alert_status&token=8ec4d9fa8caaec10baf81b14f9411528c569312d)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)[![Lines of Code](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=ncloc&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Reliability Issues](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_reliability_issues&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Maintainability Rating](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_maintainability_rating&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)

## Table of Contents

- [Introduction](#introduction)
- [Installation](#installation)
- [Creating a Release](#creating-a-release)
- [Docker](#docker)
- [Quick Start — Web UI](#quick-start--web-ui)
- [Prerequisites](#prerequisites)
- [Configuration](#configuration)
  - [GitHub.com (Cloud)](#githubcom-cloud)
  - [GitHub Enterprise Server (on-premises)](#github-enterprise-server-on-premises)
  - [GitLab (Cloud and On-premises)](#gitlab-cloud-and-on-premises)
  - [Bitbucket Cloud](#bitbucket-cloud)
  - [Bitbucket Data Center (on-premises)](#bitbucket-data-center-on-premises)
  - [Azure DevOps Services (Cloud)](#azure-devops-services-cloud)
  - [File Mode](#file-mode)
  - [Optional Parameters](#optional-parameters)
- [Reports](#reports)
- [Supported Languages](#supported-languages)
- [Execution Log](#execution-log)
- [Future Features](#future-features)


## Introduction

![logo](imgs/Logob.png)

**GoLC** (Go Line Counter) counts physical lines of source code across all programming languages supported by [SonarQube](https://www.sonarsource.com/knowledge/languages/) — without running a full Sonar analysis.

It connects directly to your DevOps platform, identifies the largest branch of each repository, and counts total lines of code per language. At the end of the analysis it produces PDF, JSON, and CSV reports and launches a web dashboard to explore the results.

**Supported platforms:** GitHub.com · GitHub Enterprise Server · GitLab Cloud · GitLab Self-Managed · Bitbucket Cloud · Bitbucket Data Center · Azure DevOps Services · Local files/directories

> Current version: **v2.0** — available on the [`ver2.0`](https://github.com/sonar-solutions/sonar-golc/tree/ver2.0) branch.

---

## Installation

**Option 1 — Download a pre-built release**

Download the latest binaries from the [Releases page](https://github.com/sonar-solutions/sonar-golc/releases). Two binaries are available:

| Binary | Purpose |
|--------|---------|
| `webui` | Browser-based launcher with live progress |
| `ResultsAll` | Results web dashboard |

**Option 2 — Build from source**

Requires [Go 1.25+](https://go.dev/).

```bash
git clone https://github.com/sonar-solutions/sonar-golc.git
cd sonar-golc

go build -tags webui -o webui webui.go golc.go
go build -tags resultsall -o ResultsAll ResultsAll.go
```

---

## Creating a Release

The `create_release_sample.sh` script builds all binaries for every supported platform and uploads them as ZIP archives to a GitHub release.

**Prerequisites:** `bash`, `go`, `zip`, `curl`, `jq`, `git`

### 1. Configure the script

Open `create_release_sample.sh` and set the three variables at the top:

```bash
export TAG="V2.0"           # Git tag that will be created on GitHub
export Release1="2.0"       # Version number used in file names
export buildpath="/tmp/golc-releases/"  # Local directory for build output
```

Also update these two variables if releasing to a different repository:

```bash
GITHUB_ORG="sonar-solutions"   # GitHub organization
GITHUB_REPO="sonar-golc"       # Repository name
```

Update `RELEASE_DESCRIPTION` with the changelog for this release.

### 2. Set your GitHub token

The token must have `repo` scope and be authorized for the `sonar-solutions` organization (including SAML SSO if enforced):

```bash
export GITHUB_TOKEN=ghp_...
```

### 3. Run the script

```bash
chmod +x create_release_sample.sh
./create_release_sample.sh
```

The script will:
1. Build `webui` and `ResultsAll` for all 6 platform combinations (arm64/amd64 × macOS/Linux/Windows)
2. Package each combination into a ZIP archive containing the binaries, README, config sample, and assets
3. Create `source.zip` and `source.tar.gz` from the current git HEAD
4. Create (or update) the GitHub release at the specified tag and upload all archives

### Release archive contents

Each platform ZIP (e.g. `golc_2.0_darwin_arm64.zip`) contains:

```
golc_2.0_darwin_arm64/
├── webui          # Browser-based launcher (contains analysis engine)
├── ResultsAll     # Results dashboard
├── config.json    # Pre-filled from config_sample.json
├── README.md
├── LICENSE
└── imgs/
```

> On Windows, binaries are named `webui.exe` and `ResultsAll.exe`.

---

## Docker

Use the published image **`timothe/sonar-golc`** from Docker Hub. Config is provided via a mounted directory; the results dashboard is served on port 8090. Set the DevOps platform with the **`GOLC_DEVOPS`** environment variable.

```bash
mkdir -p config && cp config_sample.json config/config.json
# Edit config/config.json with your tokens and organization

docker run -p 8090:8090 \
  -v "$(pwd)/config:/config:ro" \
  -e GOLC_DEVOPS=Github \
  timothe/sonar-golc
```

To persist results on the host, add `-v "$(pwd)/data:/data"`. View logs with `docker logs <container>`.

**Docker Compose:**
```bash
docker pull timothe/sonar-golc && docker tag timothe/sonar-golc sonar-golc
mkdir -p config && cp config_sample.json config/config.json
# Edit config/config.json, then:
docker compose up
```

| Variable | Default | Description |
|----------|---------|-------------|
| `GOLC_DEVOPS` | `Github` | Platform key: `Github`, `Gitlab`, `BitBucket`, `BitBucketSRV`, `Azure`, `File` |
| `GOLC_RESULTS_PORT` | `8090` | Port for the ResultsAll dashboard (also accepts legacy `PORT`) |
| `GOLC_WEBUI_PORT` | `8091` | Port for the Web UI (not applicable in Docker mode) |

See [docs/docker.md](docs/docker.md) for full Docker usage details.

---

## Port Configuration

Both binaries pick up their port from environment variables, falling back to defaults:

| Variable | Binary | Default | Description |
|----------|--------|---------|-------------|
| `GOLC_WEBUI_PORT` | `webui` | `8091` | Web UI listening port |
| `GOLC_RESULTS_PORT` | `ResultsAll` | `8090` | Results dashboard listening port |

```bash
GOLC_WEBUI_PORT=9001 ./webui
GOLC_RESULTS_PORT=9002 ./ResultsAll
```

---

## Quick Start — Web UI

The Web UI is the easiest way to run GoLC. It provides a browser-based step-by-step interface to configure your platform, run the analysis with a live progress bar, and open the results dashboard — no manual config editing required.

**Start the Web UI:**
```bash
./webui
# Open http://localhost:8091 in your browser
```

**Workflow:**

1. **Choose your platform** — click the platform card (GitHub, GitLab, Bitbucket, Azure DevOps, or File).
2. **Configure** — enter your credentials. Settings from your last run are pre-filled automatically.
3. **Run Analysis** — click "Run Analysis". A live log and progress bar show real-time status.
4. **View Results** — when complete, click "View Results" to open the results dashboard.

> **GitLab tip:** Leave the "Group URL slug" field blank to automatically discover and analyze all groups your token has access to.

---

## Prerequisites

A personal access token for your DevOps platform with the following minimum permissions:

| Platform | Required scopes |
|----------|----------------|
| GitHub / GitHub Enterprise | `repo` |
| GitLab | `read_repository`, `read_api` |
| Bitbucket Cloud | Repositories: Read · Projects: Read · Account: Read |
| Bitbucket Data Center | Repo read, pull |
| Azure DevOps | Code: Read |

---

## Configuration

Copy `config_sample.json` to `config.json` and fill in your credentials:

```bash
cp config_sample.json config.json
```

The sections below show the minimum required fields for each platform.

---

### GitHub.com (Cloud)

```json
"Github": {
  "Users": "your-github-login",
  "AccessToken": "ghp_...",
  "Organization": "your-org"
}
```

---

### GitHub Enterprise Server (on-premises)

```json
"GithubEnterprise": {
  "Users": "your-login",
  "AccessToken": "ghp_...",
  "Organization": "your-org",
  "Url": "https://github.yourcompany.com/",
  "Baseapi": "github.yourcompany.com",
  "Protocol": "https"
}
```

---

### GitLab (Cloud and On-premises)

```json
"Gitlab": {
  "Users": "your-gitlab-login",
  "AccessToken": "glpat-...",
  "Organization": "group-url-slug"
}
```

**Important notes:**
- `Organization` must be the **URL slug** (the path shown in your browser URL), not the display name. For example, if your group URL is `https://gitlab.com/my-group`, use `"Organization": "my-group"`.
- To analyze **multiple groups**, use a comma-separated list: `"Organization": "group1,group2"`.
- **Leave `Organization` blank** to automatically discover and analyze all groups your token has access to.
- The default URL is `https://gitlab.com/`. For **GitLab Self-Managed**, override it:

```json
"Gitlab": {
  "Url": "https://gitlab.yourcompany.com/",
  "Protocol": "https"
}
```

---

### Bitbucket Cloud

```json
"BitBucket": {
  "Users": "your.email@example.com",
  "AccessToken": "ATATT3x...",
  "Workspace": "your-workspace-slug",
  "Organization": "your-workspace-slug"
}
```

> **Token type:** Bitbucket Cloud requires **API Tokens** (not App Passwords). Create one in your Atlassian account settings. The token starts with `ATATT3x...`.
>
> **`Users`** must be your **email address**, not your username.

---

### Bitbucket Data Center (on-premises)

```json
"BitBucketSRV": {
  "Users": "your-login",
  "AccessToken": "your-token",
  "Organization": "your-org",
  "Url": "https://bitbucket.yourcompany.com/",
  "Protocol": "https"
}
```

---

### Azure DevOps Services (Cloud)

```json
"Azure": {
  "AccessToken": "your-pat",
  "Organization": "your-org"
}
```

> **Token scopes required:** Code: Read · Project and Team: Read

---

### File Mode

Analyze local directories. Set the `Directory` field, or create a `.cloc_file_load` file listing directories one per line (overrides `Directory`).

```json
"File": {
  "Organization": "my-label",
  "Directory": "../my-repos/."
}
```

---

### Optional Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `DefaultBranch` | bool | `true` = analyze only the default branch (faster). `false` = scan all branches and pick the largest. |
| `Branch` | string | Analyze a specific branch name across all repos (e.g. `"main"`). Leave blank for auto. |
| `Multithreading` | bool | Enable parallel analysis. Default: `true`. |
| `Workers` | int | Number of concurrent workers. Default: `10`. Increase for faster analysis on capable hardware. |
| `ExcludePaths` | array | Directories to exclude within each repo (e.g. `["test", "pkg/generated"]`). Matched at repo root, fully recursive. |
| `ExtExclusion` | array | File extensions to exclude (e.g. `[".css", ".js"]`). |
| `FileExclusion` | string | Path to a file listing repos/projects to skip (e.g. `.cloc_github_ignore`). |
| `ResultByFile` | bool | Generate per-file results in addition to per-language summaries. |
| `Project` | string | Limit analysis to a specific project key (Bitbucket, Azure DevOps). |
| `Repos` | string | Limit analysis to specific repo slugs (comma-separated). |
| `Org` | bool | `true` = analyze an organization. `false` = analyze a personal account (GitHub only). |

**Exclusion file syntax** (`.cloc_<platform>_ignore`):

```
# Bitbucket / Azure DevOps
PROJECT_KEY
PROJECT_KEY/REPO_SLUG
REPO_SLUG

# GitHub / GitLab
repo-slug-1
repo-slug-2
```

---

## Reports

Reports are written to the `Results` directory:

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

### Report example

![report](imgs/report.png)

Report by file:

![report](imgs/reportbyfiles.png)

---

## Supported Languages

```
Language           | Extensions                               | Single Comments | Multi Line Comments
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

GoLC writes a detailed log to `Logs/Logs.log` in the current directory. The file is recreated on each run. Use it to troubleshoot issues or monitor execution.

---

## Future Features

- **Improved exclusion patterns** for more flexible per-repo control
- **Additional platform integrations**
- **Performance optimizations** for large organizations
- **Security enhancements**
