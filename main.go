package main

import (
	"log"
	"net/http"
	"strings"

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

	if err := initI18n(); err != nil {
		log.Fatalf("i18n: %v", err)
	}
	if err := initDB(); err != nil {
		log.Fatalf("db: %v", err)
	}

	http.HandleFunc("/i18n.json", handleI18nJSON)
	http.Handle("/", http.FileServer(http.Dir(paths.Web)))
	http.HandleFunc("/ws", handleWS)

	log.Printf("RETRO BBS listening on %s", cfg.Server.HTTPAddr)
	log.Printf("  root:   %s", paths.Root)
	log.Printf("  web:    %s", paths.Web)
	log.Printf("  data:   %s", paths.Data)
	log.Printf("  config: %s", paths.Config)
	if paths.DotEnv != "" {
		log.Printf("  dotenv: %s", paths.DotEnv)
	}
	log.Fatal(http.ListenAndServe(cfg.Server.HTTPAddr, nil))
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	s := &session{
		conn:     conn,
		screen:   "home",
		page:     1,
		imagesOn: cfg.Images.ShowByDefault,
	}
	s.run()
}
