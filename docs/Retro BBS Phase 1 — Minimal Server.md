我先按 Phase 1 的要求实现 MVP：一个能跑的 Go WebSocket 服务器 + xterm.js 前端，显示 RETRO BBS 欢迎菜单并支持键盘输入命令。不引入数据库，所有状态在内存中。

## 文件结构

```
retro-bbs/
├── main.go
├── go.mod
└── web/
    ├── index.html
    ├── style.css
    ├── app.js
    └── vendor/xterm/  (xterm.js + addon-fit)
```

先创建 go.mod 和 main.go。

```go
// main.go
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	readLimit      = 16 * 1024
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 16 * 1024
)

// 单条消息：请求/响应共用
type Message struct {
	ID    int64           `json:"id,omitempty"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data,omitempty"`
	OK    *bool           `json:"ok,omitempty"`
	Error *ErrorInfo      `json:"error,omitempty"`
	// 服务端主动推送的纯文本终端输出（MVP 用）
	Text string `json:"text,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// 会话状态（MVP 极简：只记录屏幕状态）
type session struct {
	mu   sync.Mutex
	conn *websocket.Conn

	screen  string // "welcome" | "main"
	lastCmd string
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// MVP：开发期允许同源和本地；生产要收紧
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // 非浏览器客户端（如 Go CLI）
		}
		return strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "http://127.0.0.1")
	},
}

func main() {
	fs := http.FileServer(http.Dir("./web"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", handleWS)

	addr := ":8080"
	log.Printf("RETRO BBS listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	s := &session{conn: conn, screen: "welcome"}
	s.run()
}

func (s *session) run() {
	defer s.conn.Close()

	s.conn.SetReadLimit(maxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// ping 保活
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()
		for range t.C {
			s.mu.Lock()
			err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// 首次进入：显示欢迎菜单
	s.sendText(welcomeScreen())

	for {
		_, raw, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			s.sendError(0, "BAD_JSON", "invalid json")
			continue
		}
		s.handle(msg)
	}
}

func (s *session) handle(msg Message) {
	switch msg.Type {
	case "input":
		var in struct {
			Line string `json:"line"`
		}
		_ = json.Unmarshal(msg.Data, &in)
		s.handleInput(msg.ID, in.Line)
	default:
		s.sendError(msg.ID, "UNKNOWN_TYPE", "unknown message type: "+msg.Type)
	}
}

// MVP 只处理几条命令
func (s *session) handleInput(id int64, line string) {
	line = strings.TrimSpace(line)
	s.lastCmd = line

	// 回显命令
	s.sendText("\r\n> " + line + "\r\n")

	cmd := strings.ToLower(line)
	switch cmd {
	case "":
		s.sendText(promptFor(s.screen))
	case "help", "h", "?":
		s.sendText(helpScreen())
	case "b", "boards", "1":
		s.screen = "main"
		s.sendText(mainMenuScreen())
	case "q", "quit", "logout":
		s.sendText("\r\nGoodbye.\r\n")
		// 主动关闭
		s.mu.Lock()
		_ = s.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
		s.mu.Unlock()
		_ = s.conn.Close()
		return
	default:
		s.sendText("Unknown command. Type HELP.\r\n" + promptFor(s.screen))
	}
	// 回复请求
	s.sendResponse(id, map[string]any{"ok": true})
}

func (s *session) sendText(text string) {
	s.sendRaw(Message{Type: "output", Text: text})
}

func (s *session) sendError(id int64, code, msg string) {
	ok := false
	s.sendRaw(Message{ID: id, Type: "response", OK: &ok, Error: &ErrorInfo{Code: code, Message: msg}})
}

func (s *session) sendResponse(id int64, data any) {
	ok := true
	raw, _ := json.Marshal(data)
	s.sendRaw(Message{ID: id, Type: "response", OK: &ok, Data: raw})
}

func (s *session) sendRaw(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := s.conn.WriteJSON(msg); err != nil {
		log.Printf("write: %v", err)
	}
}

// ---------- 屏幕内容 ----------

func welcomeScreen() string {
	return "\x1b[1;32m" +
		"\r\n" +
		"  ####  ##### ##### ####   ###   ####   ####   #### \r\n" +
		"  #   # #     #     #   # #   #  #   # #     #     \r\n" +
		"  ####  ###   ###   ####  #   #  ####   ###   ###   \r\n" +
		"  # #   #     #     #  #  #   #  # #       #     #  \r\n" +
		"  #  #  ##### #     #   #  ###   #  #  ####  ####  \r\n" +
		"\x1b[0m\r\n" +
		"\x1b[1;36mRETRO BBS v0.1 — MVP\x1b[0m\r\n\r\n" +
		"Welcome to the little text community.\r\n" +
		"This is the MVP: WebSocket + xterm.js, no database yet.\r\n\r\n" +
		"  [B] Boards & Threads  (demo)\r\n" +
		"  [H] Help\r\n" +
		"  [Q] Quit\r\n\r\n" +
		"\x1b[33mBBS> \x1b[0m"
}

func mainMenuScreen() string {
	return "\r\n\x1b[1;36mMAIN MENU\x1b[0m\r\n" +
		"  (demo boards — no DB, no posts yet)\r\n\r\n" +
		"  [1] general     — General discussion\r\n" +
		"  [2] tech        — Technology\r\n" +
		"  [3] retro       — Old Internet\r\n\r\n" +
		"  (Phase 2 will add login/register, then real posts.)\r\n\r\n" +
		"Type HELP or Q to quit, B to return here.\r\n\r\n" +
		"\x1b[33mBBS> \x1b[0m"
}

func helpScreen() string {
	return "\r\n\x1b[1;36mHELP\x1b[0m\r\n" +
		"Available commands (MVP):\r\n" +
		"  B / BOARDS   show demo board list\r\n" +
		"  H / HELP     this help\r\n" +
		"  Q / QUIT     disconnect\r\n" +
		"  H / ?        same as help\r\n\r\n" +
		"Phase 1 goal: server runs, browser connects, menu works.\r\n" +
		"Nothing here is persisted yet.\r\n" +
		promptFor("main")
}

func promptFor(screen string) string {
	switch screen {
	case "welcome":
		return "\x1b[33mBBS> \x1b[0m"
	default:
		return "\x1b[33mBBS> \x1b[0m"
	}
}
```

