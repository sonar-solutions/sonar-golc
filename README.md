![Static Badge](https://img.shields.io/badge/Go-v1.22-blue:)

[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=sonar-solutions_sonar-golc&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=sonar-solutions_sonar-golc)[![Lines of Code](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=ncloc&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Reliability Issues](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_reliability_issues&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)[![Maintainability Rating](https://nautilus.sonarqube.org/api/project_badges/measure?project=SonarSource-Demos_sonar-golc&metric=software_quality_maintainability_rating&token=sqb_44cfc298b697f0c4fcbb32de1de67db5ca2c341f)](https://nautilus.sonarqube.org/dashboard?id=SonarSource-Demos_sonar-golc)

## Table of Contents

- [Introduction](#introduction)
- [Installation](#installation)
- [Docker](#docker)
- [Prerequisites](#prerequisites)
- [Usage](#usage)
  - [Environment Configuration](#environment-configuration)
  - [GitHub.com (Cloud) Basic Configuration](#githubcom-cloud-basic-configuration)
  - [GitHub Enterprise Server (on-premises) Basic Configuration](#github-enterprise-server-on-premises-basic-configuration)
  - [GitLab (Cloud and On-premises) Basic Configuration](#gitlab-cloud-and-on-premises-basic-configuration)
  - [Bitbucket Cloud Basic Configuration](#bitbucket-cloud-basic-configuration)
  - [Bitbucket Data Center (on-premises) Basic Configuration](#bitbucket-data-center-on-premises-basic-configuration)
  - [Azure DevOps Services (Cloud) Basic Configuration](#azure-devops-services-cloud-basic-configuration)
  - [File Mode Basic Configuration](#file-mode-basic-configuration)
  - [Optional Parameters](#optional-parameters)
  - [Run GoLC](#run-golc)
- [Reports](#reports)
- [Web UI](#web-ui)
- [Supported languages](#supported-languages)
- [Execution Log](#execution-log)
- [Future Features](#future-features)


## Introduction

![logo](imgs/Logob.png)

**GoLC** is a clever abbreviation for "Go Line Counter," drawing inspiration from [CLOC](https://github.com/AlDanial/cloc "AlDanial") and various other line-counting tools in Go like [GCloc](https://github.com/JoaoDanielRufino/gcloc "João Daniel Rufino").

**GoLC** counts physical lines of source code in numerous programming languages supported by the Developer, Enterprise, and Data Center editions of [SonarQube](https://www.sonarsource.com/knowledge/languages/) across your Bitbucket Cloud, Bitbucket Data Center (on-premises), GitHub.com (Cloud), GitHub Enterprise Server (on-premises), GitLab.com (Cloud), GitLab Self-Managed (on-premises), and Azure DevOps Services (Cloud) repositories.
GoLC can be used to estimate LoC counts that would be produced by a Sonar analysis of these projects, without having to implement this analysis.

GoLC The tool analyzes your repositories and identifies the largest branch of each repository, counting the total number of lines of code per language for that branch. At the end of the analysis, a text and PDF report is generated, along with a JSON results file for each repository.It starts an HTTP service to display an HTML page with the results.

> This last version is ver1.0.9 is available for Bitbucket Cloud, Bitbucket Data Center (on-premises), GitHub.com (Cloud), GitHub Enterprise Server (on-premises), GitLab.com (Cloud), GitLab Self-Managed (on-premises), and Azure DevOps Services (Cloud) repositories and Files.


---
## Installation

You can install from the stable release by clicking [here](https://github.com/SonarSource-Demos/sonar-golc/releases/tag/V1.0.9)

## Docker

Use the published image **`timothe/sonar-golc`** from Docker Hub. Config is provided via a mounted directory; the web UI is served on port 8092. The DevOps platform to analyze is set with the **`GOLC_DEVOPS`** environment variable (e.g. `Github`, `Gitlab`, `BitBucket`, `File`).

**Run with the published image:**
```bash
mkdir -p config && cp config_sample.json config/config.json
# Edit config/config.json with your tokens and organization

docker run -p 8092:8092 \
  -v "$(pwd)/config:/config:ro" \
  -e GOLC_DEVOPS=Github \
  timothe/sonar-golc
```

To persist results on the host, add: `-v "$(pwd)/data:/data"`. After the analysis completes, open **http://localhost:8092** to view the results. View logs with `docker logs <container>`.

**Building the image** (optional, e.g. for local development): `docker build -t sonar-golc .` — then use `sonar-golc` instead of `timothe/sonar-golc` in the commands below.

**Docker Compose:** Put `config.json` in a `config/` directory. The Compose file uses the image `sonar-golc` (pull the published image and tag it, or use your built image):

```bash
docker pull timothe/sonar-golc && docker tag timothe/sonar-golc sonar-golc
mkdir -p config && cp config_sample.json config/config.json
# Edit config/config.json, then:
docker compose up
```

**Docker Compose variables:** Set these in a `.env` file in the project root or pass them when running (e.g. `GOLC_DEVOPS=Gitlab docker compose up`).

| Variable | Default | Description |
|----------|---------|-------------|
| `GOLC_DEVOPS` | `Github` | DevOps platform to analyze. Must match a key in your config (e.g. `Github`, `Gitlab`, `BitBucket`, `BitBucketSRV`, `Azure`, `File`). |

The Compose file mounts `./config` read-only; `/data` uses an anonymous volume so results persist across restarts.

See [docs/docker.md](docs/docker.md) for full Docker and Compose usage (all env vars, port override, bind mounts).

## Prerequisites 

* A personal access tokens for : Bitbucket Cloud,Bitbucket DC, GitHub, GitLab and Azure DevOps. The token must have:
     - repo scope.
     - Perform pull request actions
     - Push, pull and clone repositories
     - For example, for GitLab, the permissions needed are: read_repository, read_api
  
* [Go language installed](https://go.dev/) : If you want to use the sources...


 ## Usage

 ### Environment Configuration

 Before running GoLC, you need to configure your environment by initializing the various values in the config.json file.
 If using the sources, copy the **config_sample.json** file to **config.json** and modify the various entries.


## GitHub.com (Cloud) Basic Configuration:

Specify the following parameters in the config.json file:

 ```json
"Github": { 
  "Users": "xxxxxxxxxxxxxx" : Your User login
  "AccessToken": "xxxxxxxxxxxxxx" : Your Token
  "Organization": "xxxxxx": Your organization
 ```

Save the config.json file and [Run GoLC](#run-golc)

## GitHub Enterprise Server (on-premises) Basic Configuration:

Specify the following parameters in the config.json file:

```json
"GithubEnterprise": {
  "Users": "xxxxxxxxxxxxxx" : Your User login
  "AccessToken": "xxxxxxxxxxxxxx" : Your Token
  "Organization": "xxxxxx": Your organization
  "Url": "https://github.yourcompany.com/": Your GitHub Enterprise Server URL
  "Baseapi": "github.yourcompany.com", Your GitHub Enterprise Server hostname
  "Protocol": "https" Adjust the protocol used if needed
}
```

Save the config.json file and [Run GoLC](#run-golc)


## GitLab (Cloud and On-premises) Basic Configuration:


For GitLab (Cloud and On-premises), specify the following parameters in the config.json file:

 ```json
"Gitlab": { 
  "Users": "xxxxxxxxxxxxxx" : Your User login
  "AccessToken": "xxxxxxxxxxxxxx" : Your Token
  "Organization": "xxxxxx": Your group
 ```

You can specify multiple groups by providing a comma-separated list in `Organization`, e.g., `"Organization": "group1,group2"`. A single group works as `"Organization": "group1"`.


For **GitLab Self-Managed (on-premises)**, also modify the URL configuration:

```json
"Gitlab": {
  "Url": "https://gitlab.yourcompany.com/": Your GitLab Self-Managed Server URL
  "Protocol": "https": Adjust the protocol used if needed
}
```

Save the config.json file and [Run GoLC](#run-golc)

## Bitbucket Cloud Basic Configuration:

For Bitbucket Cloud, specify the following parameters in the config.json file:

```json
"BitBucket": { 
  "Users": "your.email@example.com": Your Atlassian account email address
  "AccessToken": "ATATT3x...": Your Bitbucket API token
  "Workspace": "your-workspace-slug": Your workspace slug/ID
  "Organization": "your-workspace-slug": Your organization/workspace name (typically same as Workspace)
  "Project": "your-project-slug1,your-project-slug2": (Optional) comma seperated list of project slugs
}
```

### Token Requirements

**Important**: Bitbucket Cloud requires **API Tokens** (not App Passwords) for authentication. App Passwords have been deprecated as of June 2025.

Grant the following scopes to the API Token:
   - **Repositories: Read** - Required to list and access repositories
   - **Projects: Read** - Required to list projects
   - **Account: Read** - Required for user information


The token will start with `ATATT3x...` and is a long string.

#### Finding Your Workspace

Your workspace slug can be found in your Bitbucket URL:
- If your Bitbucket URL is `https://bitbucket.org/your-workspace/`, then `your-workspace` is your workspace slug
- You can also find it in your workspace settings

#### Configuration Example

```json
"BitBucket": { 
  "Users": "john.doe@example.com",
  "AccessToken": "ATATT3x...your-token-here",
  "Workspace": "my-workspace",
  "Organization": "my-workspace"
}
```

❗️ **Important Notes**:
- The **Workspace** parameter is required and is used for all Bitbucket Cloud API operations
- The **Organization** parameter is used for reporting purposes and should typically be set to the same value as your workspace name
- The **Users** field must contain your **email address** (not username) for proper authentication
- API tokens are required - App Passwords are no longer supported

Save the config.json file and [Run GoLC](#run-golc)

## Bitbucket Data Center (on-premises) Basic Configuration:

For Bitbucket Data Center (on-premises), specify the following parameters in the config.json file:

```json
"BitBucketSRV": {
  "Users": "xxxxxxxxxxxxxx" : Your User login
  "AccessToken": "xxxxxxxxxxxxxx" : Your Token
  "Organization": "xxxxxx": Your organization
  "Url": "https://bitbucket.yourcompany.com/": Your Bitbucket Data Center Server URL (ending with a '/')
  "Protocol": "https": Adjust the protocol used if needed
}
```

Save the config.json file and [Run GoLC](#run-golc)

## Azure DevOps Services (Cloud) Basic Configuration:

For Azure DevOps Services (Cloud), specify the following parameters in the config.json file:

```json
"Azure": { 
  "Users": "xxxxxxxxxxxxxx" : Your User login
  "AccessToken": "xxxxxxxxxxxxxx" : Your Token
  "Organization": "xxxxxx": Your organization
}
```

Save the config.json file and [Run GoLC](#run-golc)


 ## File Mode Basic Configuration

For the **File** mode, if you want to have a list of directories to analyze, you create a **.cloc_file_load** file and add the directories to be analyzed line by line.If the **.cloc_file_load**. file is provided, its contents will override the **Directory** parameter.

## Optional Parameters

❗️ The parameters **'Period'**, **'Factor'**, and **'Stats'** should not be modified as they will be used in a future version.

❗️ The parameters **'Multithreading'** and **'Workers'** initialize whether multithreading is enabled or not, allowing parallel analysis. You can disable it by setting **'Multithreading'** to **false**. **'Workers'** corresponds to the number of concurrent analyses.These parameters can be adjusted according to the performance of the compute running GoLC.

❗️ The boolean parameter **DefaultBranch**, if set to true, specifies that only the default branch of each repository should be analyzed. If set to false, it will analyze all branches of each repository to determine the most important one.

❗️ Exclude extensions.
If you want to exclude files by their extensions, use the parameter **'ExtExclusion'**. For example, if you want to exclude all CSS or JS files : 'ExtExclusion':[".css",".js"],

❗️ Results By File.
If you want results by file rather than globally by language, you need to set the **'ResultByFile'** parameter to true in the **config.json** file. In the **Results** directory, you will then have a JSON file for each analyzed repository containing a list of files with details such as the number of lines of code, comments, etc. Additionally, a PDF file named **complete_report.pdf** will be available in the **Results/reports** directory. To generate this report, you need to run the **ResultByfiles** program.

❗️Selecting a specific branch.
The '**Branch**' input allows you to select a specific branch for all repositories within an organization or project, or for a single repository. For example, if you only want all branches to be "main", '**"Branch":"main"**' .

❗️ File Exclusions.
You can create a **.cloc_'your_platform'_ignore** file to ignore projects or repositories in the analysis. 
```json
   "FileExclusion":".cloc_bitbucketdc_ignore"
```
The syntax of this file is as follows for BitBucket:

```
REPO_SLUG
PROJECT_KEY 
PROJECT_KEY/REPO_SLUG
```

```
- REPO_SLUG = for one Repository
- PROJECT_KEY = for one Project
- PROJECT_KEY/REPO_SLUG For un Repository in one Project
```

The syntax of this file is as follows for GitHub:

```
REPO1_SLUG
REPO2_SLUG
...
```

```
- REPO1_SLUG = for one Repository
```

**File Mode Configuration:**

The syntax of this file is as follows for File:

```
DIRECTORY_NAME
FILE_NAME
...
```

**Azure DevOps Ignore File Configuration:**

The syntax of this file is as follows for Azure DevOps:

```
PROJECT_KEY/REPO_SLUG
PROJECT_KEY
```

❗️ Results All.
Results ALL is the default report format. It generates a report for by language and a report for by file. The variable to initialize this mode is **'ResultAll'**, which is set to true in the configuration file **config.json.**"

❗️ The boolean parameter **Org**, if set to true, will run the analysis on an organization. If set to false, it will run on a user account. The **Organization** parameter should be set to your personal account. This functionality is available for GitHub.

❗️ Exclude directories.
To exclude directories from your repository from the analysis, initialize the variable **'ExcludePaths': ['']**. For example, to exclude two directories: **'ExcludePaths': ['test1', 'pkg/test2']**.

❗️ If '**Projects**' and '**Repos**' are not specified, the analysis will be conducted on all repositories. You can specify a project name (PROJECT_KEY) in '**Projects**', and the analysis will be limited to the specified project. If you specify '**Repos**' (REPO_SLUG), the analysis will be limited to the specified repositories.
```json
"Project": "",
"Repos": "",
```
❗️ The '**Projects**' entry is supported exclusively on the BitBucket and AzureDevops platform.

❗️ **Log level.** You can control output verbosity for troubleshooting. In **config.json**, set `"Logging": { "Level": "info" }` to one of `trace`, `debug`, `info`, `warn`, or `error` (default is `info`). From the command line, use **`-log-level=<level>`** to override the config (e.g. `-log-level=debug`), or **`-verbose`** / **`-v`** to set the level to debug. Use `debug` or `trace` for file-level and report-level troubleshooting (files found, excluded, scanned; reports created with metrics).

 ## Run GoLC

 To launch GoLC with the following command, you must specify your DevOps platform. In this example, we analyze repositories hosted on Bitbucket Cloud. The supported flags for -devops are :
 ```bash
flag : <BitBucketSRV>||<BitBucket>||<Github>||<GithubEnterprise>||<Gitlab>||<Azure>||<File>

 ```
 You can fine-tune log output with **`-log-level=debug`** (or `trace`, `info`, `warn`, `error`) or **`-verbose`** / **`-v`** for debug-level logging. Run **`golc -help`** to see all flags.

 ❗️ GoLC runs on Windows, Linux, and OSX, but the preferred platforms are OSX or Linux.

```bash

If the Results directory exists, GoLC will prompt you to delete it before starting a new analysis and will also offer to save the previous analysis. If you respond 'y', a Saves directory will be created containing a zip file, which will be a compressed version of the Results directory.

$:> golc -devops BitBucket

✅ Using configuration for DevOps platform 'BitBucket'

❗️ Directory <'Results'> already exists. Do you want to delete it? (y/n): y
❗️ Do you want to create a backup of the directory before deleting? (y/n): n


🔎 Analysis of devops platform objects ...

✅ The number of project(s) to analyze is 8

         🟢  Analyse Projet: test2 
          ✅ The number of Repositories found is: 1

        🟢  Analyse Projet: tests 
          ✅ The number of Repository found is: 1
        ✅ Repo: testempty - Number of branches: 1

        🟢  Analyse Projet: sri 
          ✅ The number of Repository found is: 0

        🟢  Analyse Projet: Bitbucket Pipes 
          ✅ The number of Repository found is: 5
        ✅ Repo: sonarcloud-quality-gate - Number of branches: 9
        ✅ Repo: sonarcloud-scan - Number of branches: 8
        ✅ Repo: official-pipes - Number of branches: 14
        ✅ Repo: sonarqube-scan - Number of branches: 7
        ✅ Repo: sonarqube-quality-gate - Number of branches: 2
         ........


✅ The largest repo is <sample-nodejs-project> in the project <SAMPLES> with the branch <demo-app-week> and a size of 425.45 KB

✅ Total size of your organization's repositories: 877.65 KB
✅ Total repositories analyzed: 11 - Find empty : 1

🔎 Analysis of Repos ...

Extracting files from repo : testempty 
        ✅ json report exported to Results/Result_TES_testempty_main.json
        ✅ The repository <testempty> has been analyzed
                                                                                                    
        ✅ json report exported to Results/Result_CLOC_gcloc_DEV.json
        ✅ The repository <gcloc> has been analyzed
                                                                                              
        ✅ json report exported to Results/Result_BBPIPES_sonarcloud-quality-gate_master.json
        ✅ The repository <sonarcloud-quality-gate> has been analyzed
                                                                                              
        ✅ json report exported to Results/Result_BBPIPES_sonarcloud-scan_master.json
        ✅ The repository <sonarcloud-scan> has been analyzed
         ........

🔎 Analyse Report ...

✅ Number of Repository analyzed in Organization <sonar-demo> is 11 
✅ The repository with the largest line of code is in project <CLOC> the repo name is <gcloc> with <2.05M> lines of code
✅ The total sum of lines of code in Organization <sonar-demo> is : 2.06M Lines of Code


✅ Reports are located in the <'Results'> directory
✅ Time elapsed : 00:01:01

ℹ️  To generate and visualize results on a web interface, follow these steps: 

        ✅ run : ResultsAll
$:>        

```

✅ Run on Windows

For execution on Windows, it is preferable to use PowerShell.

```
PS C:\Users\ecadmin\sonar-golc> .\golc.exe -devops File

✅ Using configuration for DevOps platform 'File'
❗️ Directory <'C:\Users\ecadmin\sonar-golc\Results'> already exists. Do you want to delete it? (y/n): y
❗️ Do you want to create a backup of the directory before deleting? (y/n): n

🔎 Analysis of Directories ...
 Extracting files from sonar-golc
OutputName: Result_sonar-golc

        ✅ json report exported to Results\Result_sonar-golc.json
        ✅ 1 The directory <c:\Users\ecadmin\Picktalk> has been analyzed

🔎 Analyse Report ...

✅ Number of Directory analyzed in Organization <test> is 1
✅ The total sum of lines of code in Organization <test> is : 41.48K Lines of Code

✅ Reports are located in the <'Results'> directory

✅ Time elapsed : 00:00:02

ℹ️  To generate and visualize results on a web interface, follow these steps:
        ✅ run : ResultsAll

PS C:\Users\ecadmin\sonar-golc>
```


## Reports

The report files are created in PDF, JSON, and CSV formats for the report by files.

```bash
Results
├── Byfile-report
│   ├── csv-report
│   │   └── Result……_byfile.csv
│   ├── pdf-report
│   │   └── Result……_byfile.pdf
│   └── Result……_byfile.json
└── Bylanguage-report
│   ├── csv-report
│   ├── pdf-report
│   └── Result……_.json
├── GlobalReport.json
├── GlobalReport.pdf
├── GlobalReport.txt
```


To view the results on a web interface, you need to launch the '**ResultsAll**' program.

The '**ResultsAll**' program prompts you if you want to view the results on a web interface.It starts an HTTP service on the default port 8091. If this port is in use, you can choose another port.
To stop the local HTTP service, press the Ctrl+C keys


```bash
$:> ./ResultsAll

✅ Launching web visualization...
❗️ Port 8091 is already in use.
✅ Please enter the port you wish to use :  9090
✅ Server started on http://localhost:9090
✅ please type < Ctrl+C > to stop the server
$:> 
```

From the web interface, you have the option to download the report files in ZIP format.

## Web UI

![webui](imgs/webui.png)

### Report example

![report](imgs/report.png)

Report By file :

![report](imgs/reportbyfiles.png)

## Supported languages

To show all supported languages use the subcommand languages :

 ```
$:> golc.go -languages

Language           | Extensions                               | Single Comments | Multi Line
                    |                                          |                 | Comments
-------------------+------------------------------------------+-----------------+--------------
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
Flex               | .as                                      | //              | /* */ 
Golang             | .go                                      | //              | /* */ 
HTML               | .html, .htm, .cshtml, .vbhtml, .aspx,    |                 | <!-- --> 
                    | .ascx, .rhtml, .erb, .shtml, .shtm, cmp  |                 | <!-- -->
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

 ❗️ To add a new language, you need to add an entry to the Languages structure defined in the file [assets/languages.go](assets/languages.go).


## Execution Log

The application generates a log file named `Logs.log` in the current directory. This log file records all the steps of the GoLC execution process, providing detailed information about the application's runtime behavior.

### Location
The log file is created in directory `Logs`, is placed in the following path:  `<GoLCHome/Logs>`.

### Usage
You can refer to this log file to troubleshoot issues, monitor the application's execution, and understand its internal processes.

❗️ At each execution the file is deleted

### Example Log Entry

 ```
[2024-07-11 17:22:52] INFO ✅ Using configuration for DevOps platform 'Github'

[2024-07-11 17:22:55] INFO 🔎 Analysis of devops platform objects ...

 Repos saved successfully! 
[2024-07-11 17:22:56] INFO        ✅ The number of Repo(s) found is: 50

[2024-07-11 17:22:57] INFO      ✅ 1 Repo: sonar-aws-cicd-tutorial - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:22:59] INFO      ✅ 2 Repo: sonar-golc - Number of branches: 1 - largest Branch: ver1.0.3 
[2024-07-11 17:22:59] INFO      ✅ 3 Repo: jenkins-docker - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:23:00] INFO      ✅ 4 Repo: abapGit - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:23:01] INFO      ✅ 5 Repo: abap2UI5 - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:23:02] INFO      ✅ 6 Repo: abap-cheat-sheets - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:23:03] INFO      ✅ 7 Repo: Container_Architecture - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:23:04] INFO      ✅ 8 Repo: k8s-helm-sq-key - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:23:04] INFO      ✅ 9 Repo: k8s-hpa-sonarqubedce - Number of branches: 1 - largest Branch: main 
[2024-07-11 17:23:05] INFO      ✅ 10 Repo: GitHub-Monorepo-Example - Number of branches: 1 - largest Branch: master 
............
✅ Result saved successfully!

[2024-07-11 17:23:35] INFO ✅ The largest Repository is <sonar-aws-cicd-tutorial> in the organization <XXXXXXXXXXXXXX> with the branch <main> 
[2024-07-11 17:23:35] INFO ✅ Total Repositories that will be analyzed: 49 - Find empty : 1 - Excluded : 0 - Archived : 0
[2024-07-11 17:23:35] INFO ✅ Total Branches that will be analyzed: 49

[2024-07-11 17:23:35] INFO 🔎 Analysis of Repos ...

 Waiting for workers...
[2024-07✅ json report exported to /XXXXXXX/sonar-golc/Results/Result_SonarSource-Demos_employee-api_main.json
[2024-07-11 17:23:36] INFO      ✅ 2 The repository <employee-api> has been analyzed

 Waiting for workers...
[2024-07✅ json report exported to /XXXXXXX/sonar-golc/Results/Result_SonarSource-Demos_jenkins-docker_main.json
[2024-07-11 17:23:36] INFO      ✅ 3 The repository <jenkins-docker> has been analyzed
............

[2024-07-11 17:27:20] INFO 🔎 Analyse Report ...

[2024-07-11 17:27:20] INFO ✅ Number of Repository analyzed in Organization <XXXXXXXXXXXXXX> is 49 
[2024-07-11 17:27:20] INFO ✅ The total sum of lines of code in Organization <XXXXXXXXXXXXXX>  is : 5.62M Lines of Code

[2024-07-11 17:27:20] INFO ✅ Reports are located in the <'Results'> directory
[2024-07-11 17:27:20] INFO ✅ Time elapsed : 00:02:24

[2024-07-11 17:27:20] INFO  ℹ️  To generate and visualize results on a web interface, follow these steps: 
[2024-07-11 17:27:20] INFO      ✅ run : ResultsAll

  ```


## Future Features

We are continuously working to enhance and expand the functionality of our application. Here are some of the upcoming features you can look forward to:

- **Improved Exclusion Patterns**: Enhancements to the exclusion patterns to provide more precise and flexible control over what is included or excluded in various operations.
- **Additional Integrations**: We are exploring support for other platforms and services to broaden the scope of our integrations and offer more flexibility to our users.
- **Improved User Interface**: Enhancements to the user interface to provide a more intuitive and user-friendly experience.
- **Performance Optimizations**: Ongoing efforts to optimize the performance and scalability of the application to handle larger workloads more efficiently.
- **Security Enhancements**: Continued focus on strengthening the security of the application to protect user data and ensure privacy.

Stay tuned for updates as we roll out these new features and improvements!
