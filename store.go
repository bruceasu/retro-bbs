package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func postFilePath(id string, now time.Time) string {
	return filepath.Join(cfg.Storage.DataDir, "posts",
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		id+".md")
}

func writePostFile(id string, content []byte) (string, error) {
	now := time.Now()
	rel := postFilePath(id, now)
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