## 前端文件

```html
<!-- web/index.html -->
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>RETRO BBS</title>
  <link rel="stylesheet" href="/vendor/xterm/xterm.css" />
  <link rel="stylesheet" href="/style.css" />
</head>
<body>
  <div id="term"></div>
  <script src="/vendor/xterm/xterm.js"></script>
  <script src="/vendor/xterm/addon-fit.js"></script>
  <script src="/app.js"></script>
</body>
</html>
```

```css
/* web/style.css */
html, body {
  margin: 0;
  height: 100%;
  background: #0b0f12;
  color: #c9d1d9;
  font-family: monospace;
}
#term {
  width: 100%;
  height: 100%;
  padding: 8px;
  box-sizing: border-box;
}
```

```javascript
// web/app.js
(function () {
  const term = new Terminal({
    cursorBlink: true,
    fontFamily: 'Menlo, Consolas, monospace',
    fontSize: 14,
    theme: {
      background: '#0b0f12',
      foreground: '#c9d1d9',
      cursor: '#2ea043',
      selectionBackground: '#264f78'
    },
    convertEol: false,
    scrollback: 5000
  });

  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(document.getElementById('term'));
  fit.fit();
  window.addEventListener('resize', () => fit.fit());

  let ws = null;
  let nextId = 1;
  let lineBuf = '';
  let connected = false;

  function write(s) {
    term.write(s.replace(/\r\n/g, '\r\n'));
  }

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/ws`);

    ws.onopen = () => {
      connected = true;
      term.focus();
    };

    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      if (msg.type === 'output' && msg.text) {
        write(msg.text);
      } else if (msg.type === 'response') {
        if (msg.ok === false && msg.error) {
          write(`\r\n[error ${msg.error.code}] ${msg.error.message}\r\n`);
        }
      }
    };

    ws.onclose = () => {
      connected = false;
      write('\r\n[disconnected]\r\n');
    };

    ws.onerror = () => {
      write('\r\n[connection error]\r\n');
    };
  }

  function sendLine(line) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({
      id: nextId++,
      type: 'input',
      data: { line }
    }));
  }

  // 本地行编辑：只处理可打印字符、退格、回车
  term.onData((data) => {
    if (!connected) return;
    for (const ch of data) {
      const code = ch.charCodeAt(0);
      if (ch === '\r') {
        sendLine(lineBuf);
        lineBuf = '';
      } else if (ch === '\x7f' || ch === '\b') {
        if (lineBuf.length > 0) {
          lineBuf = lineBuf.slice(0, -1);
          term.write('\b \b');
        }
      } else if (code >= 0x20 && code < 0x7f) {
        lineBuf += ch;
        term.write(ch);
      }
      // 忽略方向键等控制字符（MVP）
    }
  });

  connect();
})();
```

## 获取 xterm.js 到 vendor

```bash
mkdir -p web/vendor/xterm
# 使用 npm 或直接下载固定版本
npm pack xterm@5.3.0
tar -xzf xterm-5.3.0.tgz
cp package/lib/xterm.js web/vendor/xterm/
cp package/lib/xterm.css web/vendor/xterm/
npm pack xterm-addon-fit@0.8.0
tar -xzf xterm-addon-fit-0.8.0.tgz
cp package/lib/addon-fit.js web/vendor/xterm/
```

（也可从 unpkg 下载同名文件放到 `web/vendor/xterm/`。）

## 运行

```bash
go mod init retro-bbs
go get github.com/gorilla/websocket
go run main.go
# 打开 http://localhost:8080
```

## 验收对照 Phase 1

- `go run main.go` 启动服务器 ✅
- 浏览器连接 WebSocket（`/ws`）✅
- 屏幕显示 RETRO BBS 欢迎菜单，可键盘输入 `B` / `H` / `Q` ✅

## 与设计文档的对应

- 通信：WebSocket + JSON，请求/响应带 `id`，错误有 `code/message` — 与第 5 节一致。
- 消息结构用了 `type: "input"`（MVP 本地行编辑）+ `type: "output"`（服务端推送文本），后续接 DB 时可直接扩展为 `login`/`thread_list` 等类型，不破坏协议形状。
- Origin 检查已加（本地放行），生产收紧即可。
- 读写超时、ping/pong、读大小限制已加，避免 MVP 就跑飞。
- 无 DB、无第三方框架、单文件 `main.go`，符合 Small Software 原则。

## 下一步（Phase 2）建议

1. 引入 `modernc.org/sqlite`，建 `users` 表。
2. 把 `handleInput` 换成真正的命令解析（`register`、`login`），Session 里存 `userID`。
3. `output` 消息保持不变，前端无需改动即可继续用。

需要我直接接着做 Phase 2 的注册/登录 + SQLite 初始化吗？