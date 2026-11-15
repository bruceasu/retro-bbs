package main

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 64 * 1024
)

type session struct {
	mu   chan struct{}
	conn *websocket.Conn

	screen   string
	menu     []MenuItem
	lastErr  string
	imagesOn bool
	userID   string
	username string
	nickname string
	isAdmin  bool

	boardID    string
	threadID   string
	replyID    string
	draftID    string
	prevScreen string
	page       int

	searchQuery string
	searchPage  int

	remoteAddr string
	connUserID string

	pendingDelete *pendingDelete
}

func (s *session) run() {
	s.mu = make(chan struct{}, 1)
	defer s.releaseConn()

	defer s.conn.Close()
	s.conn.SetReadLimit(maxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()
		for range t.C {
			s.mu <- struct{}{}
			err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
			<-s.mu
			if err != nil {
				return
			}
		}
	}()

	s.showHome("", "")

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
	case "action":
		var in struct {
			Action string `json:"action"`
			Value  string `json:"value"`
		}
		if err := json.Unmarshal(msg.Data, &in); err != nil {
			s.sendError(msg.ID, "BAD_ACTION", "invalid action payload")
			return
		}
		s.handleAction(msg.ID, in.Action, in.Value)
	default:
		s.sendError(msg.ID, "UNKNOWN_TYPE", "unknown type: "+msg.Type)
	}
}

func (s *session) handleAction(id int64, action, value string) {
	switch action {
	case "select":
		s.doSelect(value)
	case "back":
		s.goBack()
	case "command":
		s.route(value)
	case "submit":
		s.handleSubmit(value)
	case "save_draft":
		s.handleSaveDraft(value)
	case "page":
		s.doPage(value)
	case "new_thread", "reply", "edit_thread", "toggle_images", "profile", "help", "lang", "search",
		"admin", "admin_boards", "admin_hidden_threads", "admin_hidden_replies",
		"board_new", "reply_manage", "delete_thread", "delete_reply",
		"purge_thread", "purge_reply", "delete_board",
		"confirm_yes", "confirm_no",
		"restore_thread", "restore_reply", "toggle_board_hidden", "reply_action",
		"edit_profile",
		"admin_agents", "agent_new":
		s.route(action)
	case "quit":
		s.sendScreen(Screen{Screen: "bye", Title: "title.goodbye",
			Lines: []Line{{Key: "msg.goodbye"}}})
		s.mu <- struct{}{}
		_ = s.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
		<-s.mu
		return
	default:
		s.sendError(id, "UNKNOWN_ACTION", "unknown action: "+action)
		return
	}
	s.sendResponse(id, map[string]any{"ok": true})
}

func (s *session) doSelect(value string) {
	for _, it := range s.menu {
		if it.Action == value || it.Key == value {
			s.route(it.Action)
			return
		}
	}
	s.route(value)
}

