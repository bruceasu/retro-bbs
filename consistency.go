package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// checkConsistency 启动时执行。
// 1) DB 有记录但文件缺失 → 隐藏主题 / 删除回复
// 2) 文件存在但 DB 无记录 → 移到 data/orphan/
func checkConsistency() {
	if err := checkMissingFiles(); err != nil {
		log.Printf("consistency (missing files): %v", err)
	}
	if err := cleanupOrphans(); err != nil {
		log.Printf("consistency (orphans): %v", err)
	}
}

func checkMissingFiles() error {
	// threads
	rows, err := db.Query(`SELECT id, file_path, hidden FROM threads`)
	if err != nil {
		return err
	}
	type threadFix struct {
		id, path string
	}
	var hideThreads []threadFix
	for rows.Next() {
		var id, path string
		var hidden int
		_ = rows.Scan(&id, &path, &hidden)
		if hidden != 0 {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				hideThreads = append(hideThreads, threadFix{id, path})
			} else {
				log.Printf("consistency: stat thread %s: %v", id, err)
			}
		}
	}
	rows.Close()

	for _, t := range hideThreads {
		log.Printf("consistency: thread %s missing file %s, marking hidden", t.id, t.path)
		if _, err := db.Exec(`UPDATE threads SET hidden = 1 WHERE id = ?`, t.id); err != nil {
			log.Printf("consistency: hide thread %s: %v", t.id, err)
		}
	}

	// posts
	pRows, err := db.Query(`SELECT id, thread_id, file_path FROM posts`)
	if err != nil {
		return err
	}
	type postFix struct {
		id, threadID, path string
	}
	var delPosts []postFix
	for pRows.Next() {
		var id, tid, path string
		_ = pRows.Scan(&id, &tid, &path)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				delPosts = append(delPosts, postFix{id, tid, path})
			} else {
				log.Printf("consistency: stat post %s: %v", id, err)
			}
		}
	}
	pRows.Close()

	affectedThreads := make(map[string]bool)
	for _, p := range delPosts {
		log.Printf("consistency: post %s missing file %s, deleting", p.id, p.path)
		if _, err := db.Exec(`DELETE FROM posts WHERE id = ?`, p.id); err != nil {
			log.Printf("consistency: delete post %s: %v", p.id, err)
			continue
		}
		unindexByRef("post", p.id)
		affectedThreads[p.threadID] = true
	}
	for tid := range affectedThreads {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM posts WHERE thread_id = ?`, tid).Scan(&n)
		_, _ = db.Exec(`UPDATE threads SET reply_count = ? WHERE id = ?`, n, tid)
		log.Printf("consistency: thread %s reply_count -> %d", tid, n)
	}
	return nil
}

func cleanupOrphans() error {
	postsRoot := filepath.Join(cfg.Storage.DataDir, "posts")
	orphanRoot := filepath.Join(cfg.Storage.DataDir, "orphan")

	if _, err := os.Stat(postsRoot); os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(orphanRoot, 0o755); err != nil {
		return err
	}

	known := make(map[string]bool)

	tRows, err := db.Query(`SELECT file_path FROM threads`)
	if err != nil {
		return err
	}
	for tRows.Next() {
		var p string
		_ = tRows.Scan(&p)
		known[filepath.Clean(p)] = true
	}
	tRows.Close()

	pRows, err := db.Query(`SELECT file_path FROM posts`)
	if err != nil {
		return err
	}
	for pRows.Next() {
		var p string
		_ = pRows.Scan(&p)
		known[filepath.Clean(p)] = true
	}
	pRows.Close()

	var orphans []string
	_ = filepath.WalkDir(postsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		if strings.HasSuffix(path, ".tmp") {
			return nil
		}
		if known[filepath.Clean(path)] {
			return nil
		}
		orphans = append(orphans, path)
		return nil
	})

	if len(orphans) == 0 {
		return nil
	}

	for _, p := range orphans {
		rel, err := filepath.Rel(postsRoot, p)
		if err != nil {
			continue
		}
		dst := filepath.Join(orphanRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			log.Printf("consistency: mkdir for orphan %s: %v", dst, err)
			continue
		}
		if err := os.Rename(p, dst); err != nil {
			log.Printf("consistency: move orphan %s -> %s: %v", p, dst, err)
			continue
		}
		log.Printf("consistency: orphan %s -> %s", p, dst)
	}
	log.Printf("consistency: moved %d orphan file(s) to %s", len(orphans), orphanRoot)
	return nil
}