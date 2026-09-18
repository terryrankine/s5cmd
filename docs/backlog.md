# Backlog — s5cmd fork → upstream

Updated: 2026-09-17. Owner: terryrankine.

## State of play

- Fork `master` is 57 commits ahead of `peak/s5cmd`, 0 behind. All bundled in upstream PR #859 (March 2026): CI green, 1 community approval, zero maintainer review in 6 months.
- Upstream master last commit: June 2025. But every open PR was touched on 2026-09-11 — someone is triaging.
- New upstream PRs since March that overlap ours: #860–#864 (gustcol, small and well-scoped), #871 (SDK bump), #878 (lyda, mega dep update — CI failing).
- Lesson: small, single-purpose PRs with tests are what gets reviewed. #859 is too big to land.

## Review findings on fork master (2026-09-17)

Full suite passes (Go 1.24 Linux, `-race`). `go vet`, `staticcheck 2026.2.1`, `unparam` clean.

| ID | Severity | Where | Finding |
|----|----------|-------|---------|
| B1 | **High** | `storage/fs.go:221,240` | Dry-run returns `os.NewFile(0, os.DevNull)`. fd 0 is stdin on Unix. `cp.go:687` calls `file.Close()` on it → closes stdin. `s5cmd run --dry-run < cmds` stops reading after the first download. Fix: `os.OpenFile(os.DevNull, os.O_WRONLY, 0)`. |
| B2 | Medium | `log/log.go WithLogFile` | Open error is swallowed; logs silently go to stdout. `--log-file /bad/path` should fail. |
| B3 | Medium | `command/cp.go:889 shouldOverride` | Destination client built from `c.storageOpts`, which carries the *source* region. `--no-clobber`/`--if-*` with `--destination-region` HEADs the wrong region. #839 is not actually fixed for `cp`. gustcol's #862 fixes it. |
| B4 | Low | `storage/s3.go multipartCopy` | Always merges source metadata into the new object, ignoring `--metadata-directive REPLACE`. Also drops source `SSEKMSKeyId`. |
| B5 | Low | `command/rm.go:186` | DIROBJ fix relaxes the dir-skip for all remote objects. Validation still rejects `s3://b/dir/` (IsPrefix) so the fix is unreachable without `--raw`; and no e2e test. gustcol's #861 is the cleaner fix. |
| B6 | Low | `progressbar/progressbar.go` | `formatBytes` uses 1000-based `kMGTPE`; `strutil.HumanizeBytes` uses 1024-based `K/M/G`. Two conventions in one binary. `Finish()` prints "Transfer complete" even for zero-byte runs. |
| B7 | Info | `storage/s3.go ShouldRetry` | `sessionCache.clear()` doesn't affect the in-flight request; the SDK's `AfterRetryHandler` → `Credentials.Expire()` is what actually refreshes. Harmless, comment is misleading. Static creds now retry N times before failing (slower fail). |
| B8 | Info | `docs/issue-tracker.md` | Stale: #815, #817, #834/#707 moved to fixed after it was written. |

## Backlog

Each item = one branch off `upstream/master` = one upstream PR. Reviewed and tested (Linux, `-race`) before push. Order is by value ÷ size.

### Tier 1 — original fixes, no upstream overlap

