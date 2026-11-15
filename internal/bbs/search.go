package bbs

import (
	"fmt"
	"log"
	"os"
	"strings"
)

const searchPageSize = 20

type SearchResult struct {
	Kind     string // "thread" | "post"
	RefID    string
	ThreadID string
	Title    string
	Author   string
	Updated  int64
}

// rebuildSearchIndex 全量重建 FTS5 索引。启动时调用。
// 删除所有条目 → 重扫 threads + posts → 批量插入。
func rebuildSearchIndex() error {
	if _, err := db.Exec(`DELETE FROM search_index`); err != nil {
		return fmt.Errorf("clear index: %w", err)
	}

	type row struct {
		kind, refID, threadID, title, body string
		updated                            int64
	}
	var rows []row

	// threads
	tRows, err := db.Query(`SELECT id, title, file_path, updated_at FROM threads WHERE hidden = 0`)
	if err != nil {
		return fmt.Errorf("query threads: %w", err)
	}
	for tRows.Next() {
		var id, title, filePath string
		var updated int64
		if err := tRows.Scan(&id, &title, &filePath, &updated); err != nil {
			continue
		}
		body, _ := os.ReadFile(filePath)
		rows = append(rows, row{"thread", id, id, title, string(body), updated})
	}
	tRows.Close()

	// posts
	pRows, err := db.Query(`SELECT id, thread_id, file_path, updated_at FROM posts`)
	if err != nil {
		return fmt.Errorf("query posts: %w", err)
	}
	for pRows.Next() {
		var id, threadID, filePath string
		var updated int64
		if err := pRows.Scan(&id, &threadID, &filePath, &updated); err != nil {
			continue
		}
		body, _ := os.ReadFile(filePath)
		rows = append(rows, row{"post", id, threadID, "", string(body), updated})
	}
	pRows.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO search_index(kind, ref_id, thread_id, title, body, updated_at) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, r := range rows {
		_, _ = stmt.Exec(r.kind, r.refID, r.threadID, r.title, r.body, r.updated)
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("search: indexed %d entries", len(rows))
	return nil
}

// unindexByRef 删除一条索引（FTS5 需要按 rowid 删）。
func unindexByRef(kind, refID string) {
	var rowid int64
	err := db.QueryRow(`SELECT rowid FROM search_index WHERE kind = ? AND ref_id = ?`, kind, refID).Scan(&rowid)
	if err == nil {
		_, _ = db.Exec(`DELETE FROM search_index WHERE rowid = ?`, rowid)
	}
}

func indexThread(threadID string) error {
	var title, filePath string
	var updated int64
	err := db.QueryRow(`SELECT title, file_path, updated_at FROM threads WHERE id = ? AND hidden = 0`, threadID).
		Scan(&title, &filePath, &updated)
	if err != nil {
		// 帖子被隐藏或不存在 → 从索引里删掉
		unindexByRef("thread", threadID)
		return nil
	}
	body, _ := os.ReadFile(filePath)
	unindexByRef("thread", threadID)
	_, err = db.Exec(
		`INSERT INTO search_index(kind, ref_id, thread_id, title, body, updated_at)
		 VALUES ('thread', ?, ?, ?, ?, ?)`,
		threadID, threadID, title, string(body), updated)
	return err
}

func indexPost(postID string) error {
	var threadID, filePath string
	var updated int64
	err := db.QueryRow(`SELECT thread_id, file_path, updated_at FROM posts WHERE id = ?`, postID).
		Scan(&threadID, &filePath, &updated)
	if err != nil {
		unindexByRef("post", postID)
		return nil
	}
	body, _ := os.ReadFile(filePath)
	unindexByRef("post", postID)
	_, err = db.Exec(
		`INSERT INTO search_index(kind, ref_id, thread_id, title, body, updated_at)
		 VALUES ('post', ?, ?, '', ?, ?)`,
		postID, threadID, string(body), updated)
	return err
}

// searchPosts 执行 FTS5 查询。返回结果和总数。
func searchPosts(query string, limit, offset int) ([]SearchResult, int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, 0, nil
	}
	if len(query) > 200 {
		query = query[:200]
	}
	// 简单转义：去掉 FTS5 元字符，空格分隔的词用 AND（FTS5 默认）
	words := strings.Fields(query)
	for i, w := range words {
		w = strings.ReplaceAll(w, `"`, "")
		w = strings.ReplaceAll(w, `'`, "")
		w = strings.ReplaceAll(w, `(`, "")
		w = strings.ReplaceAll(w, `)`, "")
		w = strings.ReplaceAll(w, `*`, "")
		words[i] = w
	}
	q := strings.Join(words, " ")
	if q == "" {
		return nil, 0, nil
	}

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM search_index WHERE search_index MATCH ?`, q).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}

	rows, err := db.Query(`
		SELECT kind, ref_id, thread_id, COALESCE(title, ''), updated_at
		FROM search_index
		WHERE search_index MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		_ = rows.Scan(&r.Kind, &r.RefID, &r.ThreadID, &r.Title, &r.Updated)
		if r.Kind == "thread" {
			_ = db.QueryRow(`
				SELECT u.username FROM threads t JOIN users u ON u.id = t.author_id
				WHERE t.id = ?`, r.RefID).Scan(&r.Author)
		} else {
			_ = db.QueryRow(`
				SELECT u.username FROM posts p JOIN users u ON u.id = p.author_id
				WHERE p.id = ?`, r.RefID).Scan(&r.Author)
			// 回复没有标题，用所属主题的标题代替显示
			var threadTitle string
			_ = db.QueryRow(`SELECT title FROM threads WHERE id = ?`, r.ThreadID).Scan(&threadTitle)
			if threadTitle != "" {
				r.Title = "re: " + threadTitle
			}
		}
		results = append(results, r)
	}
	return results, total, nil
}