func (s *session) route(action string) {
	switch {
	case action == "boards":
		s.showBoards()
	case action == "help":
		s.showHelp()
	case action == "home":
		s.showHome("", "")
	case action == "login":
		s.showLogin("")
	case action == "register":
		s.showRegister("")
	case action == "lang":
		s.showLang()
	case action == "logout":
		_ = s.setUser("", "", "")
		s.showHome("flash.logged_out", "")
	case action == "drafts":
		s.showDraftList()
	case action == "profile":
		s.showProfile()
	case action == "edit_profile" && s.userID != "":
		s.showProfileEdit("")
	case action == "toggle_images":
		s.imagesOn = !s.imagesOn
		s.repaint()
	case action == "search":
		s.showSearch("")
	case action == "search_again":
		s.showSearch(s.searchQuery)
	case action == "new_thread" && s.boardID != "":
		s.showNewThread("", "", "", "")
	case action == "reply" && s.threadID != "":
		s.showReply("", "", "")
	case action == "edit_thread" && s.threadID != "":
		s.showEditThread("")
	case action == "reply_manage" && s.threadID != "":
		s.showReplyManage()
	case action == "reply_action" && s.threadID != "" && s.replyID != "":
		s.showReplyAction()
	case action == "delete_thread":
		s.prepareDeleteThread()
	case action == "delete_reply":
		s.prepareDeleteReply()
	case action == "purge_thread":
		s.preparePurgeThread()
	case action == "purge_reply":
		s.preparePurgeReply()
	case action == "delete_board":
		s.prepareDeleteBoard()
	case action == "confirm_yes":
		s.executePendingDelete()
	case action == "confirm_no":
		s.pendingDelete = nil
		s.repaint()
	case action == "restore_thread":
		s.restoreThread(s.threadID)
	case action == "restore_reply":
		s.restoreReply(s.replyID)
	case action == "toggle_board_hidden":
		s.toggleBoardHidden(s.boardID)
	case action == "admin":
		s.showAdmin()
	case action == "admin_boards":
		s.showAdminBoards()
	case action == "admin_hidden_threads":
		s.showAdminHiddenThreads()
	case action == "admin_hidden_replies":
		s.showAdminHiddenReplies()
	case action == "admin_agents":
		s.showAdminAgents()
	case action == "agent_new":
		s.showAgentNew("")
	case strings.HasPrefix(action, "agent_view:"):
		s.showAgentView(strings.TrimPrefix(action, "agent_view:"))
	case strings.HasPrefix(action, "agent_rotate:"):
		s.rotateAgentKey(strings.TrimPrefix(action, "agent_rotate:"))
	case strings.HasPrefix(action, "agent_delete:"):
		s.deleteAgent(strings.TrimPrefix(action, "agent_delete:"))
	case action == "board_new":
		s.showBoardNew("")
	case action == "back":
		s.goBack()
	case action == "quit":
		s.sendScreen(Screen{Screen: "bye", Title: "title.goodbye",
			Lines: []Line{{Key: "msg.goodbye"}}})
	case strings.HasPrefix(action, "board_admin:"):
		s.showBoardAdmin(strings.TrimPrefix(action, "board_admin:"))
	case strings.HasPrefix(action, "board:"):
		s.boardID = strings.TrimPrefix(action, "board:")
		s.page = 1
		s.showThreadList("", "")
	case strings.HasPrefix(action, "thread:"):
		s.threadID = strings.TrimPrefix(action, "thread:")
		s.replyID = ""
		s.showThreadRead()
	case strings.HasPrefix(action, "post:"):
		rest := strings.TrimPrefix(action, "post:")
		parts := strings.SplitN(rest, ":", 2)
		s.replyID = parts[0]
		if len(parts) == 2 && parts[1] != "" {
			s.threadID = parts[1]
		}
		s.showReplyAction()
	case strings.HasPrefix(action, "draft_open:"):
		s.openDraft(strings.TrimPrefix(action, "draft_open:"))
	case strings.HasPrefix(action, "draft_del:"):
		s.deleteDraft(strings.TrimPrefix(action, "draft_del:"))
	case strings.HasPrefix(action, "lang:"):
		// 客户端本地处理
	default:
		s.sendOutput("Unknown: " + action + "\r\n")
		s.repaint()
	}
}

func (s *session) doPage(dir string) {
	switch s.screen {
	case "thread_list":
		if dir == "prev" && s.page > 1 {
			s.page--
			s.showThreadList("", "")
		} else if dir == "next" {
			s.page++
			s.showThreadList("", "")
		}
	case "search_results":
		if dir == "prev" && s.searchPage > 1 {
			s.showSearchResults(s.searchQuery, s.searchPage-1)
		} else if dir == "next" {
			s.showSearchResults(s.searchQuery, s.searchPage+1)
		}
	}
}

