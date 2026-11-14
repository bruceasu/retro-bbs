CREATE TABLE IF NOT EXISTS boards (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS threads (
    id          TEXT PRIMARY KEY,
    board_id    TEXT NOT NULL,
    author_id   TEXT NOT NULL,
    title       TEXT NOT NULL,
    file_path   TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    reply_count INTEGER NOT NULL DEFAULT 0,
    hidden      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_threads_board ON threads(board_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS posts (
    id         TEXT PRIMARY KEY,
    thread_id  TEXT NOT NULL,
    author_id  TEXT NOT NULL,
    file_path  TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_posts_thread ON posts(thread_id, created_at ASC);

CREATE TABLE IF NOT EXISTS profiles (
			user_id  TEXT PRIMARY KEY,
			nickname TEXT NOT NULL DEFAULT '',
			bio      TEXT NOT NULL DEFAULT '',
			avatar   TEXT NOT NULL DEFAULT ''
		);
