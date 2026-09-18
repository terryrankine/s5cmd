# Maintaining this fork

This fork exists to ship fixes upstream (`peak/s5cmd`) has not merged. The goal is to shrink it, not grow it. Most upkeep is automated; this file says what runs by itself, what a person has to do, and the rules the automation assumes.

## Support policy

| What | Promise |
|---|---|
| Runs on | Linux kernel ≥ 3.2 (any distro, any libc — binaries are static), macOS 11+, Windows 10 / Server 2016+; containers for amd64, arm64, ppc64le, riscv64, arm/v6, arm/v7, 386 |
| Built with | the two Go releases Go supports. "current" moves when Go ships a major (Feb/Aug); the `go.mod` floor moves in a minor release, by hand |
| Container base | current Alpine; rebuilt on Alpine EOL or a Go security release |
| Old releases | no backports — the fix for any release is the next release |

## Versioning

| Bump | When | Who |
|---|---|---|
| **patch** `vX.Y.Z+1` | dependency, security and toolchain updates | **automatic** — merging a `dependabot/*` or `maintenance/*` PR tags and publishes; release notes are generated from the commit messages (`fix:`/`feat:`/`deps:` prefixes), so write them well |
| **minor** `vX.Y+1.0` | features, behaviour changes, Go floor bump | you: edit `CHANGELOG.md` (`## Unreleased` → version + date), `git tag -a vX.Y.0`, push the tag |
| **major** | breaking CLI changes | you, same steps |

Tags are unsigned; commits are signed. `master` is protected by a ruleset: changes land only through a PR with green `ci-ok` (the whole build/test/qa matrix) and `codespell` checks, no force-pushes, no bypass — the automation obeys the same rule: it only pushes tags. A tag push runs `goreleaser.yml`: build → **draft** release → smoke test of the uploaded Linux binary with the e2e suite (including the path-traversal guard) → publish with the hand-written CHANGELOG section as notes if one exists, else the generated notes → `docker.yml` pushes `ghcr.io/<owner>/s5cmd:<tag>` and `:latest`.

## What runs by itself

| Workflow | When | Does |
|---|---|---|
| `ci.yml` | every PR and push | build/test on Go floor + current × Linux/macOS/Windows; `qa` lint |
| `maintenance.yml` → `health` | Mondays | full test run on master with today's runners and images |
| `maintenance.yml` → `govulncheck` | Mondays | reachable-vulnerability scan; opens a `maintenance/security-*` PR that bumps deps, or a `needs-human` issue if only a Go bump fixes it |
| `maintenance.yml` → `go-drift` | Mondays | new Go major → PR moving "current" in CI, goreleaser and Dockerfile |
| Dependabot | Mondays / monthly | `aws-sdk-go`, `golang.org/x/*`, Docker base images, Actions, tools module |
| `auto-release.yml` | on merge of `dependabot/*` or `maintenance/*` | waits for green CI on master, tags the next patch; nothing is pushed to master |
| `goreleaser.yml` | on tag | build, smoke, publish |
| `docker.yml` | on publish | multi-arch image to GHCR |

## When you get pinged

Anything the automation cannot finish opens an issue labelled **`needs-human`** (and a failed scheduled run is an email). The cases:

- **health fails** — the world moved (image removed, runner changed). Fix the workflow; that PR is a `maintenance/*` branch, so merging it releases nothing unless code changed.
- **govulncheck needs a Go bump** — bump `go-version` in `ci.yml`/`goreleaser.yml` and `FROM golang:` in `Dockerfile` on a `maintenance/go-security` branch; merging auto-releases a patch.
- **smoke test failed on a tag** — the release stays a draft. Read the run, fix, tag the next version. Never publish the draft by hand without a passing smoke run.
- **auto-release could not tag** — usually a CI failure on master right after a merge. Fix master, then tag by hand.

Merging a maintenance PR is the only routine decision: read the PR body (it says what changed and why), check CI is green, merge. Expect roughly one a fortnight.

## Twice a year (Feb/Aug, when Go ships)

`go-drift` opens the "current" bump for you. Decide the floor yourself: if the release named in `go.mod` (`go 1.24.0`) is no longer supported by Go and nothing you care about builds with it, raise it in a **minor** release and say so in the CHANGELOG. Raising it is a compatibility change for people who build from source; the binaries are unaffected.

## Upstream

- Everything here is submitted upstream as small PRs (see `docs/issue-tracker.md`). When any of them merge, rebase on upstream and drop the duplicate.
- Quarterly: triage new upstream issues against this build and update `docs/issue-tracker.md`.
- Decision point **2027-03**: if upstream is still idle, choose between publishing this fork as the successor (module rename, tap, GHCR namespace) or freezing it with a security-only promise. Put the answer here.

## Local development

- Windows: symlink tests skip without Developer Mode; the Linux suite runs in Docker: `docker run --rm -v "$PWD:/src" -w /src golang:1.27 go test -race ./...`
- `make check` needs `make bootstrap` (tools live in `tools/go.mod`).
- Run the e2e suite against a prebuilt binary: `S5CMD_TEST_BINARY=./s5cmd go test ./e2e/`.
- Commit signing uses an SSH key; pushes of workflow changes need SSH (the `gh` token lacks `workflow` scope).
