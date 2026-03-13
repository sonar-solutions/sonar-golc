# Wiki source files

The markdown files in this folder are the **source for the [GitHub Wiki](https://github.com/sonar-solutions/sonar-golc/wiki)** of the sonar-golc repository.

- **Home.md** → wiki home page (index).
- Other `.md` files (e.g. **Introduction.md**, **Docker.md**) → one wiki page per file. GitHub shows them with the filename as the page title (e.g. `Web-UI`, `Supported-languages`).

---

## How to push everything (repo + native GitHub Wiki)

### 1. Push the repo changes (README, docs/wiki)

From the **sonar-golc** repo root (e.g. on branch `57-improve-readme-and-documentation`):

```bash
cd /path/to/sonar-golc
git add README.md docs/wiki/
git status   # confirm: README.md + docs/wiki/*.md
git commit -m "Improve README (centered logo, structure) and add wiki source under docs/wiki"
git push origin 57-improve-readme-and-documentation
```

Open a PR so your teammate can review. After merge, continue with step 2 when you want the Wiki to go live.

### 2. Push to GitHub’s native Wiki

GitHub’s Wiki is a separate git repo. You publish it by cloning that repo, copying these markdown files into it, and pushing.

1. **Enable the Wiki** (if not already):
   - Repo → **Settings** → **Features** → check **Wiki** → Save.

2. **Clone the wiki repo** (use the wiki URL, not the main repo):

   ```bash
   git clone https://github.com/sonar-solutions/sonar-golc.wiki.git
   cd sonar-golc.wiki
   ```

3. **Copy wiki source into the clone**  
   From the **sonar-golc** repo root (adjust paths if needed):

   ```bash
   # From sonar-golc repo root
   cp docs/wiki/Home.md    /path/to/sonar-golc.wiki/
   cp docs/wiki/Introduction.md /path/to/sonar-golc.wiki/
   cp docs/wiki/Installation.md /path/to/sonar-golc.wiki/
   cp docs/wiki/Docker.md       /path/to/sonar-golc.wiki/
   cp docs/wiki/Prerequisites.md /path/to/sonar-golc.wiki/
   cp docs/wiki/Usage.md        /path/to/sonar-golc.wiki/
   cp docs/wiki/Reports.md     /path/to/sonar-golc.wiki/
   cp docs/wiki/Web-UI.md      /path/to/sonar-golc.wiki/
   cp docs/wiki/Supported-languages.md /path/to/sonar-golc.wiki/
   cp docs/wiki/Execution-Log.md       /path/to/sonar-golc.wiki/
   cp docs/wiki/Future-Features.md     /path/to/sonar-golc.wiki/
   ```

   Or, in one go (from repo root, with wiki clone as sibling):

   ```bash
   cp docs/wiki/*.md ../sonar-golc.wiki/
   # Remove the wiki’s README if you don’t want it as a page (optional)
   # rm ../sonar-golc.wiki/README.md
   ```

4. **Commit and push the wiki**:

   ```bash
   cd /path/to/sonar-golc.wiki
   git add *.md
   git status
   git commit -m "Add documentation pages from docs/wiki"
   git push origin main
   ```

   (If the wiki repo uses `master` as default branch, use `git push origin master`.)

After the push, the [Wiki](https://github.com/sonar-solutions/sonar-golc/wiki) will show the new pages. **Home.md** becomes the wiki’s landing page.
