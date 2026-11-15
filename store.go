package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// threadFilePath 主题正文路径：data/posts/YYYY/MM/t_xxx.md
func threadFilePath(id string, now time.Time) string {
	return filepath.Join(cfg.Storage.DataDir, "posts",
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		id+".md")
}

// writeThreadFile 写主题正文，临时文件 + rename。
func writeThreadFile(id string, content []byte) (string, error) {
	now := time.Now()
	rel := threadFilePath(id, now)
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return "", err
	}
	tmp := rel + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, rel); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return rel, nil
}

// writeReplyFile 写回复正文到主题的子目录：
//
//	data/posts/YYYY/MM/t_xxx.replies/p_yyy.md
//
// 路径基于 thread 的 file_path 反推目录，确保和主题同目录（跨月回复也正确）。
// thread 必须存在于 DB，否则返回错误。
func writeReplyFile(threadID, postID string, content []byte) (string, error) {
	var threadPath string
	if err := db.QueryRow(`SELECT file_path FROM threads WHERE id = ?`, threadID).Scan(&threadPath); err != nil {
		return "", fmt.Errorf("thread not found: %w", err)
	}
	threadDir := filepath.Dir(threadPath)
	replyDir := filepath.Join(threadDir, threadID+".replies")
	if err := os.MkdirAll(replyDir, 0o755); err != nil {
		return "", err
	}
	rel := filepath.Join(replyDir, postID+".md")
	tmp := rel + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, rel); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return rel, nil
}

func readFile(rel string) ([]byte, error) {
	return os.ReadFile(rel)
}

func draftFilePath(userID, id string) string {
	return filepath.Join(cfg.Storage.DataDir, "drafts", userID, id+".md")
}

func writeDraftFile(userID, id string, content []byte) (string, error) {
	rel := draftFilePath(userID, id)
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return "", err
	}
	tmp := rel + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, rel); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return rel, nil
}
