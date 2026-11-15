package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"
)

func (s *session) handleSubmit(valueJSON string) {
	var fields map[string]string
	if err := json.Unmarshal([]byte(valueJSON), &fields); err != nil {
		s.sendError(0, "BAD_FORM", "invalid form data")
		return
	}
	switch s.screen {
	case "login":
		if err := s.doLogin(fields["username"], fields["password"]); err != nil {
			s.showLogin(err.Error())
			return
		}
		s.showHome("flash.welcome_back", s.nickname)
	case "register":
		if err := s.doRegister(fields["username"], fields["password"]); err != nil {
			s.showRegister(err.Error())
			return
		}
		s.showHome("flash.account_created", s.nickname)
	case "new_thread":
		s.submitNewThread(fields["title"], fields["body"], fields["draft_id"])
	case "reply":
		s.submitReply(fields["body"], fields["draft_id"])
	case "edit_thread":
		s.submitEditThread(fields["title"], fields["body"])
	case "profile_edit":
		s.submitProfile(fields["nickname"], fields["bio"])
	case "search":
		q := strings.TrimSpace(fields["query"])
		if q == "" {
			s.showSearch("error.query_required")
			return
		}
		s.showSearchResults(q, 1)
	case "board_new":
		s.submitBoardNew(fields["board_id"], fields["board_name"], fields["board_desc"])
	default:
		s.sendOutput("No form on this screen.\r\n")
		s.repaint()
	}
}

func (s *session) submitNewThread(title, body, draftID string) {
	if s.userID == "" {
		s.showNewThreadErr("error.login_required")
		return
	}
	if s.boardID == "" {
		s.showNewThreadErr("error.board_required")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		s.showNewThreadErr("error.title_required")
		return
	}
	if len(title) > cfg.Posts.TitleMaxSize {
		s.showNewThreadErr("error.title_too_long")
		return
	}
	if len(body) == 0 {
		s.showNewThreadErr("error.body_required")
		return
	}
	if len(body) > cfg.Posts.MaxSize {
		s.showNewThreadErr("error.body_too_long")
		return
	}

	boardID := s.boardID
	id := newID("t")
	rel, err := writeThreadFile(id, []byte(body))
	if err != nil {
		log.Printf("write thread file: %v", err)
		s.showNewThreadErr("error.save_failed")
		return
	}
	now := time.Now().Unix()
	_, err = db.Exec(
		`INSERT INTO threads(id, board_id, author_id, title, file_path, created_at, updated_at, reply_count, hidden)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0)`,
		id, boardID, s.userID, title, rel, now, now,
	)
	if err != nil {
		_ = os.Remove(rel)
		s.showNewThreadErr("error.save_failed")
		return
	}
	if err := indexThread(id); err != nil {
		log.Printf("index thread %s: %v", id, err)
	}

	if draftID != "" {
		s.removeDraft(draftID)
	} else {
		s.removeDraftsForContext(boardID, "")
	}

	s.threadID = id
	s.page = 1
	s.showThreadRead()
}

func (s *session) submitReply(body, draftID string) {
	if s.userID == "" {
		s.showReplyErr("error.login_required")
		return
	}
	if s.threadID == "" {
		s.showReplyErr("error.thread_required")
		return
	}
	if len(body) == 0 {
		s.showReplyErr("error.body_required")
		return
	}
	if len(body) > cfg.Posts.MaxSize {
		s.showReplyErr("error.body_too_long")
		return
	}

	threadID := s.threadID
	id := newID("p")
	rel, err := writeReplyFile(threadID, id, []byte(body))
	if err != nil {
		log.Printf("write reply file: %v", err)
		s.showReplyErr("error.save_failed")
		return
	}
	now := time.Now().Unix()
	_, err = db.Exec(
		`INSERT INTO posts(id, thread_id, author_id, file_path, created_at, updated_at, hidden)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`,
		id, threadID, s.userID, rel, now, now,
	)
	if err != nil {
		_ = os.Remove(rel)
		s.showReplyErr("error.save_failed")
		return
	}
	_, _ = db.Exec(`UPDATE threads SET reply_count = reply_count + 1, updated_at = ? WHERE id = ?`, now, threadID)
	if err := indexPost(id); err != nil {
		log.Printf("index post %s: %v", id, err)
	}

	if draftID != "" {
		s.removeDraft(draftID)
	} else {
		s.removeDraftsForContext(s.boardID, threadID)
	}

	s.showThreadRead()
}

func (s *session) submitEditThread(title, body string) {
	if s.userID == "" {
		s.showEditThreadErr("error.login_required")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		s.showEditThreadErr("error.title_required")
		return
	}
	if len(title) > cfg.Posts.TitleMaxSize {
		s.showEditThreadErr("error.title_too_long")
		return
	}
	if len(body) == 0 {
		s.showEditThreadErr("error.body_required")
		return
	}
	if len(body) > cfg.Posts.MaxSize {
		s.showEditThreadErr("error.body_too_long")
		return
	}

	var authorID, filePath string
	err := db.QueryRow(`SELECT author_id, file_path FROM threads WHERE id = ?`, s.threadID).
		Scan(&authorID, &filePath)
	if err != nil {
		s.showEditThreadErr("error.not_found")
		return
	}
	if authorID != s.userID || !cfg.Posts.AllowEdit {
		s.showEditThreadErr("error.not_owner")
		return
	}

	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		s.showEditThreadErr("error.save_failed")
		return
	}
	if err := os.Rename(tmp, filePath); err != nil {
		_ = os.Remove(tmp)
		s.showEditThreadErr("error.save_failed")
		return
	}
	if _, err := db.Exec(`UPDATE threads SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().Unix(), s.threadID); err != nil {
		s.showEditThreadErr("error.save_failed")
		return
	}
	if err := indexThread(s.threadID); err != nil {
		log.Printf("reindex thread %s: %v", s.threadID, err)
	}
	s.showThreadRead()
}

// submitBoardNew 管理员创建新面板。
func (s *session) submitBoardNew(boardID, name, desc string) {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	boardID = strings.ToLower(strings.TrimSpace(boardID))
	name = strings.TrimSpace(name)
	desc = strings.TrimSpace(desc)

	if !isValidBoardID(boardID) {
		s.showBoardNew("error.board_id_invalid")
		return
	}
	if name == "" {
		s.showBoardNew("error.board_name_required")
		return
	}
	if len(name) > 40 {
		s.showBoardNew("error.board_name_too_long")
		return
	}
	if len(desc) > 200 {
		s.showBoardNew("error.board_desc_too_long")
		return
	}
	var exists int
	_ = db.QueryRow(`SELECT COUNT(*) FROM boards WHERE id = ?`, boardID).Scan(&exists)
	if exists > 0 {
		s.showBoardNew("error.board_exists")
		return
	}
	_, err := db.Exec(
		`INSERT INTO boards(id, name, description, created_at, hidden) VALUES (?, ?, ?, ?, 0)`,
		boardID, name, desc, time.Now().Unix())
	if err != nil {
		s.showBoardNew("error.save_failed")
		return
	}
	s.showAdminBoards()
}

// isValidBoardID 只允许 2-20 字符的 a-z 0-9 _ -
func isValidBoardID(id string) bool {
	if len(id) < 2 || len(id) > 20 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
