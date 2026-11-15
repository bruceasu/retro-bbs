# Backup & Restore

RETRO BBS stores data in two places: SQLite metadata (`data/bbs.db`)
and Markdown files (`data/posts/`, `data/drafts/`, `data/avatars/`).
Both must be backed up together.

## What gets backed up

| Item | In archive |
| --- | --- |
| `bbs.db` | yes, via `VACUUM INTO` (WAL merged, consistent snapshot) |
| `posts/` | yes, all `.md` files including `*.replies/` subdirs |
| `drafts/` | yes |
| `avatars/` | yes |
| `orphan/` | **no** — audit material, not authoritative data |
| `tmp/` | **no** — transient |
| `.env`, `config.toml` | **no** — deployment config, not data |

## Create a backup

Stop the server first (or accept eventual consistency; the startup
consistency check will fix any drift on the next start).

    retro-bbs backup

Produces `backups/bbs-YYYYMMDD-HHMMSS.zip` with `bbs.db` at the root
and `posts/`, `drafts/`, `avatars/` subdirs.

If `BBS_ROOT` is set, the archive goes to `$BBS_ROOT/backups/`.

## Restore from a backup

The restore command refuses to overwrite existing data.

    # stop the server
    mv data data.old
    retro-bbs restore backups/bbs-20260101-120000.zip
    # start the server — it will rebuild the search index
    # and run the startup consistency check

`data.old/` is your safety copy; delete it once you have verified
the restored server.

## Startup consistency check

On every startup, before serving requests, the server:

1. **Scans `threads`** — if `file_path` is missing on disk, the thread
   is marked `hidden = 1`. It disappears from `thread_list` but its
   reply rows are kept (they may still be readable if only the
   thread body was lost).
2. **Scans `posts`** — if `file_path` is missing on disk, the row is
   deleted, its `search_index` entry is removed, and the parent
   thread's `reply_count` is recalculated from the remaining rows.
3. **Scans `data/posts/`** — files that are not referenced by any
   `threads.file_path` or `posts.file_path` are moved to
   `data/orphan/`, keeping their original `YYYY/MM/...` path so you
   can inspect them by hand.

The check only logs; it never aborts startup. If a file is
unreadable for a non-`ENOENT` reason (permissions, network), the
row is left alone and a warning is logged — the next start will
retry.

## What to do with `data/orphan/`

Orphan files usually mean:

- you deleted a thread row by hand but left the file behind
- you restored an old backup that lost the rows that pointed at those files

Look through `data/orphan/` and either delete it (files are no longer
referenced) or restore the missing rows by hand. The server will not
re-attach orphan files automatically — that would require guessing,
and guessing is how data gets corrupted.

## Restoring a hidden thread

If a thread was marked `hidden = 1` because its file was missing
during a startup check, and you later recover the file from a
backup, re-enable it manually:

    sqlite3 data/bbs.db "UPDATE threads SET hidden = 0 WHERE id = 't_xxx';"

Then restart the server so the search index picks it up again.
