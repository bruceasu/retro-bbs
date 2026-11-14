package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server struct {
		HTTPAddr string `toml:"http_addr"`
	} `toml:"server"`
	Storage struct {
		DataDir string `toml:"data_dir"`
	} `toml:"storage"`
	Posts struct {
		MaxSize      int  `toml:"max_size"`
		TitleMaxSize int  `toml:"title_max_size"`
		PageSize     int  `toml:"page_size"`
		AllowEdit    bool `toml:"allow_edit"`
		AllowDelete  bool `toml:"allow_delete"`
	} `toml:"posts"`
	Drafts struct {
		MaxSize int `toml:"max_size"`
	} `toml:"drafts"`
	Images struct {
		ShowByDefault       bool `toml:"show_by_default"`
		AllowExternalImages bool `toml:"allow_external_images"`
		MaxAvatarSize       int  `toml:"max_avatar_size"`
	} `toml:"images"`
	Security struct {
		MaxConnectionsPerUser int `toml:"max_connections_per_user"`
	} `toml:"security"`
	Search struct {
		Enabled bool `toml:"enabled"`
	} `toml:"search"`
}

func defaultConfig() Config {
	var c Config
	c.Server.HTTPAddr = ":8080"
	c.Storage.DataDir = "data"
	c.Posts.MaxSize = 65536
	c.Posts.TitleMaxSize = 200
	c.Posts.PageSize = 20
	c.Posts.AllowEdit = true
	c.Posts.AllowDelete = false
	c.Drafts.MaxSize = 65536
	c.Images.ShowByDefault = false
	c.Images.AllowExternalImages = true
	c.Images.MaxAvatarSize = 524288
	c.Security.MaxConnectionsPerUser = 3
	c.Search.Enabled = true
	return c
}

type Paths struct {
	Root   string
	Web    string
	Data   string
	Config string
	DotEnv string
}

func resolveRoot() string {
	if v := os.Getenv("BBS_ROOT"); v != "" {
		if filepath.IsAbs(v) {
			return filepath.Clean(v)
		}
		abs, err := filepath.Abs(v)
		if err == nil {
			return abs
		}
		return v
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, "web")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "config.toml")); err == nil {
			return dir
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func resolvePath(root, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(root, p)
}

func resolvePaths(root string) Paths {
	return Paths{
		Root:   root,
		Web:    filepath.Join(root, "web"),
		Data:   filepath.Join(root, "data"),
		Config: filepath.Join(root, "config.toml"),
	}
}

func userConfigDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "retro-bbs")
	}
	return ""
}

func loadConfig(path string) Config {
	cfg := defaultConfig()
	if _, err := os.Stat(path); err != nil {
		log.Printf("config %s not found, using defaults", path)
		return cfg
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		log.Printf("config %s invalid: %v", path, err)
	}
	return cfg
}

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