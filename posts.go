package main

import (
	"encoding/json"
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
	rel, err := writePostFile(id, []byte(body))
	if err != nil {
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
	rel, err := writePostFile(id, []byte(body))
	if err != nil {
		s.showReplyErr("error.save_failed")
		return
	}
	now := time.Now().Unix()
	_, err = db.Exec(
		`INSERT INTO posts(id, thread_id, author_id, file_path, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, threadID, s.userID, rel, now, now,
	)
	if err != nil {
		_ = os.Remove(rel)
		s.showReplyErr("error.save_failed")
		return
	}
	_, _ = db.Exec(`UPDATE threads SET reply_count = reply_count + 1, updated_at = ? WHERE id = ?`, now, threadID)

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
	s.showThreadRead()
}
