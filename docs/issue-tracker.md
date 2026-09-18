# Issue Tracker — peak/s5cmd Open Issues vs fix/apply-upstream-prs Branch

Generated: 2026-03-18. Updated: 2026-09-17.
Branch: `master` on `terryrankine/s5cmd` (released as v2.4.0; fixes below marked *unreleased* are on `master` after v2.4.0)
Upstream: everything was bundled in [#859](https://github.com/peak/s5cmd/pull/859) (no maintainer review in 6 months). Now being re-submitted as small PRs: [#879](https://github.com/peak/s5cmd/pull/879)–[#885](https://github.com/peak/s5cmd/pull/885). Work list: `docs/backlog.md`.

## FIXED — Issues resolved by our branch

| Issue | Title | Fix | Comment to Post |
|-------|-------|-----|-----------------|
| [#29](https://github.com/peak/s5cmd/issues/29) | EntityTooLarge: fail to cp 6.3GB between buckets | `9aee5e5` feat: automatic multipart copy >5 GiB | Fixed in our fork — objects >5 GiB now automatically use multipart copy (CreateMultipartUpload + UploadPartCopy). See commit `9aee5e5`. |
| [#521](https://github.com/peak/s5cmd/issues/521) | sync executes erroneous cp commands with certain characters | `a359daf` fix: shellquote for special chars | Fixed in our fork — replaced `fmt.Sprintf("%q")` with `shellquote.Join()` for proper POSIX shell quoting of object keys. See commit `a359daf`. |
| [#526](https://github.com/peak/s5cmd/issues/526) | Frequent signature expiry during sync of large files | `91f45bd` fix: retry expired session token | Fixed in our fork — ExpiredToken/ExpiredTokenException now triggers session cache clear + retry instead of failing. See commit `91f45bd`. |
| [#571](https://github.com/peak/s5cmd/issues/571) | Assume role profile doesn't work | `edb59f9` fix: respect --profile flag | Fixed in our fork — `--profile` now uses SDK shared config resolution (SSO, assume-role) instead of forcing NewSharedCredentials. See commit `edb59f9`. |
| [#678](https://github.com/peak/s5cmd/issues/678) | ExpiredToken error when cp large dataset | `91f45bd` fix: retry expired session token | Fixed in our fork — expired tokens now auto-refresh and retry. See commit `91f45bd`. |
| [#709](https://github.com/peak/s5cmd/issues/709) | Region setting ignored in credentials file | `edb59f9` + `8078699` | Fixed in our fork — `--profile` respects shared config, and sync now applies source/destination region overrides. See commits `edb59f9`, `8078699`. |
| [#715](https://github.com/peak/s5cmd/issues/715) | remote-to-remote copy --if-source-newer uses wrong region | `8078699` fix: sync region overrides | Fixed in our fork — sync now creates separate storage clients with source/destination region overrides. See commit `8078699`. |
| [#728](https://github.com/peak/s5cmd/issues/728) | invalid argument ERROR syncing files with certain characters | `a359daf` fix: shellquote | Fixed in our fork — same as #521. See commit `a359daf`. |
| [#753](https://github.com/peak/s5cmd/issues/753) | Option to show progress in non-interactive terminals | `f41bf88` feat: --log-progress | Fixed in our fork — new `--log-progress` (`-lp`) flag outputs progress to stderr in pipe-friendly format. See commit `f41bf88`. |
| [#775](https://github.com/peak/s5cmd/issues/775) | Non regular files break cp even if excluded | `d04203c` fix: filter before reject | Fixed in our fork — IsRegular() check moved after exclude/include filters. See commit `d04203c`. |
| [#792](https://github.com/peak/s5cmd/issues/792) | Unnecessary headObject calls when progress bar disabled | `f2ef696` perf: skip stat | Fixed in our fork — stat call skipped when progress bar is NoOp. See commit `f2ef696`. |
| [#794](https://github.com/peak/s5cmd/issues/794) | Add extra virtual host style endpoints support | `afa3158` feat: --addressing-style | Fixed in our fork — new `--addressing-style` flag (path/virtual) + `S3_ADDRESSING_STYLE` env var. See commit `afa3158`. |
| [#816](https://github.com/peak/s5cmd/issues/816) | sync not respecting region flags | `8078699` fix: sync region overrides | Fixed in our fork — sync now applies `--source-region`/`--destination-region`. See commit `8078699`. |
| [#820](https://github.com/peak/s5cmd/issues/820) | Package stdlib-go1.22.10 has critical Vulnerability | `9080e28` Go 1.24 upgrade | Fixed in our fork — upgraded to Go 1.24 which resolves all Go 1.22 CVEs. See commit `9080e28`. |
| [#824](https://github.com/peak/s5cmd/issues/824) | sync silently fails in wrong region | `8078699` + `2589055` | Fixed in our fork — sync region overrides + improved error handling. See commits `8078699`, `2589055`. |
| [#827](https://github.com/peak/s5cmd/issues/827) | --exclude does not work on file-names as expected | `d04203c` fix: filter order | Fixed in our fork — exclude/include filters now checked before non-regular file rejection. See commit `d04203c`. |
| [#835](https://github.com/peak/s5cmd/issues/835) | Cannot upgrade build later than Golang 1.22.x | `9080e28` Go 1.24 upgrade | Fixed in our fork — bumped go.mod to Go 1.24, extsort to v1.4.2. See commit `9080e28`. |
| [#839](https://github.com/peak/s5cmd/issues/839) | shouldOverride should match SetRegion logic | `d53ea47` + `8078699` + *unreleased* dst-region fix | Fixed in our fork — nil deref guard, sync region overrides, and (after v2.4.0) shouldOverride now HEADs the destination with `--destination-region`. Same as upstream [#862](https://github.com/peak/s5cmd/pull/862). |
| [#815](https://github.com/peak/s5cmd/issues/815) | sync --delete does not respect --exclude | *unreleased*, upstream [#883](https://github.com/peak/s5cmd/pull/883) | Fixed in our fork — filters applied relative to the destination prefix in planRun, and no longer re-applied by the generated `rm --raw`. Two e2e tests. |
| [#817](https://github.com/peak/s5cmd/issues/817) | Humanize ls no suffix for bytes | `de03428`, upstream [#884](https://github.com/peak/s5cmd/pull/884) | Fixed in our fork — sizes below 1K now print as `512B`. |

## IMPROVED — Issues partially addressed or likely improved

| Issue | Title | Relevant Fix | Notes |
|-------|-------|-------------|-------|
| [#400](https://github.com/peak/s5cmd/issues/400) | runtime: exceeds 10000-thread limit | `3658f54` os.Exit→cancel | Graceful shutdown via context cancellation reduces thread pressure |
| [#519](https://github.com/peak/s5cmd/issues/519) | Stop using AWS_REGION for source bucket region | `8078699` sync region overrides | Source/dest region overrides now applied independently |
| [#520](https://github.com/peak/s5cmd/issues/520) | ls fails with BucketRegionError | `edb59f9` + `8078699` | Profile + region handling improvements |
| [#554](https://github.com/peak/s5cmd/issues/554) | Retries failed when SDK can't send request | `91f45bd` + `6138e7e` | Retry logic improved + AWS SDK bump |
| [#575](https://github.com/peak/s5cmd/issues/575) | Access Key/Secret should not be mandatory | `edb59f9` profile fix | Improved credential chain handling |
| [#615](https://github.com/peak/s5cmd/issues/615) | Correct exit codes for SIGINT signals | `3658f54` + `aedd1cc` | os.Exit replaced with context cancellation |
| [#660](https://github.com/peak/s5cmd/issues/660) | sync: NoSuchKey for file with space | `a359daf` shellquote | Special character handling fixed |
| [#674](https://github.com/peak/s5cmd/issues/674) | i/o timeout in sync | `2589055` sync error handling | Better error propagation and cancellation |
| [#677](https://github.com/peak/s5cmd/issues/677) | SerializationError: failed to decode REST XML | `6138e7e` AWS SDK bump | SDK v1.55.5 may fix XML parsing |
| [#680](https://github.com/peak/s5cmd/issues/680) | --show-progress stops at 50% | `64e54f4` atomic loads | Data race fix on progress counters |
| [#681](https://github.com/peak/s5cmd/issues/681) | sync --delete hangs with many files | `2589055` + `9080e28` | Error handling + extsort bump |
| [#689](https://github.com/peak/s5cmd/issues/689) | unexpected EOF when sync/cp files | `91f45bd` + `2589055` | Retry on expired token + sync error handling |
| [#690](https://github.com/peak/s5cmd/issues/690) | Extended character support | `a359daf` shellquote | Improved quoting, may not cover all cases |
| [#691](https://github.com/peak/s5cmd/issues/691) | Files not found (u202f) in sync | `a359daf` shellquote | Unicode character handling improved |
| [#693](https://github.com/peak/s5cmd/issues/693) | Download speed reduces with large files | `ff4fd4e` pooled buffers | Buffer pools may improve throughput |
| [#695](https://github.com/peak/s5cmd/issues/695) | Source error with sync delete all dest files | `2589055` sync error handling | Error handling improvements |
| [#696](https://github.com/peak/s5cmd/issues/696) | Multipart signature auth error | `6138e7e` AWS SDK bump | SDK v1.55.5 may fix signing |
| [#702](https://github.com/peak/s5cmd/issues/702) | ls --region argument | `8078699` region improvements | Region handling improved |
| [#749](https://github.com/peak/s5cmd/issues/749) | sync failed due to broken symlink | `d04203c` filter order | Non-regular files filtered more gracefully |
| [#784](https://github.com/peak/s5cmd/issues/784) | --show-progress output on newline | `f41bf88` --log-progress | New flag provides newline-separated output |
| [#787](https://github.com/peak/s5cmd/issues/787) | Add show progress option to sync | `f41bf88` --log-progress | --log-progress works with cp (sync uses cp internally) |
| [#802](https://github.com/peak/s5cmd/issues/802) | Error uploading large file with multipart | `9aee5e5` + `ff4fd4e` | Multipart copy + buffer pools |
| [#805](https://github.com/peak/s5cmd/issues/805) | CVE-2025-22871 | `9080e28` Go 1.24 | Go upgrade addresses CVEs |
| [#807](https://github.com/peak/s5cmd/issues/807) | Can't use docker with EKS Pod Identity | `6138e7e` AWS SDK bump | SDK v1.55.5 adds EKS Pod Identity support |
| [#851](https://github.com/peak/s5cmd/issues/851) | Logs get into stdout messing up pipes | `11b13bf` --log-file | New --log-file flag redirects logs |

## NOT FIXED — Open issues not addressed

### Bug Reports
| Issue | Title | Category |
|-------|-------|----------|
| [#517](https://github.com/peak/s5cmd/issues/517) | Incorrect results for keys ending in `/` | Bug |
| [#649](https://github.com/peak/s5cmd/issues/649) | sync --stat does not report correct rm quantity | Bug |
| [#707](https://github.com/peak/s5cmd/issues/707) | rm doesn't delete 0-byte folder placeholders | Bug — fork commit `4e631d3` is unreachable: validation rejects `s3://b/dir/` before it runs. Adopt upstream [#861](https://github.com/peak/s5cmd/pull/861) instead. |
| [#718](https://github.com/peak/s5cmd/issues/718) | --no-clobber with cp works wrong | Bug |
| [#720](https://github.com/peak/s5cmd/issues/720) | Strange behaviour of sync | Bug |
| [#744](https://github.com/peak/s5cmd/issues/744) | s5cmd does not honor umask settings | Bug |
| [#745](https://github.com/peak/s5cmd/issues/745) | Killed - Out of Memory | Bug |
| [#751](https://github.com/peak/s5cmd/issues/751) | Sync list incomplete with UTF error | Bug |
| [#755](https://github.com/peak/s5cmd/issues/755) | ls yields both relative and absolute paths | Bug |
| [#800](https://github.com/peak/s5cmd/issues/800) | input/output error with cp symlink | Bug |
| [#810](https://github.com/peak/s5cmd/issues/810) | no match found for local destination | Bug |
| [#826](https://github.com/peak/s5cmd/issues/826) | cp fails with chmod operation not permitted | Bug |
| [#834](https://github.com/peak/s5cmd/issues/834) | rm does not delete DIROBJ objects | Bug — same as #707. |
| [#845](https://github.com/peak/s5cmd/issues/845) | cp Timezone Issue | Bug |
| [#852](https://github.com/peak/s5cmd/issues/852) | sync --delete not deleting removed files | Bug |

### Feature Requests
| Issue | Title | Category |
|-------|-------|----------|
| [#2](https://github.com/peak/s5cmd/issues/2) | Multiple local sources (expanded wildcards) | Feature |
| [#88](https://github.com/peak/s5cmd/issues/88) | --color option | Feature |
| [#152](https://github.com/peak/s5cmd/issues/152) | MD5 option for overwrite/sync | Feature |
| [#319](https://github.com/peak/s5cmd/issues/319) | Failed to create new OS thread | Infrastructure |
| [#350](https://github.com/peak/s5cmd/issues/350) | Preserve permissions | Feature |
| [#388](https://github.com/peak/s5cmd/issues/388) | LastModified filter on s3 listing | Feature |
| [#390](https://github.com/peak/s5cmd/issues/390) | Respect ulimit or warn | Feature |
| [#433](https://github.com/peak/s5cmd/issues/433) | Limit upload/download bandwidth | Feature |
| [#489](https://github.com/peak/s5cmd/issues/489) | Tree command | Feature |
| [#515](https://github.com/peak/s5cmd/issues/515) | GrantRead possible? | Feature |
| [#528](https://github.com/peak/s5cmd/issues/528) | Resume interrupted downloads | Feature |
| [#532](https://github.com/peak/s5cmd/issues/532) | Preserve timestamps | Feature |
| [#540](https://github.com/peak/s5cmd/issues/540) | 1 depth wildcard | Feature |
| [#557](https://github.com/peak/s5cmd/issues/557) | QUIC support | Feature |
| [#561](https://github.com/peak/s5cmd/issues/561) | Sync based on hash mismatch | Feature |
| [#592](https://github.com/peak/s5cmd/issues/592) | GCS SSE using CSEK | Feature |
| [#651](https://github.com/peak/s5cmd/issues/651) | Delete bucket with objects | Feature |
| [#655](https://github.com/peak/s5cmd/issues/655) | --include flag for mv and ls | Feature |
| [#670](https://github.com/peak/s5cmd/issues/670) | Copy from external S3 API | Feature |
| [#673](https://github.com/peak/s5cmd/issues/673) | --no-guess-mimetype | Feature |
| [#688](https://github.com/peak/s5cmd/issues/688) | --show-progress for s5cmd run | Feature |
| [#694](https://github.com/peak/s5cmd/issues/694) | endpoint_url in config file | Feature |
| [#700](https://github.com/peak/s5cmd/issues/700) | Different endpoints for src/dst | Feature |
| [#719](https://github.com/peak/s5cmd/issues/719) | Dualstack (IPv6) | Feature |
| [#725](https://github.com/peak/s5cmd/issues/725) | S3 OneZone Express | Feature |
| [#741](https://github.com/peak/s5cmd/issues/741) | Multi account copy/sync | Feature |
| [#746](https://github.com/peak/s5cmd/issues/746) | Efficient incremental sync | Feature |
| [#750](https://github.com/peak/s5cmd/issues/750) | Create tarball from objects | Feature |
| [#752](https://github.com/peak/s5cmd/issues/752) | Conditional write support | Feature |
| [#754](https://github.com/peak/s5cmd/issues/754) | Upload with pre-signed URL | Feature |
| [#762](https://github.com/peak/s5cmd/issues/762) | cp all versions of single key | Feature |
| [#771](https://github.com/peak/s5cmd/issues/771) | Fish completion | Feature |
| [#785](https://github.com/peak/s5cmd/issues/785) | rclone --links support | Feature |
| [#786](https://github.com/peak/s5cmd/issues/786) | Ubuntu PPA | Packaging |
| [#797](https://github.com/peak/s5cmd/issues/797) | HTTP connections less persistent | Performance |
| [#803](https://github.com/peak/s5cmd/issues/803) | PutObjectTagging support | Feature |
| [#808](https://github.com/peak/s5cmd/issues/808) | SSE-C support | Feature |
| [#812](https://github.com/peak/s5cmd/issues/812) | force-glacier-transfer for sync | Feature |
| [#813](https://github.com/peak/s5cmd/issues/813) | metadata-directive default | Behavior |
| [#821](https://github.com/peak/s5cmd/issues/821) | Multi-Region Access Points | Feature |
| [#822](https://github.com/peak/s5cmd/issues/822) | Local time zones in ls | Feature |
| [#832](https://github.com/peak/s5cmd/issues/832) | Upgrade to AWS SDK 2 | Infrastructure |
| [#837](https://github.com/peak/s5cmd/issues/837) | Separate storage class for small objects | Feature |
| [#846](https://github.com/peak/s5cmd/issues/846) | mv remove empty directories | Feature |

### Documentation
| Issue | Title |
|-------|-------|
| [#499](https://github.com/peak/s5cmd/issues/499) | Use Github issue template |
| [#584](https://github.com/peak/s5cmd/issues/584) | Goreleaser deprecates archives.replacements |
| [#686](https://github.com/peak/s5cmd/issues/686) | Ubuntu 20.04 installation instruction |
| [#703](https://github.com/peak/s5cmd/issues/703) | Python Wheels |
| [#706](https://github.com/peak/s5cmd/issues/706) | README.md internal link incorrect |
| [#765](https://github.com/peak/s5cmd/issues/765) | Complete --help outputs in README |
| [#773](https://github.com/peak/s5cmd/issues/773) | Document escaping in s5cmd run |

### Questions (not bugs)
| Issue | Title |
|-------|-------|
| [#414](https://github.com/peak/s5cmd/issues/414) | How does s5cmd handle large downloads if object changes? |
| [#418](https://github.com/peak/s5cmd/issues/418) | Concurrency flag performance |
| [#454](https://github.com/peak/s5cmd/issues/454) | Where does s5cmd performance come from? |
| [#488](https://github.com/peak/s5cmd/issues/488) | Known file corruption risk |
| [#514](https://github.com/peak/s5cmd/issues/514) | cp with multiple options |
| [#531](https://github.com/peak/s5cmd/issues/531) | Read after write consistency |
| [#545](https://github.com/peak/s5cmd/issues/545) | Configuration did not take effect |
| [#551](https://github.com/peak/s5cmd/issues/551) | Can't ls GCS bucket |
| [#667](https://github.com/peak/s5cmd/issues/667) | Low throughput uploading single file |
| [#679](https://github.com/peak/s5cmd/issues/679) | Inconsistent downloads on Lambda |
| [#687](https://github.com/peak/s5cmd/issues/687) | Publish to pypi |
| [#743](https://github.com/peak/s5cmd/issues/743) | numworker issue |
| [#748](https://github.com/peak/s5cmd/issues/748) | Does s5cmd skip hidden files? |
| [#791](https://github.com/peak/s5cmd/issues/791) | I have more directory with files |
| [#819](https://github.com/peak/s5cmd/issues/819) | How to reach max performance |
| [#829](https://github.com/peak/s5cmd/issues/829) | Does s5cmd check download integrity? |
| [#830](https://github.com/peak/s5cmd/issues/830) | How to control multipart chunk size? |
| [#831](https://github.com/peak/s5cmd/issues/831) | This program does not work at all |
| [#844](https://github.com/peak/s5cmd/issues/844) | Does rm have parallelism? |
| [#855](https://github.com/peak/s5cmd/issues/855) | Is this package being maintained? |

## Stats

- **Total open issues:** 136
- **Fixed by our branch:** 32
- **Improved/partially addressed:** 24
- **Not fixed:** 66
- **Questions (not actionable):** 20

## Reconciliation against v2.4.1 (2026-09-18)

Upstream has 144 open issues (136 in March + 8 new, none closed). Each was checked against the v2.4.1 build; "verified" means an automated test in this repo reproduces the issue on the old code and passes on v2.4.1.

### Resolved in v2.4.1 (new since March)

| Issue | Title | How | Verified |
|-------|-------|-----|----------|
| [#872](https://github.com/peak/s5cmd/issues/872) | **Path traversal in S3→local downloads (RCE)** | keys resolving outside the destination are rejected; exit 1 | e2e cp + sync, 11 unit cases; confirmed vulnerable on v2.4.0 first |
| [#869](https://github.com/peak/s5cmd/issues/869) | sync errors but exits 0 | every printed ERROR reaches the exit code | e2e |
| [#852](https://github.com/peak/s5cmd/issues/852) | sync --delete never deletes | root cause: destination listing failed silently (BucketRegionError); listing errors now stop the sync; generated rm carries the region | e2e with a region-redirecting fake |
| [#804](https://github.com/peak/s5cmd/issues/804) | errors to stdout | usage errors → stderr | e2e |
| [#744](https://github.com/peak/s5cmd/issues/744) | umask not honoured | files created 0666 before umask | unit (umask 022/002/077) |
| [#826](https://github.com/peak/s5cmd/issues/826) | chmod not permitted | no chmod call any more | same fix |
| [#868](https://github.com/peak/s5cmd/issues/868) | --exclude-from / --include-from | added to cp, mv, rm, sync, ls, du, select | unit + e2e |
| [#649](https://github.com/peak/s5cmd/issues/649) | --stat rm count | one stat per object | e2e |
| [#755](https://github.com/peak/s5cmd/issues/755) | ls mixes full and relative paths | exact match rendered like its siblings | unit + e2e |
| [#870](https://github.com/peak/s5cmd/issues/870), [#844](https://github.com/peak/s5cmd/issues/844) | slow / non-parallel deletes | DeleteObjects batches run on the worker pool (was a fixed 10) | unit (concurrency asserted); 3.3s → 1.05s on a 100-batch mock |
| [#707](https://github.com/peak/s5cmd/issues/707), [#834](https://github.com/peak/s5cmd/issues/834) | rm of directory markers | `rm --raw s3://b/dir/` | unit + e2e |
| [#815](https://github.com/peak/s5cmd/issues/815) | sync --delete ignores --exclude | fixed properly (v2.4.0's fix was incomplete; copy side fixed too) | 6 e2e |
| [#839](https://github.com/peak/s5cmd/issues/839) | shouldOverride region | destination HEAD uses --destination-region | — (same as upstream #862) |
| [#817](https://github.com/peak/s5cmd/issues/817) | humanize suffix | `512B` | unit + e2e |
| [#824](https://github.com/peak/s5cmd/issues/824) | sync silently fails in wrong region | covered by the #852 fix | e2e |

### Investigated, not a bug

| Issue | Finding |
|-------|---------|
| [#718](https://github.com/peak/s5cmd/issues/718) | `cp -n` producing `file.jpg.jpg`: not reproducible on v2.4.1, master, or the reporter's exact v2.2.2 build. The doubled names must already be on disk. Regression tests added for the dir-without-slash + `-n` shape. Suggested reply: `ls -la 2022/01`. |

### New since March, still open

| Issue | Title | Status |
|-------|-------|--------|
| [#865](https://github.com/peak/s5cmd/issues/865) | cp fails with 400 on KMS-encrypted cross-account bucket | needs real AWS to reproduce |
| [#875](https://github.com/peak/s5cmd/issues/875) | "Lots of output, no actual syncing" (sshfs destination) | question; possibly UNC path parsing on Windows |
| [#877](https://github.com/peak/s5cmd/issues/877) | scoped prefix suggestion | feature, vague |
| [#873](https://github.com/peak/s5cmd/issues/873) | new release possible | this is it |
| [#697](https://github.com/peak/s5cmd/issues/697) | dry-run indication in output | deferred: changes output scripts parse |
| [#796](https://github.com/peak/s5cmd/issues/796) | message when sync has nothing to do | deferred: same reason |
| [#542](https://github.com/peak/s5cmd/issues/542) | X-Amz-Content-Sha256 on cross-region copy | needs real AWS |
| [#758](https://github.com/peak/s5cmd/issues/758) | sync s3 to s3? | question (it works) |

Everything else in the NOT FIXED tables above is unchanged.
