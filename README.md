![Static Badge](https://img.shields.io/badge/Go-v1.22-blue:) [![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc) [![Lines of Code](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=ncloc&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc) [![Reliability Issues](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_reliability_issues&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc) [![Maintainability Rating](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_maintainability_rating&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)

## Introduction

<p align="center"><img src="imgs/Logob.png" alt="GoLC logo" /></p>

**GoLC** is a clever abbreviation for "Go Line Counter," drawing inspiration from [CLOC](https://github.com/AlDanial/cloc) and other line-counting tools in Go like [GCloc](https://github.com/JoaoDanielRufino/gcloc).

**GoLC** counts physical lines of source code in the programming languages supported by SonarQube (Developer, Enterprise, and Data Center editions) across your Bitbucket Cloud/DC, GitHub.com/Enterprise, GitLab.com/Self-Managed, and Azure DevOps repositories. It estimates LoC without running a full Sonar analysis. The tool analyzes your repositories, picks the largest branch per repo, counts lines per language, and produces text/PDF reports, JSON per repository, and an HTTP service to view results in the browser.

> Version **1.0.9** supports Bitbucket Cloud/DC, GitHub.com/Enterprise, GitLab.com/Self-Managed, Azure DevOps, and local **File** mode.

## Installation

Install from the [latest release](https://github.com/SonarSource-Demos/sonar-golc/releases/tag/V1.0.9) (e.g. download the binary for your OS). To run the latest build:

```bash
cp config_sample.json config.json
# Edit config.json with your token and organization (see Usage below)
./golc -devops Github
./ResultsAll   # optional: start web UI to view results
```

For **Docker** and **Docker Compose** (published image `timothe/sonar-golc`, env vars, mounts, port override), see the [Wiki → Docker](https://github.com/SonarSource-Demos/sonar-golc/wiki/Docker) section.

## Prerequisites

- **Personal access token** for your DevOps platform (Bitbucket Cloud/DC, GitHub, GitLab, Azure DevOps) with:
  - **repo** scope (or equivalent: e.g. GitLab `read_repository`, `read_api`)
  - Permission to list and clone repositories
- [Go](https://go.dev/) — only if you build from source.

## Usage

Copy `config_sample.json` to `config.json` and set your platform. Example for **GitHub.com**:

```json
"Github": {
  "Users": "your-username",
  "AccessToken": "ghp_xxxxxxxxxxxx",
  "Organization": "your-org"
}
```

Then run GoLC with the matching `-devops` flag. Supported values: `Github`, `GithubEnterprise`, `Gitlab`, `BitBucket`, `BitBucketSRV`, `Azure`, `File`.

```bash
./golc -devops Github
```

If the **Results** directory already exists, GoLC will ask whether to delete it and optionally create a backup. Reports are written under `Results/`. To view them in a browser, run `./ResultsAll` and open the URL it prints (default port 8091).

Full configuration for all platforms (GitHub Enterprise, GitLab, Bitbucket Cloud/DC, Azure, File mode), token details, and run examples are in the [Wiki → Usage](https://github.com/SonarSource-Demos/sonar-golc/wiki/Usage) section.

## Optional Parameters

| Parameter | Brief description |
|-----------|-------------------|
| **Period**, **Factor**, **Stats** | Reserved; do not modify. |
| **Multithreading**, **Workers** | Enable/disable parallel analysis and set concurrency. |
| **DefaultBranch** | If `true`, only the default branch per repo is analyzed. |
| **ExtExclusion** | Exclude files by extension (e.g. `[".css", ".js"]`). |
| **ResultByFile** | If `true`, get per-file JSON and run `ResultByfiles` for PDF. |
| **Branch** | Restrict to a single branch (e.g. `"main"`) for all repos. |
| **FileExclusion** | Use a `.cloc_<platform>_ignore` file to skip repos/projects. |
| **ResultAll** | Default report format (by language and by file). |
| **Org** (GitHub) | If `true`, analyze organization; if `false`, user account. |
| **ExcludePaths** | Exclude directories (e.g. `["test1", "pkg/test2"]`). |
| **Projects**, **Repos** | Limit scope to specific projects or repositories (BitBucket/Azure support **Projects**). |

Details and ignore-file syntax are in the [Wiki → Usage](https://github.com/SonarSource-Demos/sonar-golc/wiki/Usage#optional-parameters) section.

## Supported Languages

GoLC uses the same language definitions as SonarQube. Run `golc -languages` to print the full list. Summary:

| Language | Extensions | Single | Multi |
|----------|------------|--------|-------|
| Abap | .abap, .ab4, .flow, .asprog | *, " | — |
| ActionScript | .as | // | /* */ |
| Apex | .cls, .trigger | // | /* */ |
| C | .c | // | /* */ |
| C Header | .h | // | /* */ |
| C++ | .cpp, .cc | // | /* */ |
| C++ Header | .hh, .hpp | // | /* */ |
| C# | .cs | // | /* */ |
| COBOL | .cbl, .ccp, .cob, .cobol, .cpy | * | — |
| CSS | .css | — | /* */ |
| Dart | .dart | // | /* */ |
| Docker | Dockerfile | # | — |
| Golang | .go | // | /* */ |
| HTML | .html, .htm, .cshtml, .aspx, … | — | <!-- --> |
| Java | .java, .jav | // | /* */ |
| JavaScript | .js, .jsx, .jsp, .jspf | // | /* */ |
| Kotlin | .kt, .kts | // | /* */ |
| PHP | .php, .phtml, .inc, … | //, # | /* */ |
| Python | .py | # | """ """, ''' ''' |
| Ruby | .rb | # | =begin =end |
| Rust | .rs | // | /* */ |
| Scala | .scala | // | /* */ |
| Shell | .sh, .bash, .zsh, .ksh | # | — |
| SQL | .sql | -- | /* */ |
| Swift | .swift | // | /* */ |
| Terraform | .tf | #, // | /* */ |
| TypeScript | .ts, .tsx | // | /* */ |
| Vue | .vue | — | <!-- --> |
| XML | .xml, .XML | — | <!-- --> |
| YAML | .yaml, .yml | # | — |

Plus others (JCL, JSON, Objective-C, PL/I, RPG, Scss, T-SQL, VB6, Visual Basic .NET, XHTML, etc.). Full list: `golc -languages`. To add a language, extend the Languages structure in [assets/languages.go](assets/languages.go). See [Wiki → Supported-languages](https://github.com/SonarSource-Demos/sonar-golc/wiki/Supported-languages) for details.

## Future Features

Planned work includes: finer exclusion patterns, more integrations, UI improvements, performance and security enhancements. Contributions are welcome — open an issue or a PR if you’d like to help. See [Wiki → Future-Features](https://github.com/SonarSource-Demos/sonar-golc/wiki/Future-Features) for the roadmap.

---

**[Full documentation in Wiki](https://github.com/SonarSource-Demos/sonar-golc/wiki)** — Installation, Docker, all platform configs, Reports, Web UI, Execution log, and more.
