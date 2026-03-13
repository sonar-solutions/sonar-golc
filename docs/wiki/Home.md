# Welcome to the GoLC Wiki

<p align="center">
  <img src="https://github.com/sonar-solutions/sonar-golc/raw/main/imgs/Logob.png" alt="GoLC logo" width="200" />
</p>

**GoLC** (*Go Line Counter*) counts physical lines of source code across your Bitbucket, GitHub, GitLab, and Azure DevOps repositories, using the same language definitions as SonarQube — so you can estimate LoC without running a full Sonar analysis.

---

## Quick links

| Topic | Description |
|-------|-------------|
| [**Introduction**](Introduction) | What GoLC is and which platforms it supports |
| [**Installation**](Installation) | Get the latest release and run the binary |
| [**Docker**](Docker) | Run with Docker or Docker Compose (full guide) |
| [**Prerequisites**](Prerequisites) | Tokens and tools you need |
| [**Usage**](Usage) | Configure and run GoLC (all platforms) |
| [**Reports**](Reports) | Output layout and ResultsAll web UI |
| [**Web UI**](Web-UI) | Screenshots and report examples |
| [**Supported languages**](Supported-languages) | Language list and how to add more |
| [**Execution Log**](Execution-Log) | Log file location and format |
| [**Future Features**](Future-Features) | Roadmap and how to contribute |

---

## Get started

1. Install from the [latest release](https://github.com/sonar-solutions/sonar-golc/releases).
2. Copy `config_sample.json` to `config.json` and add your token and organization (see [Usage](Usage)).
3. Run: `./golc -devops Github` (or your platform).
4. View results: `./ResultsAll` and open the URL in your browser.

For more detail, follow the links above or the main [README](https://github.com/sonar-solutions/sonar-golc#readme) in the repository.
