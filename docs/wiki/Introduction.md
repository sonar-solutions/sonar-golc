# Introduction

![logo](https://github.com/SonarSource-Demos/sonar-golc/raw/main/imgs/Logob.png)

**GoLC** is a clever abbreviation for "Go Line Counter," drawing inspiration from [CLOC](https://github.com/AlDanial/cloc "AlDanial") and various other line-counting tools in Go like [GCloc](https://github.com/JoaoDanielRufino/gcloc "João Daniel Rufino").

**GoLC** counts physical lines of source code in numerous programming languages supported by the Developer, Enterprise, and Data Center editions of [SonarQube](https://www.sonarsource.com/knowledge/languages/) across your Bitbucket Cloud, Bitbucket Data Center (on-premises), GitHub.com (Cloud), GitHub Enterprise Server (on-premises), GitLab.com (Cloud), GitLab Self-Managed (on-premises), and Azure DevOps Services (Cloud) repositories. GoLC can be used to estimate LoC counts that would be produced by a Sonar analysis of these projects, without having to implement this analysis.

GoLC analyzes your repositories and identifies the largest branch of each repository, counting the total number of lines of code per language for that branch. At the end of the analysis, a text and PDF report is generated, along with a JSON results file for each repository. It starts an HTTP service to display an HTML page with the results.

> This last version is ver1.0.9 and is available for Bitbucket Cloud, Bitbucket Data Center (on-premises), GitHub.com (Cloud), GitHub Enterprise Server (on-premises), GitLab.com (Cloud), GitLab Self-Managed (on-premises), and Azure DevOps Services (Cloud) repositories and Files.
