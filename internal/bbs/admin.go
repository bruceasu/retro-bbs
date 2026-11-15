package bbs

import (
	"fmt"
	"os"
	"strings"
)

type pendingDelete struct {
	kind     string
	id       string
	title    string
	author   string
	replies  int
	threadID string
}

// ---------- 入口 ----------
func (s *session) showAdmin() {
	if !s.isAdmin {
		s.sendError(0, "FORBIDDEN", "admin only")
		s.showHome("", "")
		return
	}
	s.screen = "admin"
	sc := s.baseScreen("admin", "title.admin")
	sc.Lines = []Line{
		{Key: "msg.admin_prompt"}, Line{},
	}
	var hiddenThreads, hiddenReplies, agentCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE hidden = 1`).Scan(&hiddenThreads)
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts WHERE hidden = 1`).Scan(&hiddenReplies)
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_agent = 1 AND status = 'active'`).Scan(&agentCount)
	sc.Lines = append(sc.Lines,
		Line{Key: "msg.admin_hidden_counts", Args: []string{itoa(hiddenThreads), itoa(hiddenReplies)}, Kind: "author"},
		Line{Key: "msg.admin_agent_count", Args: []string{itoa(agentCount)}, Kind: "author"},
		Line{},
	)
	sc.Menu = []MenuItem{
		{Key: "b", LabelKey: "menu.admin_boards", Action: "admin_boards"},
		{Key: "t", LabelKey: "menu.admin_hidden_threads", Action: "admin_hidden_threads"},
		{Key: "r", LabelKey: "menu.admin_hidden_replies", Action: "admin_hidden_replies"},
		{Key: "a", LabelKey: "menu.admin_agents", Action: "admin_agents"},
		{Key: "q", LabelKey: "menu.back_home", Action: "home"},
	}
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

// ---------- 面板管理 ----------

func (s *session) showAdminBoards() {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.screen = "admin_boards"
	rows, err := db.Query(`SELECT id, name, description, hidden FROM boards ORDER BY id`)
	if err != nil {
		s.sendError(0, "DB", err.Error())
		return
	}
	defer rows.Close()

	sc := s.baseScreen("admin_boards", "title.admin_boards")
	sc.Lines = []Line{{Key: "msg.admin_boards_prompt"}, Line{}}

	var menu []MenuItem
	key := 1
	for rows.Next() {
		var id, name, desc string
		var hidden int
		_ = rows.Scan(&id, &name, &desc, &hidden)
		var threads int
		_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE board_id = ?`, id).Scan(&threads)
		mark := ""
		if hidden == 1 {
			mark = " [hidden]"
		}
		label := fmt.Sprintf("%-10s %-20s %d threads%s", id, name, threads, mark)
		menu = append(menu, MenuItem{
			Key:    itoa(key),
			Label:  label,
			Action: "board_admin:" + id,
		})
		key++
	}
	menu = append(menu,
		MenuItem{Key: "n", LabelKey: "menu.board_new", Action: "board_new"},
		MenuItem{Key: "q", LabelKey: "menu.back_home", Action: "admin"},
	)
	sc.Menu = menu
	sc.Hint = "hint.admin_boards"
	s.sendScreen(sc)
}

