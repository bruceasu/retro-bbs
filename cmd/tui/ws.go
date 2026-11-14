package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Screen struct {
	Screen       string     `json:"screen"`
	Title        string     `json:"title"`
	TitleDefault string     `json:"title_default,omitempty"`
	Lines        []Line     `json:"lines,omitempty"`
	Menu         []MenuItem `json:"menu,omitempty"`
	Form         *Form      `json:"form,omitempty"`
	Hint         string     `json:"hint,omitempty"`
	HintDefault  string     `json:"hint_default,omitempty"`
	ImagesOn     bool       `json:"images_on,omitempty"`

	Page      int  `json:"page,omitempty"`
	TotalPage int  `json:"total_page,omitempty"`
	CanPrev   bool `json:"can_prev,omitempty"`
	CanNext   bool `json:"can_next,omitempty"`
}

type Line struct {
	Key     string   `json:"key,omitempty"`
	Args    []string `json:"args,omitempty"`
	Default string   `json:"default,omitempty"`
	Raw     string   `json:"raw,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Spans   []Span   `json:"spans,omitempty"`
}

type MenuItem struct {
	Key      string `json:"key,omitempty"`
	LabelKey string `json:"label_key,omitempty"`
	Default  string `json:"default,omitempty"`
	Label    string `json:"label,omitempty"`
	Action   string `json:"action"`
}

type FormField struct {
	Name      string `json:"name"`
	LabelKey  string `json:"label_key"`
	Default   string `json:"default,omitempty"`
	Secret    bool   `json:"secret,omitempty"`
	Value     string `json:"value,omitempty"`
	Multiline bool   `json:"multiline,omitempty"`
}

// 与服务端 protocol.go 对齐（只读需要的字段）
type Message struct {
	ID    int64           `json:"id,omitempty"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data,omitempty"`
	OK    *bool           `json:"ok,omitempty"`
	Error *ErrorInfo      `json:"error,omitempty"`
	Text  string          `json:"text,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Span struct {
	Text  string `json:"text"`
	Style string `json:"style,omitempty"`
	URL   string `json:"url,omitempty"`
}

type Form struct {
	Fields   []FormField `json:"fields"`
	Submit   string      `json:"submit"`
	CanDraft bool        `json:"can_draft,omitempty"`
}

// WSClient 管理连接和收发。
type WSClient struct {
	mu   sync.Mutex
	conn *websocket.Conn

	nextID int64
	inbox  chan Message
	closed chan struct{}
}

// DialWS 连接 BBS 服务端。语言由 TUI 本地决定，不通过握手协商。
func DialWS(url string) (*WSClient, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	c := &WSClient{
		conn:   conn,
		inbox:  make(chan Message, 64),
		closed: make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *WSClient) readLoop() {
	defer close(c.closed)
	defer close(c.inbox)
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		c.inbox <- msg
	}
}

func (c *WSClient) Inbox() <-chan Message   { return c.inbox }
func (c *WSClient) Closed() <-chan struct{} { return c.closed }

func (c *WSClient) SendAction(action, value string) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	conn := c.conn
	c.mu.Unlock()

	b, _ := json.Marshal(map[string]any{
		"id":   id,
		"type": "action",
		"data": map[string]string{"action": action, "value": value},
	})

	c.mu.Lock()
	defer c.mu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, b)
}

func (c *WSClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
	_ = c.conn.Close()
}
