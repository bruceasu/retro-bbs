package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Prefs 是 TUI 客户端的本地偏好，存在 $USER_HOME/.config/retro-bbs-client/config.toml。
type Prefs struct {
	Lang   string `toml:"lang"`
	Editor string `toml:"editor"` // 可选，覆盖 $EDITOR
}

func prefsDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "retro-bbs-client")
	}
	return ""
}

func prefsPath() string {
	d := prefsDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "config.toml")
}

func loadPrefs() Prefs {
	p := prefsPath()
	var prefs Prefs
	if p == "" {
		return prefs
	}
	if _, err := os.Stat(p); err != nil {
		return prefs
	}
	if _, err := toml.DecodeFile(p, &prefs); err != nil {
		log.Printf("prefs %s invalid: %v", p, err)
	}
	return prefs
}

func savePrefs(prefs Prefs) {
	p := prefsPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		log.Printf("prefs mkdir: %v", err)
		return
	}
	f, err := os.Create(p)
	if err != nil {
		log.Printf("prefs create: %v", err)
		return
	}
	defer f.Close()
	_ = toml.NewEncoder(f).Encode(prefs)
}
