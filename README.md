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
- [Creating a Release](#creating-a-release)

---

## Quick Start

Download the latest release from the [Releases page](https://github.com/sonar-solutions/sonar-golc/releases).

Each archive contains two binaries:

| Binary | Purpose |
|--------|---------|
| `webui` / `webui.exe` | Browser-based launcher — start here |
| `ResultsAll` / `ResultsAll.exe` | Results dashboard (launched automatically by `webui`) |

Then run it for your operating system:

**Mac / Linux**
```bash
./webui
```

**Windows** (Command Prompt)
```bat
webui.exe
```

**Windows** (PowerShell)
```powershell
.\webui.exe
```

Open the URL printed in the terminal (default: `http://localhost:8091`), then:

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
| `ExcludePaths` | array | Directories to exclude per repo (e.g. `["test", "vendor"]`). |
| `ExtExclusion` | array | File extensions to exclude (e.g. `[".css", ".min.js"]`). |
| `FileExclusion` | string | Path to a file listing repos to skip (e.g. `.cloc_github_ignore`). |
| `ResultByFile` | bool | Generate per-file breakdowns in addition to per-language summaries. |
| `Project` | string | Limit to a specific project key (Bitbucket, Azure DevOps). |
| `Repos` | string | Limit to specific repo slugs (comma-separated). |
| `Org` | bool | `true` = organization, `false` = personal account (GitHub only). |


---

## Reports

Reports are available at the `Results` page after each run:



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
