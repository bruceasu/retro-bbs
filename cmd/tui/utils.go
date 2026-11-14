package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

// debugf 只在 BBS_DEBUG=1 时输出到 stderr
func debugf(format string, args ...any) {
	if os.Getenv("BBS_DEBUG") != "1" {
		return
	}
	fmt.Fprintf(os.Stderr, "[tui] "+format+"\n", args...)
}

// loadDotEnv 查找并加载 .env，注入 os.Environ。
// 查找顺序：~/.config/retro-bbs/.env → <root>/.env。
// 已存在的环境变量不会被覆盖（系统环境变量优先）。
// 返回实际加载的路径；没找到返回空。
func loadDotEnv(root string) string {
	candidates := []string{}
	if d := userConfigDir(); d != "" {
		candidates = append(candidates, filepath.Join(d, ".env"))
	}
	candidates = append(candidates, filepath.Join(root, ".env"))

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			parseDotEnv(path)
			return path
		}
	}
	return ""
}

func parseDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

// userConfigDir 返回 ~/.config/retro-bbs-client（Linux/macOS）或 %APPDATA%\retro-bbs-client（Windows）。
func userConfigDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "retro-bbs-client")
	}
	return ""
}
