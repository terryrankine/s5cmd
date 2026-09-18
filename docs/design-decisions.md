# Design decisions

Product decisions this fork has made, one entry each: what the user sees, the
options weighed, the choice, and the commit. Add an entry when a change
alters what a command does, not how it does it. Reversing a decision gets a
new entry that points back.

Format: **DD-n title** · status · date · commit(s)

---

**DD-1 A listing error stops a sync** · accepted · 2026-09-17 · e81a197 (v2.4.1)

`sync` plans from two listings. Options: (a) keep going with whatever was
listed, as upstream did; (b) stop on any listing error. With (a), a throttled
or failed page made the destination look partly empty: every object was
copied again and `--delete` removed the rest. Chose (b): a sync that cannot
see both sides plans nothing. Exit 1, the error names the page.

**DD-2 `--exit-on-error` stops at the first failed command** · accepted · 2026-09-18 · 5286087 (#35)

Options: (a) keep `sync --exit-on-error` as the no-op it had become after
DD-1; (b) give `run` the flag and make `sync` pass it to the `cp`/`rm`
commands it generates. Chose (b): the flag now cancels the commands still
running after the first failure, on `run` and on `sync`.

**DD-3 `ls` hides the directory marker of the listed prefix** · accepted · 2026-09-18 · 5fb11f9 (#38, upstream #517)

The console creates a zero-byte `prefix/` key for a "folder". Options: (a)
list it as `DIR prefix/` inside its own listing, as upstream did; (b) leave it
out and show an empty folder as empty. Chose (b). Nested markers still list
as `DIR`; `cp`, `sync` and `du` skip markers as before.

**DD-4 Local wildcards take the path before the first `*`/`?` literally** · accepted · 2026-09-18 · c4bcb89 (#40, upstream #810)

A directory named `data[2024]` was read as a glob class. Options: (a) keep
`filepath.Glob` semantics for the whole path; (b) treat the part before the
first wildcard literally, as S3 prefixes are. Chose (b). From the first
wildcard on, glob syntax is unchanged.

**DD-5 An unreadable entry does not end a local walk** · accepted · 2026-09-18 · 8b9e65d (#41, upstream #720)

A dangling symlink or an unreadable directory used to end the walk of its
parent silently. Options: (a) stop the whole command; (b) report the entry
and go on. Chose (b) for `cp`/`rm`/`du`, and for `sync` when the entry is
excluded; an unreadable *directory* that is not excluded still stops `sync`
(DD-1: its subtree is missing from the listing).

**DD-6 `--delete` removes nothing after a skipped source entry** · accepted · 2026-09-18 · bec9214 (#44, upstream #749, #800)

A source file that cannot be read (dangling link, broken mount) is not in the
comparison, so its destination copy looks stale. Options: (a) delete it, as
upstream did; (b) refuse every delete for that run, as rsync does on I/O
errors on the sending side. Chose (b). The rest of the sync goes on; exit 1
says why nothing was deleted.

**DD-7 A dangling symlink in the destination counts as absent** · accepted · 2026-09-18 · bec9214 (#44)

Before, it stopped the whole sync (DD-1 applied to a per-file error).
Options: (a) stop; (b) treat the link as not there: the file is copied over
it, `--delete` does not remove it, no message. Chose (b), silently: the
destination is fixed by the sync itself.

**DD-8 `--delete` removes nothing when the source matched no object** · accepted · 2026-09-18 · this change

`sync --delete s3://bucket/typo/* dir/` reported "no object found" and then
emptied `dir/`. Options: (a) keep it: an empty source means an empty
destination, as `rsync --delete` from an empty directory does; (b) refuse,
as rsync refuses a *missing* source; (c) refuse unless a new flag says
otherwise. On S3 a missing prefix and an empty one look the same, so (a)
cannot tell a typo from intent. Chose (b) without a flag: `rm dst/*` empties
a destination in one line and says what it does. Objects left out by
`--exclude` count as matched.

**DD-9 A Glacier source object stays in the comparison** · accepted · 2026-09-18 · this change

Upstream dropped Glacier objects from the source listing, so their
destination copies looked stale: `--delete --ignore-glacier-warnings`
removed the only readable copy, exit 0. After DD-6 the plain `--delete`
refused instead, for every Glacier object. Options: (a) keep dropping them
and document; (b) keep them in the comparison: never copied unless
`--force-glacier-transfer`, never a reason to delete, reported only when a
copy would be due. Chose (b). Consequence: a sync whose destination is
already current no longer exits 1 for Glacier objects it did not need.

**DD-10 A key that would escape the destination is rejected** · accepted · 2026-09-17 · def0180 (v2.4.1, upstream #872), 297f228

`cp s3://bucket/* dir/` with a key `../../etc/x` wrote outside `dir/`.
Options: (a) sanitise the key; (b) reject it with an error and copy the
rest. Chose (b): a rewritten name hides what the bucket holds.

**DD-11 Downloads honour umask** · accepted · 2026-09-17 · def0180 (v2.4.1, upstream #744, #826)

Files were created and then `chmod 0644`. Options: (a) keep the chmod; (b)
create with `0666` and let umask apply. Chose (b): `umask 002` yields `0664`,
and filesystems that reject chmod work.

**DD-12 Keys that regexp, XML or a line cannot carry still list and sync** · accepted · 2026-09-18 · d6dff88 (#42, upstream #751)

Options: (a) skip such keys with an error; (b) quote them for the matcher,
ask S3 for `encoding-type=url`, and let `run` read a quoted argument across
lines. Chose (b): a key is data, and no key ends a listing.

**DD-13 Patch releases are automatic; minor and major are by hand** · accepted · 2026-09-18 · 7ebd917, 5dcfdcc; see MAINTAINING.md

Options: (a) every release by hand; (b) dependency, security and toolchain
updates tag the next patch on merge, with notes from the commits; features
and behaviour changes wait for a person to move `## Unreleased` into a minor
release. Chose (b). No CHANGELOG entry for a patch.

**DD-14 Go support: the go.mod floor and the current release** · accepted · 2026-09-18 · 396fcee, 8edb61e; see MAINTAINING.md

Options: (a) test the last two Go releases, as upstream did; (b) test the
floor `go.mod` declares and the current release. Chose (b): the floor moves
when a dependency needs it, with a CHANGELOG line and a minor release.

---

## Open

Questions raised by probing, not decided:

- **Listing hides objects newer than the client clock.** `storage/s3.go`
  skips objects whose `LastModified` is after `time.Now()` at the start of
  the listing (upstream #203, 0b522a3), to leave out objects written during
  a long paginated walk. A client clock behind the server's hides objects
  written in the last minutes, silently, from `ls`, `cp` and `sync`. Options:
  use the server's `Date` header as "now"; or allow the S3 skew limit
  (15 min); or drop the check. Not changed.
- **`IsGlacier` only knows `GLACIER`.** `DEEP_ARCHIVE` objects cannot be
  read either and `GLACIER_IR` can; both are treated as readable. Not
  changed.
