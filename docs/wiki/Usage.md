# Usage

## Environment Configuration

Before running GoLC, configure your environment by initializing the various values in the `config.json` file. If using the sources, copy **config_sample.json** to **config.json** and modify the entries.

---

## GitHub.com (Cloud) Basic Configuration

Specify the following parameters in the config.json file:

```json
"Github": {
  "Users": "xxxxxxxxxxxxxx",
  "AccessToken": "xxxxxxxxxxxxxx",
  "Organization": "xxxxxx"
}
```

Save the config.json file and [Run GoLC](#run-golc) below.

---

## GitHub Enterprise Server (on-premises) Basic Configuration

```json
"GithubEnterprise": {
  "Users": "xxxxxxxxxxxxxx",
  "AccessToken": "xxxxxxxxxxxxxx",
  "Organization": "xxxxxx",
  "Url": "https://github.yourcompany.com/",
  "Baseapi": "github.yourcompany.com",
  "Protocol": "https"
}
```

Save the config.json file and [Run GoLC](#run-golc).

---

## GitLab (Cloud and On-premises) Basic Configuration

```json
"Gitlab": {
  "Users": "xxxxxxxxxxxxxx",
  "AccessToken": "xxxxxxxxxxxxxx",
  "Organization": "xxxxxx"
}
```

You can specify multiple groups with a comma-separated list in `Organization`, e.g. `"Organization": "group1,group2"`.

For **GitLab Self-Managed (on-premises)**, also add:

```json
"Gitlab": {
  "Url": "https://gitlab.yourcompany.com/",
  "Protocol": "https"
}
```

Save the config.json file and [Run GoLC](#run-golc).

---

## Bitbucket Cloud Basic Configuration

```json
"BitBucket": {
  "Users": "your.email@example.com",
  "AccessToken": "ATATT3x...",
  "Workspace": "your-workspace-slug",
  "Organization": "your-workspace-slug",
  "Project": "your-project-slug1,your-project-slug2"
}
```

### Token Requirements

**Important:** Bitbucket Cloud requires **API Tokens** (not App Passwords). App Passwords have been deprecated as of June 2025.

Grant these scopes: **Repositories: Read**, **Projects: Read**, **Account: Read**. The token starts with `ATATT3x...`.

#### Finding Your Workspace

- If your Bitbucket URL is `https://bitbucket.org/your-workspace/`, then `your-workspace` is your workspace slug.
- You can also find it in workspace settings.

#### Example

```json
"BitBucket": {
  "Users": "john.doe@example.com",
  "AccessToken": "ATATT3x...your-token-here",
  "Workspace": "my-workspace",
  "Organization": "my-workspace"
}
```

❗️ **Workspace** is required. **Organization** is for reporting (usually same as workspace). **Users** must be your **email address**. API tokens are required.

Save the config.json file and [Run GoLC](#run-golc).

---

## Bitbucket Data Center (on-premises) Basic Configuration

```json
"BitBucketSRV": {
  "Users": "xxxxxxxxxxxxxx",
  "AccessToken": "xxxxxxxxxxxxxx",
  "Organization": "xxxxxx",
  "Url": "https://bitbucket.yourcompany.com/",
  "Protocol": "https"
}
```

Save the config.json file and [Run GoLC](#run-golc).

---

## Azure DevOps Services (Cloud) Basic Configuration

```json
"Azure": {
  "Users": "xxxxxxxxxxxxxx",
  "AccessToken": "xxxxxxxxxxxxxx",
  "Organization": "xxxxxx"
}
```

Save the config.json file and [Run GoLC](#run-golc).

---

## File Mode Basic Configuration

For **File** mode, create a **.cloc_file_load** file and add the directories to analyze, one per line. If provided, it overrides the **Directory** parameter.

---

## Optional Parameters

❗️ **Period**, **Factor**, **Stats** — Do not modify; reserved for future use.

❗️ **Multithreading** / **Workers** — Enable parallel analysis. Set **Multithreading** to `false` to disable. **Workers** is the number of concurrent analyses.

❗️ **DefaultBranch** — If `true`, only the default branch of each repo is analyzed. If `false`, all branches are considered to pick the largest.

❗️ **ExtExclusion** — Exclude files by extension, e.g. `"ExtExclusion": [".css", ".js"]`.

❗️ **ResultByFile** — If `true`, you get per-file JSON in Results and can run **ResultByfiles** for a PDF in Results/reports.

❗️ **Branch** — Restrict to a specific branch for all repos, e.g. `"Branch": "main"`.

❗️ **FileExclusion** — Use a **.cloc_'platform'_ignore** file to ignore repos/projects. Example: `"FileExclusion": ".cloc_bitbucketdc_ignore"`.

**BitBucket ignore syntax:** `REPO_SLUG`, `PROJECT_KEY`, or `PROJECT_KEY/REPO_SLUG` per line.

**GitHub ignore syntax:** One repo slug per line.

**File mode:** `DIRECTORY_NAME` or `FILE_NAME` per line.

**Azure DevOps:** `PROJECT_KEY/REPO_SLUG` or `PROJECT_KEY` per line.

❗️ **ResultAll** — Default report format (by language and by file). Set **ResultAll** to `true` in config.json.

❗️ **Org** (GitHub only) — If `true`, run on an organization; if `false`, on a user account. **Organization** should be your personal account when `false`.

❗️ **ExcludePaths** — Exclude directories, e.g. `"ExcludePaths": ["test1", "pkg/test2"]`.

❗️ **Projects** / **Repos** — If empty, all repositories are analyzed. Set **Projects** (BitBucket/Azure) or **Repos** to limit scope.

---

## Run GoLC

Launch GoLC by specifying your DevOps platform. Supported `-devops` values:

`BitBucketSRV` | `BitBucket` | `Github` | `GithubEnterprise` | `Gitlab` | `Azure` | `File`

```bash
golc -devops BitBucket
```

❗️ GoLC runs on Windows, Linux, and OSX; preferred platforms are OSX or Linux.

If the **Results** directory already exists, GoLC will ask whether to delete it and whether to back it up (creates a **Saves** directory with a zip).

**Windows:** Prefer PowerShell; run e.g. `.\golc.exe -devops File`.

After the run, reports are in the **Results** directory. To view them in a browser, run **ResultsAll** (see [Reports](Reports)).
