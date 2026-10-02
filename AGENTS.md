# GoLC — notes for code reviewers and AI agents

## How GoLC is used

GoLC is a **plug-and-play, one-time tool**. A user downloads a release (or pulls the
container image), runs it once against their DevOps platform or a local folder, takes the
report it produces, and is done. Nobody installs it as a service or keeps it running, and
almost nobody runs it again.

Review and design against that, not against a long-lived application:

- **No upgrade paths.** Users don't carry state from one version to the next. Every
  release ships a fresh `config.json`, and the container starts from the one in the image.
  Don't raise findings about migrating config files, persisted volumes or saved settings
  across versions, and don't add migration code for them.
- **Don't keep state between runs.** Results only need to last until the user has
  downloaded their reports from the dashboard. Don't recommend persistent storage, named
  volumes or anything else that keeps data after the run. `config.json` holds the user's
  access tokens, so data that disappears with the run is the safer default.
- **The report is the product.** Counting accuracy, report correctness and a first run
  that just works matter most. Operational concerns from long-running services (uptime,
  rolling upgrades, schema evolution, data retention) don't apply.

## Release notes

The Release workflow writes each release's notes with `.github/scripts/release_notes.py`. To
see what it would write without releasing, run `.github/scripts/preview_release_notes.sh`
(the next release from `origin/main`) or `.github/scripts/preview_release_notes.sh V2.2` (an
existing release). It uses this checkout's script, so prompt changes can be tried before
they are merged, and it never pushes or publishes anything.
