# DEV.md — Developer Guide (pixu)

> Internal notes for contributors. This file lives ONLY on the `dev` branch and is
> never pushed to `master` / the remote `xvoland/pixu` — same as `MARKET.md`.

## What is pixu

Terminal image viewer written in Go. Renders images directly in the terminal via
RGB, grayscale, 256-color, ASCII, TGP (Kitty/iTerm2) and Sixel protocols. See
`README.md` (usage) and `AGENTS.md` (architecture, conventions) for the full picture.

## Branches

- **`master`** — the only branch pushed to the remote `xvoland/pixu`. Must stay a
  clean, release-ready line.
- **`dev`** — your local working branch. All day-to-day work happens here.
- **`FIX`** — local scratch branch for experiments.
- `MARKET.md` and `DEV.md` are intentionally **dev-only** and must never reach
  `master` or the remote.

## How to push

Only `master` is pushed to the remote. **Never** run `git push origin dev`.

1. Do your work and commit on `dev`.
2. Move the change(s) into `master` **without** merging `dev` — a plain
   `git merge dev` would also pull in `MARKET.md`/`DEV.md`. Instead:

   ```bash
   git checkout master
   # Option A — a single self-contained commit:
   git cherry-pick <commit-sha>
   # Option B — bring over only specific files:
   git checkout dev -- main.go .github/workflows/build.yml
   git commit -m "feat: ..."
   ```

3. Push master:

   ```bash
   git push origin master
   ```

4. Sync back so `dev` stays ahead of `master`:

   ```bash
   git checkout dev
   git merge master
   ```

> Rule of thumb: `master` is always an ancestor of `dev`; `dev` just carries the
> extra dev-only docs.

### Worked example — you edited a file on `dev`, now publish to GitHub

Say you changed `.gitignore` while on `dev` (uncommitted) and want it on
`master`/GitHub without dragging `MARKET.md`/`DEV.md` along:

```bash
# 1. Commit the work on dev (so it is captured)
git add .gitignore
git commit -m "build: update .gitignore"

# 2. Switch to master and bring ONLY that file over (cherry-pick or file checkout)
git checkout master
git checkout dev -- .gitignore          # pulls just .gitignore from dev
git commit -m "build: update .gitignore"

# 3. Publish to GitHub (only master is ever pushed)
git push origin master

# 4. Return to dev and merge master back in (dev keeps its extra docs)
git checkout dev
git merge master
```

If your change is a single self-contained commit, step 2 can be
`git cherry-pick <commit-sha>` instead of the `git checkout dev -- <file>` form.
Never run `git merge dev` / `git push origin dev` — that would push the
dev-only files to the remote.

## Build & run locally

```bash
# Full versioned build (mirrors CI ldflags)
go build -ldflags "-X main.version=$(git describe --tags --abbrev=0) -X main.buildSource=$(git describe --tags)" -o bin/pixu .

go run . --help
make build-local        # builds ./bin/pixu with the git tag embedded
make dist               # cross-build all 7 platforms into dist/
make dist-windows       # windows/amd64 + windows/arm64
make dist-linux         # linux/amd64 + linux/arm64 + linux/arm
make dist-darwin        # darwin/amd64 + darwin/arm64 (needs macOS + CGo)
```

`dist/` is gitignored (keep the `dist/.gitkeep` marker).

## Versioning & releases

- Versions are semver tags `vX.Y.Z` (currently `1.3.x`).
- **Release flow:** pushing a tag triggers CI (`.github/workflows/build.yml`) to
  build all platforms, create a GitHub Release with the binaries, and update the
  Homebrew tap formula automatically:

  ```bash
  git tag v1.3.6
  git push origin v1.3.6
  ```

- A tag's source tarball **must** contain `qr-code.jpg` — it is embedded via
  `//go:embed` in `main.go`. If you retag, point the tag at a commit where that
  file is already tracked, or the brew build fails with
  `pattern qr-code.jpg: no matching files found`.
- The Homebrew tap auto-update needs the repo secret **`TAP_TOKEN`** (a PAT with
  write access to `xvoland/homebrew-pixu`). Without it the `update-tap` job is a
  no-op (`continue-on-error`) and you must bump the formula by hand.

## Homebrew

- Tap repo: `xvoland/homebrew-pixu` → `Formula/pixu.rb`.
- Install: `brew tap xvoland/pixu && brew install pixu`.
- The formula is rewritten by CI on each release tag; only hand-edit it for
  emergencies.