func (s *session) goBack() {
	switch s.screen {
	case "thread_read":
		s.threadID = ""
		s.replyID = ""
		s.showThreadList("", "")
	case "reply_manage":
		s.replyID = ""
		s.showThreadRead()
	case "reply_action":
		s.replyID = ""
		s.showReplyManage()
	case "thread_list":
		s.boardID = ""
		s.showBoards()
	case "search_results":
		s.showSearch(s.searchQuery)
	case "admin":
		s.showHome("", "")
	case "admin_boards":
		s.showAdmin()
	case "admin_hidden_threads":
		s.showAdmin()
	case "admin_hidden_replies":
		s.showAdmin()
	case "board_admin":
		s.showAdminBoards()
	case "board_new":
		s.showAdminBoards()
	case "delete_confirm", "purge_confirm":
		s.pendingDelete = nil
		s.repaint()
	case "admin_agents", "agent_new", "agent_view", "agent_created":
		s.showAdminAgents()
	case "new_thread", "reply", "edit_thread":
		switch s.prevScreen {
		case "draft_list":
			s.showDraftList()
		case "thread_read":
			if s.threadID != "" {
				s.showThreadRead()
			} else {
				s.showThreadList("", "")
			}
		case "thread_list":
			s.showThreadList("", "")
		default:
			if s.threadID != "" {
				s.showThreadRead()
			} else if s.boardID != "" {
				s.showThreadList("", "")
			} else {
				s.showHome("", "")
			}
		}
		s.prevScreen = ""
	case "profile_edit":
		s.showProfile()
	case "draft_list", "profile", "lang", "login", "register", "boards", "help", "search":
		s.showHome("", "")
	default:
		s.showHome("", "")
	}
}

func (s *session) repaint() {
	switch s.screen {
	case "home":
		s.showHome("", "")
	case "login":
		s.showLogin(s.lastErr)
	case "register":
		s.showRegister(s.lastErr)
	case "boards":
		s.showBoards()
	case "thread_list":
		s.showThreadList("", "")
	case "thread_read":
		s.showThreadRead()
	case "draft_list":
		s.showDraftList()
	case "profile":
		s.showProfile()
	case "lang":
		s.showLang()
	case "help":
		s.showHelp()
	case "search":
		s.showSearch("")
	case "search_results":
		s.showSearchResults(s.searchQuery, s.searchPage)
	case "admin":
		s.showAdmin()
	case "admin_boards":
		s.showAdminBoards()
	case "admin_hidden_threads":
		s.showAdminHiddenThreads()
	case "admin_hidden_replies":
		s.showAdminHiddenReplies()
	case "admin_agents":
		s.showAdminAgents()
	case "agent_new":
		s.showAgentNew("")
	case "agent_view":
		s.showAgentView(s.username)
	case "board_admin":
		s.showBoardAdmin(s.boardID)
	case "board_new":
		s.showBoardNew("")
	case "reply_manage":
		s.showReplyManage()
	case "reply_action":
		s.showReplyAction()
	case "delete_confirm":
		s.showDeleteConfirm()
	case "purge_confirm":
		s.showPurgeConfirm()
	case "new_thread", "reply", "edit_thread", "profile_edit":
		// 表单屏不 repaint
	default:
		s.showHome("", "")
	}
}

func (s *session) sendScreen(sc Screen) {
	fillDefaults(&sc)
	raw, _ := json.Marshal(sc)
	s.sendRaw(Message{Type: "screen", Data: raw})
}

func (s *session) sendOutput(text string) {
	s.sendRaw(Message{Type: "output", Text: text})
}

func (s *session) sendError(id int64, code, msg string) {
	ok := false
	s.sendRaw(Message{ID: id, Type: "response", OK: &ok,
		Error: &ErrorInfo{Code: code, Message: msg}})
}

func (s *session) sendResponse(id int64, data any) {
	ok := true
	raw, _ := json.Marshal(data)
	s.sendRaw(Message{ID: id, Type: "response", OK: &ok, Data: raw})
}

func (s *session) sendRaw(msg Message) {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	s.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := s.conn.WriteJSON(msg); err != nil {
		log.Printf("write: %v", err)
	}
}
