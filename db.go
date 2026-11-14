package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDB() error {
	if err := os.MkdirAll(cfg.Storage.DataDir, 0o755); err != nil {
		return err
	}
	dbPath := filepath.Join(cfg.Storage.DataDir, "bbs.db")
	var err error
	db, err = sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            TEXT PRIMARY KEY,
			username      TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at    INTEGER NOT NULL,
			status        TEXT NOT NULL DEFAULT 'active'
		);
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
		CREATE TABLE IF NOT EXISTS drafts (
			id         TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL,
			board_id   TEXT NOT NULL DEFAULT '',
			thread_id  TEXT NOT NULL DEFAULT '',
			title      TEXT NOT NULL DEFAULT '',
			file_path  TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_drafts_user ON drafts(user_id, updated_at DESC);
		CREATE TABLE IF NOT EXISTS profiles (
			user_id  TEXT PRIMARY KEY,
			nickname TEXT NOT NULL DEFAULT '',
			bio      TEXT NOT NULL DEFAULT '',
			avatar   TEXT NOT NULL DEFAULT ''
		);
	`)
	if err != nil {
		return err
	}
	cleanupOrphanDrafts()
	return seedBoards()
}

func seedBoards() error {
	boards := []struct{ id, name, desc string }{
		{"general", "General", "General discussion"},
		{"tech", "Technology", "Technology and tools"},
		{"retro", "Retro", "Old Internet and BBS culture"},
	}
	for _, b := range boards {
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO boards(id, name, description, created_at) VALUES (?, ?, ?, ?)`,
			b.id, b.name, b.desc, time.Now().Unix(),
		); err != nil {
			return err
		}
	}
	return nil
}

// 清理无上下文的坏草稿（历史 bug 遗留）
func cleanupOrphanDrafts() {
	rows, err := db.Query(`SELECT id, file_path FROM drafts WHERE board_id = '' AND thread_id = ''`)
	if err != nil {
		return
	}
	var rels []string
	var ids []string
	for rows.Next() {
		var id, rel string
		_ = rows.Scan(&id, &rel)
		ids = append(ids, id)
		if rel != "" {
			rels = append(rels, rel)
		}
	}
	rows.Close()
	for _, id := range ids {
		_, _ = db.Exec(`DELETE FROM drafts WHERE id = ?`, id)
	}
	for _, rel := range rels {
		_ = os.Remove(rel)
	}
}
