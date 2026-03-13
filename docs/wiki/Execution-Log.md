# Execution Log

The application writes a log file named `Logs.log` in the **Logs** directory under the GoLC home path (`<GoLCHome>/Logs`). It records all steps of the GoLC run.

Use this log to troubleshoot issues, monitor execution, and understand internal behavior.

❗️ The log file is deleted at each new execution.

### Example log entries

```
[2024-07-11 17:22:52] INFO ✅ Using configuration for DevOps platform 'Github'
[2024-07-11 17:22:55] INFO 🔎 Analysis of devops platform objects ...
[2024-07-11 17:22:56] INFO        ✅ The number of Repo(s) found is: 50
[2024-07-11 17:22:57] INFO      ✅ 1 Repo: sonar-aws-cicd-tutorial - Number of branches: 1 - largest Branch: main
…
[2024-07-11 17:27:20] INFO ✅ Reports are located in the <'Results'> directory
[2024-07-11 17:27:20] INFO      ✅ run : ResultsAll
```
