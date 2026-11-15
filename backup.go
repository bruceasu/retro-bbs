package main

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// runBackupCmd 实现 `retro-bbs backup`。
// 用 VACUUM INTO 复制出干净的 db，再和 posts/drafts/avatars 一起打包成 zip。
func runBackupCmd() {
	root := resolveRoot()
	paths = resolvePaths(root)
	cfg = loadConfig(paths.Config)
	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = "data"
	}
	cfg.Storage.DataDir = resolvePath(root, cfg.Storage.DataDir)
	paths.Data = cfg.Storage.DataDir

	dbPath := filepath.Join(paths.Data, "bbs.db")
	if _, err := os.Stat(dbPath); err != nil {
		log.Fatalf("backup: db not found at %s", dbPath)
	}

	outDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("backup: mkdir %s: %v", outDir, err)
	}
	ts := time.Now().Format("20060102-150405")
	archivePath := filepath.Join(outDir, "bbs-"+ts+".zip")

	// 1) VACUUM INTO 临时 db
	tmpDB := filepath.Join(os.TempDir(), "retro-bbs-backup-"+ts+".db")
	defer os.Remove(tmpDB)

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("backup: open db: %v", err)
	}
	if _, err := conn.Exec(`VACUUM INTO ?`, tmpDB); err != nil {
		conn.Close()
		log.Fatalf("backup: vacuum into: %v", err)
	}
	conn.Close()

	// 2) 打包
	f, err := os.Create(archivePath)
	if err != nil {
		log.Fatalf("backup: create %s: %v", archivePath, err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	addFile := func(name, path string) error {
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		return err
	}

	if err := addFile("bbs.db", tmpDB); err != nil {
		log.Fatalf("backup: zip db: %v", err)
	}

	for _, sub := range []string{"posts", "drafts", "avatars"} {
		base := filepath.Join(paths.Data, sub)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(paths.Data, path)
			if relErr != nil {
				return nil
			}
			return addFile(filepath.ToSlash(rel), path)
		})
		if err != nil {
			log.Fatalf("backup: walk %s: %v", sub, err)
		}
	}

	log.Printf("backup: %s", archivePath)
	log.Printf("backup: contents = bbs.db + posts/ + drafts/ + avatars/")
	log.Printf("backup: excluded = data/orphan/, data/tmp/, .env, config.toml")
}

// runRestoreCmd 实现 `retro-bbs restore <archive.zip>`。
// 安全规则：data/bbs.db 已存在则拒绝，防止误覆盖。
func runRestoreCmd() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: retro-bbs restore <archive.zip>")
		os.Exit(1)
	}
	archivePath := os.Args[2]
	if _, err := os.Stat(archivePath); err != nil {
		log.Fatalf("restore: archive not found: %s", archivePath)
	}

	root := resolveRoot()
	paths = resolvePaths(root)
	cfg = loadConfig(paths.Config)
	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = "data"
	}
	cfg.Storage.DataDir = resolvePath(root, cfg.Storage.DataDir)
	paths.Data = cfg.Storage.DataDir

	existingDB := filepath.Join(paths.Data, "bbs.db")
	if _, err := os.Stat(existingDB); err == nil {
		log.Fatalf("restore: %s already exists; move or remove data/ before restoring", existingDB)
	}

	r, err := zip.OpenReader(archivePath)
	if err != nil {
		log.Fatalf("restore: open %s: %v", archivePath, err)
	}
	defer r.Close()

	if err := os.MkdirAll(paths.Data, 0o755); err != nil {
		log.Fatalf("restore: mkdir %s: %v", paths.Data, err)
	}

	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			log.Printf("restore: skip unsafe path %q", f.Name)
			continue
		}
		dst := filepath.Join(paths.Data, name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			log.Fatalf("restore: mkdir for %s: %v", dst, err)
		}
		rc, err := f.Open()
		if err != nil {
			log.Fatalf("restore: open %s in zip: %v", f.Name, err)
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			log.Fatalf("restore: create %s: %v", dst, err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			log.Fatalf("restore: write %s: %v", dst, err)
		}
		out.Close()
		rc.Close()
	}

	log.Printf("restore: %s -> %s", archivePath, paths.Data)
	log.Printf("restore: start the server to rebuild search index and run consistency checks")
}