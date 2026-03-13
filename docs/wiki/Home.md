# Welcome to the GoLC Wiki

<p align="center">
  <img src="https://github.com/sonar-solutions/sonar-golc/raw/main/imgs/Logob.png" alt="GoLC logo" width="200" />
</p>

**GoLC** is a clever abbreviation for "Go Line Counter," drawing inspiration from [CLOC](https://github.com/AlDanial/cloc "AlDanial") and various other line-counting tools in Go like [GCloc](https://github.com/JoaoDanielRufino/gcloc "João Daniel Rufino").

**GoLC** counts physical lines of source code in numerous programming languages supported by the Developer, Enterprise, and Data Center editions of [SonarQube](https://www.sonarsource.com/knowledge/languages/) across your Bitbucket Cloud, Bitbucket Data Center (on-premises), GitHub.com (Cloud), GitHub Enterprise Server (on-premises), GitLab.com (Cloud), GitLab Self-Managed (on-premises), and Azure DevOps Services (Cloud) repositories. GoLC can be used to estimate LoC counts that would be produced by a Sonar analysis of these projects, without having to implement this analysis.

GoLC analyzes your repositories and identifies the largest branch of each repository, counting the total number of lines of code per language for that branch. At the end of the analysis, a text and PDF report is generated, along with a JSON results file for each repository. It starts an HTTP service to display an HTML page with the results.

> This last version is ver1.0.9 and is available for Bitbucket Cloud, Bitbucket Data Center (on-premises), GitHub.com (Cloud), GitHub Enterprise Server (on-premises), GitLab.com (Cloud), GitLab Self-Managed (on-premises), and Azure DevOps Services (Cloud) repositories and Files.


---

## Quick links

| Topic | Description |
|-------|-------------|
| [**Installation**](Docker) | Get the latest release and run the binary |
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