- On macOS the formula ad-hoc signs the binary (`codesign --sign -`) and strips
  `com.apple.quarantine`, so users no longer need `xattr -dr ...`. CI also
  ad-hoc signs the darwin release binaries.

## CI

`.github/workflows/build.yml`:

- `push` to `master` → builds win/linux (ubuntu-latest) + darwin (macos-latest)
  and uploads artifacts only.
- `push` of a `v*` tag → same build **plus** creates the GitHub Release and
  updates the tap formula.

Actions are pinned to Node 24-compatible majors (`checkout@v7`,
`setup-go@v7`, `upload-artifact@v7`, `download-artifact@v8`).

## GitHub

- **Remotes:** `origin` → `github.com/xvoland/pixu`. Only the `master` branch is
  pushed; `dev`/`FIX` stay local. The default branch of this repo is `master`.
- **Tap repo:** `github.com/xvoland/homebrew-pixu` holds `Formula/pixu.rb`. Its
  default branch is also `master`. The tap is where `brew install pixu` resolves.
- **Releases:** created by CI on a `v*` tag push. GitHub marks the newest
  non-draft release as **Latest** automatically; README's
  `releases/latest/download/...` links depend on that.
- **Secrets (in `xvoland/pixu`):**
  - `TAP_TOKEN` — a PAT (classic with `repo` scope, or fine-grained with write to
    `xvoland/homebrew-pixu`). Used by the `update-tap` job to push the formula.
    Rotate with:
    ```bash
    gh secret set TAP_TOKEN --repo xvoland/pixu --body "<new_pat>"
    ```
  - Verify a token still has write access with a throwaway workflow that clones the
    tap and runs `git push --dry-run` (clone succeeds = read; push succeeds = write).
- **Branch protection:** not enabled — consider protecting `master` (require CI
  green) if the project grows.

## Future / maintenance tasks

- **New release:**
  ```bash
  git tag v1.3.6 && git push origin v1.3.6   # CI builds, releases, updates tap
  ```
- **Bump an action (Node/runner deprecation):** check the current major and update
  the pins in `.github/workflows/build.yml`:
  ```bash
  for a in actions/checkout actions/setup-go actions/upload-artifact actions/download-artifact; do
    echo "$a -> $(gh api repos/$a/releases/latest --jq .tag_name)"
  done
  ```
- **Add a build platform:** extend the `GOOS`/`GOARCH` matrix in the `Makefile`
  `dist*` targets, then the CI already picks them up.
- **`qr-code.jpg` changed:** the embedded file must be present in the tag's source
  tarball. If you replace it, **retag** (`git tag -f vX.Y.Z` at a commit where it
  is tracked and `git push -f origin vX.Y.Z`); otherwise `brew install` fails with
  `pattern qr-code.jpg: no matching files found`.
- **Failed release job:** re-run with `gh run rerun <id>`. The `release` job is
  idempotent — if the release already exists it skips creation rather than erroring.
- **Refresh release binaries manually:** `make dist` then
  `gh release upload vX.Y.Z --clobber dist/pixu-*`.
- **New CLI flag / env var:** add it in `rootCmd` flags + env parsing, document it
  in `README.md` (and the env-var list in `AGENTS.md`), and add a test in
  `main_test.go` if behaviour-changing.
- **New rendering mode:** implement it in `renderAndOutput`, register the mode
  string, and update `AGENTS.md`/`README.md`.
- **Recover divergence:** if `dev` drifts from `master`,
  ```bash
  git checkout master && git merge --ff-only origin/master
  git checkout dev   && git merge master
  ```
- **Tests:** `go test ./...` (CI does not run tests yet — wire up a `test` job if
  wanted).

## Conventions

- Comments and docs in English; no emojis in code.
- Env vars override defaults but not explicit CLI flags:
  **CLI flag > env variable > default**.
  Supported envs: `PIXU_WIDTH`, `PIXU_HEIGHT`, `PIXU_MODE`, `PIXU_INVERT`,
  `PIXU_CHAR`, `PIXU_ROTATE`, `PIXU_ASCII_CHARS`, `PIXU_CELL_WIDTH`,
  `PIXU_CELL_HEIGHT`, `PIXU_SCALE`, `PIXU_DITHER`.
- Clipboard: macOS uses NSPasteboard (CGo/ObjC); Linux/Windows use
  `golang.design/x/clipboard` (X11 only, no Wayland).
- Use `imaging.AutoOrientation(true)` when loading images; validate all user
  input (rotate, width, height) before use.
