#!/bin/bash
# Preview the release notes the Release workflow would write, without releasing anything.
#
#   .github/scripts/preview_release_notes.sh          # next release: origin/main since the latest tag
#   .github/scripts/preview_release_notes.sh V2.2     # an existing release: V2.2 since the tag before it
#
# Runs this checkout's release_notes.py, so prompt changes can be tried before they are
# merged. It checks the chosen commit out in a temporary worktree (removed afterwards)
# and prints the notes; nothing is pushed or published. It calls Claude through Portkey
# with PORTKEY_API_KEY, or the Claude Code credentials in ANTHROPIC_AUTH_TOKEN.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
ref="${1:-origin/main}"
key="${PORTKEY_API_KEY:-${ANTHROPIC_AUTH_TOKEN:-}}"
[ -n "$key" ] || { echo "Set PORTKEY_API_KEY (or ANTHROPIC_AUTH_TOKEN) to your Portkey key." >&2; exit 1; }

git -C "$repo_root" fetch -q --tags origin
commit=$(git -C "$repo_root" rev-parse --verify "$ref^{commit}")

# A tagged commit is previewed as that release, measured from the tag before it.
if tag=$(git -C "$repo_root" describe --tags --exact-match "$commit" 2>/dev/null); then
  version="${tag#V}"
  prev=$(git -C "$repo_root" describe --tags --abbrev=0 "$commit^")
else
  version="next"
  prev=$(git -C "$repo_root" describe --tags --abbrev=0 "$commit")
fi

venv="${XDG_CACHE_HOME:-$HOME/.cache}/golc-release-notes-venv"
if [ ! -x "$venv/bin/python" ]; then
  python3 -m venv "$venv"
fi
"$venv/bin/pip" install --quiet --disable-pip-version-check -r "$repo_root/.github/scripts/requirements.txt"

worktree=$(mktemp -d)
trap 'git -C "$repo_root" worktree remove --force "$worktree" >/dev/null 2>&1 || true' EXIT
git -C "$repo_root" worktree add -q --detach "$worktree" "$commit"

echo "Previewing V$version: $prev..$(git -C "$repo_root" rev-parse --short "$commit")" >&2
cd "$worktree"
PORTKEY_API_KEY="$key" \
GH_TOKEN="${GH_TOKEN:-$(gh auth token)}" \
GITHUB_REPOSITORY=sonar-solutions/sonar-golc \
VERSION="$version" \
PREV_TAG="$prev" \
  "$venv/bin/python" "$repo_root/.github/scripts/release_notes.py"
cat release-notes.md
