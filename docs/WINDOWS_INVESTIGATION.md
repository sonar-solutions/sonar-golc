# Investigation: Windows local mode path handling failure

## Symptom

On Windows, local mode analysis was reported as "not willing to follow either relative or absolute path". The same configuration worked on Linux/macOS.

## Root causes

### 1. Case-sensitive path comparison

In `buildGitignoreFunc` (pkg/goloc) and in the analyzer's `canAdd` (pkg/analyzer), paths were compared with `strings.HasPrefix` and `strings.TrimPrefix`. On Windows the filesystem is case-insensitive (`C:\Repo` and `c:\repo` are the same), but Go string comparison is case-sensitive. So:

- `findGitRoot` could return `C:\repo` (from `filepath.Abs`),
- while `filepath.Walk` could pass `c:\repo\src\file.go`,
- and `strings.HasPrefix("c:\repo\...", "C:\repo")` is **false**, so the path was treated as outside the repo and .gitignore / exclusion logic broke.

### 2. Relative vs absolute in the ignore callback

The callback passed to the analyzer receives the same path style as the walk root. If the root was relative (e.g. `.\myproject`), paths in the callback could be relative while `repoRoot` from `findGitRoot` was absolute (it uses `filepath.Abs`). Comparing relative and absolute strings then failed.

### 3. Exclude-path prefix check in the analyzer

In `canAdd`, `strings.HasPrefix(path, pathToExclude)` was used. Again, case and slash differences on Windows could make exclusions not apply even when the file was under the excluded directory.

### 4. Local path not resolved before the getter

Relative paths (e.g. from config) were passed to the getter as-is. On Windows, resolving them to absolute form first makes behaviour consistent and avoids "path not followed" issues.

### 5. Getter and Windows drive paths

For a path like `C:\repo`, the hashicorp go-getter might not always treat it as a local path. Converting such paths to a `file:///` URL on Windows makes the getter reliably accept them.

## Fixes applied

### pkg/goloc/goloc.go

- **`pathUnderRoot`**  
  New helper that resolves both paths to absolute, normalizes with `filepath.Clean` and `filepath.ToSlash`, and on Windows compares case-insensitively (lowercasing normalized paths). Used by `buildGitignoreFunc` so .gitignore matching works for any path casing and relative/absolute mix on Windows.

- **`buildGitignoreFunc`**  
  Now uses `pathUnderRoot` instead of raw `HasPrefix`/`TrimPrefix`.

- **`getRepoPath`**  
  For local mode (no branch), resolves `params.Path` with `filepath.Abs` before calling the getter so the getter and downstream code see a single, absolute path form.

### pkg/analyzer/analyzer.go

- **`pathHasPrefix`**  
  New helper that normalizes both sides with `filepath.Clean` and `filepath.ToSlash`, and on Windows uses case-insensitive comparison before checking the prefix and the trailing `/`.

- **`canAdd`**  
  Uses `pathHasPrefix(path, pathToExclude)` instead of `strings.HasPrefix(path, pathToExclude)` so excluded directories are detected correctly on Windows regardless of case or slash style.

### pkg/getter/getter.go

- **`toFileURL`**  
  On Windows, converts an absolute path like `C:\repo` to `file:///C:/repo`.

- **`Getter`**  
  On Windows, when `src` looks like a local drive path (e.g. `X:\...`) and is not already a `file://` URL, it is converted with `toFileURL` before calling the getter client so the getter consistently accepts local paths.

## Testing

- All existing tests in `pkg/goloc` (including `TestBuildGitignoreFunc_*` and `TestFindGitRoot*`) pass.
- Project builds successfully with `go build ./...`.

## Result

Local analysis should now correctly follow both relative and absolute paths on Windows and respect .gitignore and exclusions regardless of path casing.