// showBoardAdmin 单个面板详情，含切换显隐 / 删除空板。
func (s *session) showBoardAdmin(boardID string) {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.boardID = boardID
	s.screen = "board_admin"

	var name, desc string
	var hidden int
	if err := db.QueryRow(`SELECT name, description, hidden FROM boards WHERE id = ?`, boardID).
		Scan(&name, &desc, &hidden); err != nil {
		s.showAdminBoards()
		return
	}
	var threads int
	_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE board_id = ?`, boardID).Scan(&threads)

	sc := s.baseScreen("board_admin", "title.board_admin")
	sc.Lines = []Line{
		{Key: "msg.board_id", Args: []string{boardID}, Kind: "info"},
		{Key: "msg.board_name", Args: []string{name}, Kind: "info"},
		{Key: "msg.board_desc", Args: []string{desc}, Kind: "author"},
		{Key: "msg.board_threads", Args: []string{itoa(threads)}, Kind: "author"},
	}
	if hidden == 1 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.board_hidden", Kind: "err"})
	}
	sc.Lines = append(sc.Lines, Line{})

	toggleKey := "menu.toggle_hidden"
	if hidden == 1 {
		toggleKey = "menu.toggle_show"
	}
	menu := []MenuItem{
		{Key: "t", LabelKey: toggleKey, Action: "toggle_board_hidden"},
	}
	if threads == 0 {
		menu = append(menu, MenuItem{Key: "d", LabelKey: "menu.board_delete", Action: "delete_board"})
	}
	menu = append(menu, MenuItem{Key: "q", LabelKey: "menu.back_home", Action: "admin_boards"})
	sc.Menu = menu
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func (s *session) showBoardNew(errKey string) {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.screen = "board_new"
	sc := s.baseScreen("board_new", "title.board_new")
	sc.Lines = []Line{{Key: "msg.board_new_prompt"}, Line{}}
	if errKey != "" {
		sc.Lines = append(sc.Lines, Line{Key: errKey, Kind: "err"}, Line{})
	}
	sc.Form = &Form{Fields: []FormField{
		{Name: "board_id", LabelKey: "field.board_id"},
		{Name: "board_name", LabelKey: "field.board_name"},
		{Name: "board_desc", LabelKey: "field.board_desc", Multiline: true},
	}, Submit: "submit"}
	sc.Hint = "hint.editor"
	s.sendScreen(sc)
}

func (s *session) toggleBoardHidden(boardID string) {
	if !s.isAdmin || boardID == "" {
		return
	}
	var hidden int
	if err := db.QueryRow(`SELECT hidden FROM boards WHERE id = ?`, boardID).Scan(&hidden); err != nil {
		s.showAdminBoards()
		return
	}
	newVal := 0
	if hidden == 0 {
		newVal = 1
	}
	_, _ = db.Exec(`UPDATE boards SET hidden = ? WHERE id = ?`, newVal, boardID)
	s.showBoardAdmin(boardID)
}

func (s *session) prepareDeleteBoard() {
	if !s.isAdmin || s.boardID == "" {
		return
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM boards WHERE id = ?`, s.boardID).Scan(&name); err != nil {
		s.showAdminBoards()
		return
	}
	var threads int
	_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE board_id = ?`, s.boardID).Scan(&threads)
	if threads > 0 {
		s.sendOutput("cannot delete non-empty board\r\n")
		s.showBoardAdmin(s.boardID)
		return
	}
	s.pendingDelete = &pendingDelete{
		kind:  "board",
		id:    s.boardID,
		title: name,
	}
	s.showDeleteConfirm()
}

// ---------- 隐藏内容列表 ----------

func (s *session) showAdminHiddenThreads() {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.screen = "admin_hidden_threads"
	sc := s.baseScreen("admin_hidden_threads", "title.admin_hidden_threads")
	sc.Lines = []Line{{Key: "msg.admin_hidden_threads_prompt"}, Line{}}

	rows, err := db.Query(
		`SELECT t.id, t.title, u.username, t.board_id
		 FROM threads t JOIN users u ON u.id = t.author_id
		 WHERE t.hidden = 1 ORDER BY t.updated_at DESC`)
	if err != nil {
		s.sendError(0, "DB", err.Error())
		return
	}
	defer rows.Close()

	var menu []MenuItem
	key := 1
	for rows.Next() {
		var id, title, author, boardID string
		_ = rows.Scan(&id, &title, &author, &boardID)
		label := fmt.Sprintf("%-30s %s  %s", trunc(title, 30), author, boardID)
		menu = append(menu, MenuItem{
			Key:    itoa(key),
			Label:  label,
			Action: "thread:" + id,
		})
		key++
	}
	if len(menu) == 0 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.no_hidden", Kind: "author"}, Line{})
	}
	menu = append(menu, MenuItem{Key: "q", LabelKey: "menu.back_home", Action: "admin"})
	sc.Menu = menu
	sc.Hint = "hint.admin_hidden"
	s.sendScreen(sc)
}

func (s *session) showAdminHiddenReplies() {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.screen = "admin_hidden_replies"
	sc := s.baseScreen("admin_hidden_replies", "title.admin_hidden_replies")
	sc.Lines = []Line{{Key: "msg.admin_hidden_replies_prompt"}, Line{}}

	rows, err := db.Query(
		`SELECT p.id, p.thread_id, u.username, t.title, p.created_at
		 FROM posts p JOIN users u ON u.id = p.author_id
		 JOIN threads t ON t.id = p.thread_id
		 WHERE p.hidden = 1 ORDER BY p.created_at DESC LIMIT 100`)
	if err != nil {
		s.sendError(0, "DB", err.Error())
		return
	}
	defer rows.Close()

	var menu []MenuItem
	key := 1
	for rows.Next() {
		var id, threadID, author, threadTitle string
		var created int64
		_ = rows.Scan(&id, &threadID, &author, &threadTitle, &created)
		label := fmt.Sprintf("%s  %s  re: %s", author, fmtRelTime(created), trunc(threadTitle, 30))
		menu = append(menu, MenuItem{
			Key:    itoa(key),
			Label:  label,
			Action: "post:" + id + ":" + threadID,
		})
		key++
	}
	if len(menu) == 0 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.no_hidden", Kind: "author"}, Line{})
	}
	menu = append(menu, MenuItem{Key: "q", LabelKey: "menu.back_home", Action: "admin"})
	sc.Menu = menu
	sc.Hint = "hint.admin_hidden"
	s.sendScreen(sc)
}

func (s *session) restoreThread(threadID string) {
	if !s.isAdmin {
		return
	}
	_, _ = db.Exec(`UPDATE threads SET hidden = 0 WHERE id = ?`, threadID)
	_ = indexThread(threadID)
	s.showAdminHiddenThreads()
}

func (s *session) restoreReply(postID string) {
	if !s.isAdmin {
		return
	}
	var threadID string
	if err := db.QueryRow(`SELECT thread_id FROM posts WHERE id = ?`, postID).Scan(&threadID); err != nil {
		s.showAdminHiddenReplies()
		return
	}
	_, _ = db.Exec(`UPDATE posts SET hidden = 0 WHERE id = ?`, postID)
	_, _ = db.Exec(`UPDATE threads SET reply_count = (SELECT COUNT(*) FROM posts WHERE thread_id = ? AND hidden = 0) WHERE id = ?`, threadID, threadID)
	_ = indexPost(postID)
	s.showAdminHiddenReplies()
}

// ---------- 准备删除 ----------

func (s *session) prepareDeleteThread() {
	if s.threadID == "" {
		return
	}
	var title, authorID, authorName string
	var replies int
	err := db.QueryRow(
		`SELECT t.title, t.author_id, u.username,
		        (SELECT COUNT(*) FROM posts WHERE thread_id = t.id AND hidden = 0)
		 FROM threads t JOIN users u ON u.id = t.author_id
		 WHERE t.id = ? AND t.hidden = 0`, s.threadID,
	).Scan(&title, &authorID, &authorName, &replies)
	if err != nil {
		s.showThreadList("", "")
		return
	}
	if authorID != s.userID && !s.isAdmin {
		s.sendOutput("not allowed\r\n")
		return
	}
	s.pendingDelete = &pendingDelete{
		kind:    "thread",
		id:      s.threadID,
		title:   title,
		author:  authorName,
		replies: replies,
	}
	s.showDeleteConfirm()
}

func (s *session) prepareDeleteReply() {
	if s.threadID == "" || s.replyID == "" {
		return
	}
	var authorID, authorName string
	err := db.QueryRow(
		`SELECT p.author_id, u.username FROM posts p JOIN users u ON u.id = p.author_id
		 WHERE p.id = ? AND p.thread_id = ? AND p.hidden = 0`, s.replyID, s.threadID,
	).Scan(&authorID, &authorName)
	if err != nil {
		s.showReplyManage()
		return
	}
	if authorID != s.userID && !s.isAdmin {
		s.sendOutput("not allowed\r\n")
		return
	}
	var threadTitle string
	_ = db.QueryRow(`SELECT title FROM threads WHERE id = ?`, s.threadID).Scan(&threadTitle)
	s.pendingDelete = &pendingDelete{
		kind:     "reply",
		id:       s.replyID,
		title:    threadTitle,
		author:   authorName,
		threadID: s.threadID,
	}
	s.showDeleteConfirm()
}

func (s *session) preparePurgeThread() {
	if !s.isAdmin || s.threadID == "" {
		return
	}
	var title, authorName string
	var replies int
	err := db.QueryRow(
		`SELECT t.title, u.username,
		        (SELECT COUNT(*) FROM posts WHERE thread_id = t.id)
		 FROM threads t JOIN users u ON u.id = t.author_id
		 WHERE t.id = ?`, s.threadID,
	).Scan(&title, &authorName, &replies)
	if err != nil {
		s.showThreadList("", "")
		return
	}
	s.pendingDelete = &pendingDelete{
		kind:    "purge_thread",
		id:      s.threadID,
		title:   title,
		author:  authorName,
		replies: replies,
	}
	s.showPurgeConfirm()
}

func (s *session) preparePurgeReply() {
	if !s.isAdmin || s.threadID == "" || s.replyID == "" {
		return
	}
	var authorName string
	if err := db.QueryRow(
		`SELECT u.username FROM posts p JOIN users u ON u.id = p.author_id WHERE p.id = ?`,
		s.replyID,
	).Scan(&authorName); err != nil {
		s.showReplyManage()
		return
	}
	var threadTitle string
	_ = db.QueryRow(`SELECT title FROM threads WHERE id = ?`, s.threadID).Scan(&threadTitle)
	s.pendingDelete = &pendingDelete{
		kind:     "purge_reply",
		id:       s.replyID,
		title:    threadTitle,
		author:   authorName,
		threadID: s.threadID,
	}
	s.showPurgeConfirm()
}

// ---------- 确认屏 ----------

func (s *session) showDeleteConfirm() {
	if s.pendingDelete == nil {
		s.repaint()
		return
	}
	s.screen = "delete_confirm"
	sc := s.baseScreen("delete_confirm", "title.delete_confirm")
	p := s.pendingDelete

	key := "msg.confirm_delete_thread"
	if p.kind == "reply" {
		key = "msg.confirm_delete_reply"
	} else if p.kind == "board" {
		key = "msg.confirm_delete_board"
	}
	sc.Lines = []Line{
		{Key: key, Kind: "err"}, Line{},
		{Key: "msg.delete_title", Args: []string{p.title}, Kind: "info"},
		{Key: "msg.delete_author", Args: []string{p.author}, Kind: "author"},
	}
	if p.kind == "thread" {
		sc.Lines = append(sc.Lines, Line{Key: "msg.delete_replies", Args: []string{itoa(p.replies)}, Kind: "author"})
	}
	sc.Lines = append(sc.Lines, Line{})
	sc.Menu = []MenuItem{
		{Key: "y", LabelKey: "menu.confirm_yes", Action: "confirm_yes"},
		{Key: "n", LabelKey: "menu.confirm_no", Action: "confirm_no"},
	}
	sc.Hint = "hint.confirm"
	s.sendScreen(sc)
}

func (s *session) showPurgeConfirm() {
	if s.pendingDelete == nil {
		s.repaint()
		return
	}
	s.screen = "purge_confirm"
	sc := s.baseScreen("purge_confirm", "title.purge_confirm")
	p := s.pendingDelete

	key := "msg.confirm_purge_thread"
	if p.kind == "purge_reply" {
		key = "msg.confirm_purge_reply"
	}
	sc.Lines = []Line{
		{Key: key, Kind: "err"}, Line{},
		{Key: "msg.delete_title", Args: []string{p.title}, Kind: "info"},
		{Key: "msg.delete_author", Args: []string{p.author}, Kind: "author"},
	}
	if p.kind == "purge_thread" {
		sc.Lines = append(sc.Lines, Line{Key: "msg.delete_replies", Args: []string{itoa(p.replies)}, Kind: "author"})
	}
	sc.Lines = append(sc.Lines, Line{}, Line{Key: "msg.purge_warning", Kind: "err"})
	sc.Menu = []MenuItem{
		{Key: "y", LabelKey: "menu.confirm_yes", Action: "confirm_yes"},
		{Key: "n", LabelKey: "menu.confirm_no", Action: "confirm_no"},
	}
	sc.Hint = "hint.confirm"
	s.sendScreen(sc)
}

func (s *session) executePendingDelete() {
	p := s.pendingDelete
	s.pendingDelete = nil
	if p == nil {
		s.repaint()
		return
	}

	switch p.kind {
	case "thread":
		s.softDeleteThread(p.id)
		s.threadID = ""
		s.showThreadList("flash.thread_deleted", "")
	case "reply":
		s.softDeleteReply(p.id, p.threadID)
		s.replyID = ""
		s.threadID = p.threadID
		s.showThreadRead()
	case "purge_thread":
		s.hardDeleteThread(p.id)
		s.threadID = ""
		s.showThreadList("flash.thread_purged", "")
	case "purge_reply":
		s.hardDeleteReply(p.id, p.threadID)
		s.replyID = ""
		s.threadID = p.threadID
		s.showThreadRead()
	case "board":
		_, _ = db.Exec(`DELETE FROM boards WHERE id = ?`, p.id)
		s.boardID = ""
		s.showAdminBoards()
	}
}

// ---------- 软删除 ----------

func (s *session) softDeleteThread(threadID string) {
	_, _ = db.Exec(`UPDATE threads SET hidden = 1 WHERE id = ?`, threadID)
	unindexByRef("thread", threadID)
}

func (s *session) softDeleteReply(postID, threadID string) {
	_, _ = db.Exec(`UPDATE posts SET hidden = 1 WHERE id = ?`, postID)
	_, _ = db.Exec(`UPDATE threads SET reply_count = (SELECT COUNT(*) FROM posts WHERE thread_id = ? AND hidden = 0) WHERE id = ?`, threadID, threadID)
	unindexByRef("post", postID)
}

// ---------- 永久删除 ----------

func (s *session) hardDeleteThread(threadID string) {
	var filePath string
	if err := db.QueryRow(`SELECT file_path FROM threads WHERE id = ?`, threadID).Scan(&filePath); err == nil {
		_ = os.Remove(filePath)
	}
	if filePath != "" {
		replyDir := strings.TrimSuffix(filePath, ".md") + ".replies"
		_ = os.RemoveAll(replyDir)
	}
	rows, _ := db.Query(`SELECT file_path FROM posts WHERE thread_id = ?`, threadID)
	if rows != nil {
		for rows.Next() {
			var p string
			_ = rows.Scan(&p)
			_ = os.Remove(p)
		}
		rows.Close()
	}
	var postIDs []string
	pRows, _ := db.Query(`SELECT id FROM posts WHERE thread_id = ?`, threadID)
	if pRows != nil {
		for pRows.Next() {
			var id string
			_ = pRows.Scan(&id)
			postIDs = append(postIDs, id)
		}
		pRows.Close()
	}
	for _, pid := range postIDs {
		unindexByRef("post", pid)
	}
	_, _ = db.Exec(`DELETE FROM posts WHERE thread_id = ?`, threadID)
	_, _ = db.Exec(`DELETE FROM threads WHERE id = ?`, threadID)
	unindexByRef("thread", threadID)
}

func (s *session) hardDeleteReply(postID, threadID string) {
	var filePath string
	if err := db.QueryRow(`SELECT file_path FROM posts WHERE id = ?`, postID).Scan(&filePath); err == nil {
		_ = os.Remove(filePath)
	}
	_, _ = db.Exec(`DELETE FROM posts WHERE id = ?`, postID)
	_, _ = db.Exec(`UPDATE threads SET reply_count = (SELECT COUNT(*) FROM posts WHERE thread_id = ? AND hidden = 0) WHERE id = ?`, threadID, threadID)
	unindexByRef("post", postID)
}

// ---------- 回复管理屏 ----------

func (s *session) showReplyManage() {
	if s.threadID == "" {
		s.showThreadList("", "")
		return
	}
	s.screen = "reply_manage"
	sc := s.baseScreen("reply_manage", "title.reply_manage")
	sc.Lines = []Line{{Key: "msg.reply_manage_prompt"}, Line{}}

	rows, err := db.Query(
		`SELECT p.id, p.author_id, u.username, p.created_at, p.hidden
		 FROM posts p JOIN users u ON u.id = p.author_id
		 WHERE p.thread_id = ? ORDER BY p.created_at ASC`, s.threadID)
	if err != nil {
		s.sendError(0, "DB", err.Error())
		return
	}
	defer rows.Close()

	var menu []MenuItem
	key := 1
	for rows.Next() {
		var id, authorID, authorName string
		var created int64
		var hidden int
		_ = rows.Scan(&id, &authorID, &authorName, &created, &hidden)
		mark := ""
		if hidden == 1 {
			mark = " [hidden]"
		}
		mine := ""
		if authorID == s.userID {
			mine = " (you)"
		}
		label := fmt.Sprintf("#%d %s%s  %s%s", key, authorName, mine, fmtRelTime(created), mark)
		menu = append(menu, MenuItem{
			Key:    itoa(key),
			Label:  label,
			Action: "post:" + id,
		})
		key++
	}
	if len(menu) == 0 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.no_replies", Kind: "author"}, Line{})
	}
	menu = append(menu, MenuItem{Key: "q", LabelKey: "menu.back_threads", Action: "back"})
	sc.Menu = menu
	sc.Hint = "hint.reply_manage"
	s.sendScreen(sc)
}

// showReplyAction 单条回复的操作屏。
func (s *session) showReplyAction() {
	if s.threadID == "" || s.replyID == "" {
		s.showThreadRead()
		return
	}
	s.screen = "reply_action"

	var authorID, authorName string
	var hidden int
	if err := db.QueryRow(
		`SELECT p.author_id, u.username, p.hidden
		 FROM posts p JOIN users u ON u.id = p.author_id
		 WHERE p.id = ? AND p.thread_id = ?`, s.replyID, s.threadID,
	).Scan(&authorID, &authorName, &hidden); err != nil {
		s.showReplyManage()
		return
	}

	sc := s.baseScreen("reply_action", "title.reply_action")
	sc.Lines = []Line{
		{Key: "msg.reply_author", Args: []string{authorName}, Kind: "info"},
	}
	if hidden == 1 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.reply_hidden", Kind: "err"})
	}
	sc.Lines = append(sc.Lines, Line{})

	menu := []MenuItem{}
	if hidden == 1 && s.isAdmin {
		menu = append(menu, MenuItem{Key: "r", LabelKey: "menu.restore", Action: "restore_reply"})
		menu = append(menu, MenuItem{Key: "x", LabelKey: "menu.purge", Action: "purge_reply"})
	} else if hidden == 0 {
		if authorID == s.userID || s.isAdmin {
			menu = append(menu, MenuItem{Key: "d", LabelKey: "menu.delete", Action: "delete_reply"})
		}
		if s.isAdmin {
			menu = append(menu, MenuItem{Key: "x", LabelKey: "menu.purge", Action: "purge_reply"})
		}
	}
	menu = append(menu, MenuItem{Key: "q", LabelKey: "menu.back_home", Action: "reply_manage"})
	sc.Menu = menu
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}