| # | PR | Commits to carry | Extra work |
|---|----|------------------|------------|
| P1 | fix: dry-run returns a safe file handle | `e9d08ac`, `7e82d00` | Fix B1. Add test that Close() doesn't touch fd 0. |
| P2 | fix: nil guards and error-message correctness | `1e41921`, `e6fb1ba`, `ad3e90a`, `70e9377`, `65c80ab`, `2524fc9`, `223b8f9` | Split if reviewer asks. |
| P3 | fix: data races in progress bar and stats | `64e54f4`, `a2a4333`, `9c08554`(stats test) | Add `-race` tests that fail without the fix. |
| P4 | fix: replace os.Exit/panic with cancellation in cp and sync | `3658f54`, `aedd1cc`, `2589055`, `6c7b161` | Overlaps #698 (grmrgecko, 2024). Reference it. |
| P5 | fix: sync --delete respects --exclude/--include (#815) | `1002ca2` | Needs an e2e test (none today). |
| P6 | fix: humanize bytes suffix (#817) | `de03428` | Tiny. |
| P7 | fix: log.Close idempotent; %w wrapping | `40228af`, `ecff4c6` | Tiny. |

### Tier 2 — fork-only fixes (do first in fork, then upstream)

| # | Item |
|---|------|
| F1 | Fix B2 (`--log-file` error) — then comment on #723. |
| F2 | Adopt gustcol #862 (B3) into fork. |
| F3 | Fix B4 in `multipartCopy` — then comment on #856. |
| F4 | Replace fork's rm DIROBJ change with #861's approach (B5), add e2e test. |
| F5 | Refresh `docs/issue-tracker.md` (B8). |
| F6 | Cut fork release v2.4.1 with the above. |

### Tier 3 — overlaps an open community PR (don't duplicate; comment with test results)

| Ours | Theirs | Action |
|------|--------|--------|
| `9aee5e5` multipart copy | #856 l1n | Comment: fork has run it in prod since March (AntonOks). Note B4. |
| `91f45bd` expired token retry | #683 MqllR | Compare; comment. |
| `f8e8ed1` extsort Generic API, `9080e28` Go 1.24 | #841, #848, #818, #878 | Wait for #878 outcome. |
| goreleaser v2 (`d3f0914`, `3b9147f`) | #585 | Comment with our config. |
| `e39b82a` codespell | #701 | Already theirs. |
| `8446d78` CI actions | #780 | Already theirs. |

### Tier 4 — open upstream bugs nobody has touched (candidates for new work)

| Issue | Title | Size |
|-------|-------|------|
| #718 | `--no-clobber` with cp works wrong | S |
| #649 | sync `--stat` wrong rm count | S |
| #755 | ls yields both relative and absolute paths | S |
| #744 | umask not honoured | S |
| #826 | cp fails with chmod not permitted | S |
| #845 | cp timezone issue | M |
| #852 | sync --delete not deleting removed files | M |
| #720 | strange sync behaviour | M |
| #745 | OOM on large sync | L |
| #751 | sync list incomplete with UTF error | M |

## Status 2026-09-17

Tier 1 done and submitted upstream. Each PR: lint (gofmt, vet, staticcheck, unparam, semgrep), check-codegen, check-gomod, Linux Go 1.24 `-race` suite — all clean. Only `TestAppProxy` fails in Docker, identically on pristine upstream (`localhost.` doesn't resolve in the container).

| PR | Upstream | Branch |
|----|----------|--------|
| P1 dry-run safe file handle | [#879](https://github.com/peak/s5cmd/pull/879) | `fix/dry-run-file-handle` |
| P2 nil guards | [#880](https://github.com/peak/s5cmd/pull/880) | `fix/nil-guards` |
| P3 data races | [#881](https://github.com/peak/s5cmd/pull/881) | `fix/data-races` |
| P4 os.Exit → cancellation | [#882](https://github.com/peak/s5cmd/pull/882) | `fix/cp-sync-error-handling` |
| P5 sync --delete filters | [#883](https://github.com/peak/s5cmd/pull/883) | `fix/sync-delete-filters` |
| P6 humanize B suffix | [#884](https://github.com/peak/s5cmd/pull/884) | `fix/humanize-bytes-suffix` |
| P7 %w wrapping | [#885](https://github.com/peak/s5cmd/pull/885) | `fix/s3-error-wrapping` |

Fork master now carries P1, P5, F1, F2, F3 (see CHANGELOG *Unreleased*). Still open: F4 (rm DIROBJ via #861), F6 (v2.4.1), Tier 3 comments, Tier 4.

New findings while building the PRs:

- **B1 confirmed empirically.** With v2.4.0, `s5cmd --dry-run run < 200 cp lines` hangs forever (fd 0 closed). e2e test `TestRunDryRunDownloadFromStdin` catches it.
- **B9 (High)** — the #815 fix in v2.4.0 was incomplete. `sync --delete` passed `--exclude/--include` through to the generated `rm --raw`, and raw URLs have an empty prefix, so `rm` re-matched against the full key. `folder/*` never matched; `--include` deleted nothing. Fixed on master.
- The copy side of sync has the same raw-prefix limitation (`cp --raw --exclude "sub/*"` matches full path). Separate issue; not fixed.
- extsort v1.0.2 sends a nil error on context cancel; upstream's `printError(nil)` would nil-deref. P4 guards it.
- #707/#834: fork commit `4e631d3` is dead code — `validateRMCommand` rejects `s3://b/dir/` first. Adopt #861.

## Runner and test review (2026-09-17, fork PR #1)

Fixed: goreleaser pushing to `peak/homebrew-tap` (every v2.4.0 run failed after upload); docker pushing to Docker Hub with upstream's secrets (now GHCR on forks); `toolchain go1.25.5` making the whole matrix run 1.25.5; GCS test gate that could never fire; frozen bitnami MinIO image; `TestAppProxy` leaking `http_proxy` into every later test; nil-deref in the test proxy.

Assumptions and edge cases checked:

- **Go matrix.** Sept 2026: current is 1.27, floor is 1.24. Test both. `qa` is pinned to 1.25 because staticcheck is vendored via `internal/tools`: v0.6.x is the last line that keeps `go 1.24` in go.mod (v0.8 forces `go 1.26`) and it can't read Go ≥1.26 export data. To lint on current Go, move tool installs out of go.mod.
- **GHCR visibility.** First push to `ghcr.io/terryrankine/s5cmd` creates a *private* package. Make it public once in package settings or nobody can pull it.
- **Release chain.** goreleaser creates a *draft* release; docker runs on `release: published`, i.e. only after someone publishes the draft. goreleaser does not `need` the GCS job, so a GCS failure never blocks a release (pre-existing).
- **MinIO.** Pinned `quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z`; `minio/minio` on Docker Hub is gone. Only the 3 `select` tests use it. The test job on ubuntu cannot silently skip them (env is set), so a pass means they ran.
- **`t.Setenv`** panics if the test is parallel — TestAppProxy and its subtests are sequential; adding `t.Parallel()` there would fail loudly rather than leak again.
- **`localhost.`** Go bypasses proxies for `localhost` and loopback IPs; the trailing dot defeats that. Resolvers that don't handle it (Docker) now skip. If it resolves to `::1` first, the dialer falls back to 127.0.0.1.
- **P1 dry-run + mv.** All mutating storage ops (`Copy`, `Put`, `doDelete` for `Delete`/`MultiDelete`, fs `Delete`/`Rename`/`MkdirAll`) have dry-run guards; the new `/dev/null` handle is only written by nothing and closed once.
- **P2 `--if-size-differ` + `--if-source-newer`.** The newer check overrides the size check (last check wins). Pre-existing, unchanged.
- **P4 cancellation.** `defer cancel()` in sync fires after `NewRun` returns, by which point every channel is drained, so nothing leaks on the happy path. On the early-error path only the `NewClient` failures return, before any goroutine starts. `shouldStopSync` on `RequestError` is *safer* with `--delete`: a partial source listing plus delete would remove destination objects that exist in the source.
- **P5 local destination.** `sync --delete --exclude` S3→local was untested; added `TestSyncS3BucketToLocalWithDeleteAndExcludeFilter` (fails on v2.4.0, passes now). The *copy* side still has the raw-prefix limitation: `--exclude "sub/*"` does not stop `sub/new.log` being uploaded. Separate fix.
- **Dockerfile.** `golang:1.24-alpine` was an EOL toolchain for the release image; now 1.27. `alpine:3.20` reaches EOL Nov 2026 — bump before then.
- **Windows dev.** 9 symlink e2e tests fail without Developer Mode; CI's Windows runners have the privilege. Could `t.Skip` on `ERROR_PRIVILEGE_NOT_HELD`.

## Plan from 2026-09-18: review and fix everything left, in order

Loop per item: worktree → reproducing test first → fix → line-by-line review → full suite (Linux `-race` + Windows) → merge. Releases batch: **v2.5.0** after items 2–11, **v2.6.0** after the features.

| # | Item | State |
|---|---|---|
| 1 | Quarterly upstream triage (workflow; AI routine needs GitHub connected to the Claude account) | PR #33 |
| 2 | `Waiter` collects errors internally; per-error handler replaces the drain goroutines | this PR |
| 3 | `--exit-on-error` → stop on first cp/rm failure | next |
| 4 | Fork Homebrew tap | |
| 5 | #745 OOM on large sync | |
| 6 | #751 sync listing incomplete with UTF-8 keys | |
| 7 | #720 strange sync behaviour | |
| 8 | #845 cp timezone | |
| 9 | #517 keys ending in `/` | |
| 10 | #810 "no match found" for local destination | |
| 11 | #800 / #749 symlink download errors | |
| 12+ | features: #532/#350 preserve timestamps/permissions, #433 bandwidth limit, #528 resume, #561 hash sync (PR #799), #700 per-side endpoints, #808 SSE-C, #803 tagging, #697 dry-run marker, #796 nothing-to-sync line | v2.6.0 |

"Not a bug, with tests" is an acceptable outcome for 5–11 (two of the earlier batch were).

Product decisions made along the way are in [design-decisions.md](design-decisions.md); its "Open" section lists what probing found and nobody has decided yet.
