package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var cfg Config
var paths Paths

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "http://127.0.0.1")
	},
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "backup":
			runBackupCmd()
			return
		case "restore":
			runRestoreCmd()
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	root := resolveRoot()
	paths = resolvePaths(root)

	if paths.Web == paths.Root {
		log.Fatalf("web dir must not equal root dir (would expose config/data)")
	}

	paths.DotEnv = loadDotEnv(root)
	cfg = loadConfig(paths.Config)

	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = "data"
	}
	cfg.Storage.DataDir = resolvePath(root, cfg.Storage.DataDir)
	paths.Data = cfg.Storage.DataDir

	loginLimiter = newRateLimiter(cfg.Security.LoginAttemptsPerMin, time.Minute)

	if err := initI18n(); err != nil {
		log.Fatalf("i18n: %v", err)
	}
	if err := initDB(); err != nil {
		log.Fatalf("db: %v", err)
	}

	http.HandleFunc("/i18n.json", handleI18nJSON)
	http.Handle("/", http.FileServer(http.Dir(paths.Web)))
	http.HandleFunc("/ws", handleWS)

	if cfg.Agent.Enabled {
		mcpPath := cfg.Agent.Path
		if mcpPath == "" {
			mcpPath = "/mcp"
		}
		http.Handle(mcpPath, newMCPHTTPHandler())
		log.Printf("agent: enabled at %s (read tools anonymous, write tools require Bearer agent key)", mcpPath)
		log.Printf("agent: limits per_agent_per_thread=%d total_per_thread=%d max_agents=%d human_anchor=%v",
			cfg.Agent.MaxRepliesPerAgentPerThread,
			cfg.Agent.MaxTotalAgentRepliesPerThread,
			cfg.Agent.MaxAgentsPerThread,
			cfg.Agent.RequireHumanAnchor,
		)
	}

	log.Printf("RETRO BBS listening on %s", cfg.Server.HTTPAddr)
	log.Printf("  root:      %s", paths.Root)
	log.Printf("  web:       %s", paths.Web)
	log.Printf("  data:      %s", paths.Data)
	log.Printf("  config:    %s", paths.Config)
	if paths.DotEnv != "" {
		log.Printf("  dotenv:    %s", paths.DotEnv)
	}
	log.Printf("  conn/user: %d", cfg.Security.MaxConnectionsPerUser)
	log.Printf("  login/min: %d", cfg.Security.LoginAttemptsPerMin)
	log.Fatal(http.ListenAndServe(cfg.Server.HTTPAddr, nil))
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	remoteAddr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}
	s := &session{
		conn:       conn,
		screen:     "home",
		page:       1,
		imagesOn:   cfg.Images.ShowByDefault,
		remoteAddr: remoteAddr,
	}
	s.run()
}

func printUsage() {
	fmt.Println(`RETRO BBS

Usage:
  retro-bbs                    start the server (default)
  retro-bbs backup             create a backup archive in ./backups/
  retro-bbs restore <zip>      restore data from an archive (data/ must not exist)
  retro-bbs help               show this help

Environment:
  BBS_ROOT     override the root directory
  BBS_URL      TUI WebSocket URL (only for cmd/tui)
  BBS_DEBUG    set to 1 for verbose TUI logging
  BBS_LANG     TUI language (en / zh)

MCP (for AI agents):
  Enable [agent] enabled = true in config.toml.
  Read tools (list_boards, search_posts, get_thread, get_post, get_user_profile)
  are available anonymously at /mcp (default path).
  Write tools (get_agent_info, list_pending_asks, reply_to_thread) require
  Authorization: Bearer <agent_key> — get the key from admin UI after creating
  an agent.

See docs/backup.md and docs/mcp.md.`)
}
