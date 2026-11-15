package main

import (
	"fmt"
	"strconv"
	"strings"
)

func (s *session) showThreadList(flashKey, flashArg string) {
	s.screen = "thread_list"

	var boardName string
	_ = db.QueryRow(`SELECT name FROM boards WHERE id = ? AND hidden = 0`, s.boardID).Scan(&boardName)
	if boardName == "" {
		s.boardID = ""
		s.showBoards()
		return
	}

	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE board_id = ? AND hidden = 0`, s.boardID).Scan(&total)
	totalPage := (total + cfg.Posts.PageSize - 1) / cfg.Posts.PageSize
	if totalPage < 1 {
		totalPage = 1
	}
	if s.page > totalPage {
		s.page = totalPage
	}
	offset := (s.page - 1) * cfg.Posts.PageSize

	rows, err := db.Query(
		`SELECT t.id, t.title, t.author_id, u.username, t.updated_at, t.reply_count
		 FROM threads t JOIN users u ON u.id = t.author_id
		 WHERE t.board_id = ? AND t.hidden = 0
		 ORDER BY t.updated_at DESC LIMIT ? OFFSET ?`,
		s.boardID, cfg.Posts.PageSize, offset,
	)
	if err != nil {
		s.sendOutput("DB error: " + err.Error() + "\r\n")
		return
	}
	defer rows.Close()

	sc := s.baseScreen("thread_list", "title.board_named")
	sc.Lines = []Line{
		{Raw: boardName, Kind: "h1"},
		{Key: "msg.page", Args: []string{strconv.Itoa(s.page), strconv.Itoa(totalPage)}, Kind: "author"},
		Line{},
	}
	if flashKey != "" {
		sc.Lines = append([]Line{{Key: flashKey, Args: []string{flashArg}, Kind: "ok"}, Line{}}, sc.Lines...)
	}

	key := 1
	var menu []MenuItem
	for rows.Next() {
		var id, title, authorID, username string
		var updated int64
		var replies int
		_ = rows.Scan(&id, &title, &authorID, &username, &updated, &replies)
		label := fmt.Sprintf("%-40s %s  [%d]  %s",
			trunc(title, 40), username, replies, fmtRelTime(updated))
		menu = append(menu, MenuItem{
			Key:    strconv.Itoa(key),
			Label:  label,
			Action: "thread:" + id,
		})
		key++
	}
	if len(menu) == 0 {
		menu = append(menu, MenuItem{Key: "n", LabelKey: "menu.new_thread_empty", Action: "new_thread"})
	} else {
		menu = append(menu,
			MenuItem{Key: "n", LabelKey: "menu.new_thread", Action: "new_thread"},
			MenuItem{Key: "b", LabelKey: "menu.back_boards", Action: "boards"},
		)
	}
	sc.Menu = menu
	sc.Hint = "hint.thread_list"
	sc.Page = s.page
	sc.TotalPage = totalPage
	sc.CanPrev = s.page > 1
	sc.CanNext = s.page < totalPage
	s.sendScreen(sc)
}

func (s *session) showThreadRead() {
	s.screen = "thread_read"
	var title, authorID, filePath string
	var created int64
	err := db.QueryRow(
		`SELECT title, author_id, file_path, created_at FROM threads WHERE id = ? AND hidden = 0`,
		s.threadID,
	).Scan(&title, &authorID, &filePath, &created)
	if err != nil {
		s.threadID = ""
		s.showThreadList("", "")
		return
	}
	var authorName string
	_ = db.QueryRow(`SELECT username FROM users WHERE id = ?`, authorID).Scan(&authorName)

	body, _ := readFile(filePath)
	sc := s.baseScreen("thread_read", "title.thread")
	sc.Lines = append(sc.Lines,
		Line{Raw: title, Kind: "h1"},
		Line{Key: "msg.thread_meta", Args: []string{authorName, fmtTime(created)}, Kind: "author"},
		Line{},
	)
	sc.Lines = append(sc.Lines, markdownToLines(string(body), s.imagesOn)...)
	sc.Lines = append(sc.Lines, Line{Raw: strings.Repeat("─", 40), Kind: "hr"})

	rows, err := db.Query(
		`SELECT p.author_id, u.username, p.file_path, p.created_at
		 FROM posts p JOIN users u ON u.id = p.author_id
		 WHERE p.thread_id = ? AND p.hidden = 0 ORDER BY p.created_at ASC`, s.threadID,
	)
	if err == nil {
		defer rows.Close()
		idx := 1
		for rows.Next() {
			var aid, uname, pfile string
			var pcreated int64
			_ = rows.Scan(&aid, &uname, &pfile, &pcreated)
			pbody, _ := readFile(pfile)
			sc.Lines = append(sc.Lines,
				Line{Key: "msg.reply_meta", Args: []string{strconv.Itoa(idx), uname, fmtTime(pcreated)}, Kind: "author"},
			)
			sc.Lines = append(sc.Lines, markdownToLines(string(pbody), s.imagesOn)...)
			sc.Lines = append(sc.Lines, Line{})
			idx++
		}
	}

	menu := []MenuItem{
		{Key: "r", LabelKey: "menu.reply", Action: "reply"},
		{Key: "i", LabelKey: imagesLabelKey(s.imagesOn), Action: "toggle_images"},
		{Key: "m", LabelKey: "menu.reply_manage", Action: "reply_manage"},
	}
	if authorID == s.userID && s.userID != "" && cfg.Posts.AllowEdit {
		menu = append(menu, MenuItem{Key: "e", LabelKey: "menu.edit_thread", Action: "edit_thread"})
	}
	if s.userID != "" && (authorID == s.userID || s.isAdmin) {
		menu = append(menu, MenuItem{Key: "d", LabelKey: "menu.delete", Action: "delete_thread"})
	}
	if s.isAdmin {
		menu = append(menu, MenuItem{Key: "x", LabelKey: "menu.purge", Action: "purge_thread"})
	}
	menu = append(menu, MenuItem{Key: "b", LabelKey: "menu.back_threads", Action: "back"})
	sc.Menu = menu
	sc.Hint = "hint.thread_read"
	s.sendScreen(sc)
}

func (s *session) showNewThread(errKey, title, body, draftID string) {
	if s.screen != "new_thread" {
		s.prevScreen = s.screen
	}
	s.screen = "new_thread"
	if s.userID == "" {
		s.showLogin("error.login_required")
		return
	}
	sc := s.baseScreen("new_thread", "title.new_thread")
	sc.Lines = []Line{{Key: "msg.new_thread_prompt"}, Line{}}
	if errKey != "" {
		sc.Lines = append(sc.Lines, Line{Key: errKey, Kind: "err"}, Line{})
	}
	sc.Form = &Form{
		Fields: []FormField{
			{Name: "title", LabelKey: "field.title", Value: title},
			{Name: "body", LabelKey: "field.body", Multiline: true, Value: body},
			{Name: "draft_id", Value: draftID},
		},
		Submit: "submit", CanDraft: true,
	}
	sc.Hint = "hint.editor"
	s.sendScreen(sc)
}

func (s *session) showNewThreadErr(errKey string) { s.showNewThread(errKey, "", "", "") }

func (s *session) showReply(errKey, body, draftID string) {
	if s.screen != "reply" {
		s.prevScreen = s.screen
	}
	s.screen = "reply"
	if s.userID == "" {
		s.showLogin("error.login_required")
		return
	}
	sc := s.baseScreen("reply", "title.reply")
	sc.Lines = []Line{{Key: "msg.reply_prompt"}, Line{}}
	if errKey != "" {
		sc.Lines = append(sc.Lines, Line{Key: errKey, Kind: "err"}, Line{})
	}
	sc.Form = &Form{
		Fields: []FormField{
			{Name: "body", LabelKey: "field.reply_body", Multiline: true, Value: body},
			{Name: "draft_id", Value: draftID},
		},
		Submit: "submit", CanDraft: true,
	}
	sc.Hint = "hint.editor"
	s.sendScreen(sc)
}

func (s *session) showReplyErr(errKey string) { s.showReply(errKey, "", "") }

func (s *session) showEditThread(errKey string) {
	if s.screen != "edit_thread" {
		s.prevScreen = s.screen
	}
	s.screen = "edit_thread"
	if s.userID == "" {
		s.showLogin("error.login_required")
		return
	}
	var title, authorID, filePath string
	err := db.QueryRow(`SELECT title, author_id, file_path FROM threads WHERE id = ?`, s.threadID).
		Scan(&title, &authorID, &filePath)
	if err != nil {
		s.showThreadList("", "")
		return
	}
	if authorID != s.userID || !cfg.Posts.AllowEdit {
		s.showThreadRead()
		s.sendOutput("not allowed\r\n")
		return
	}
	body, _ := readFile(filePath)
	sc := s.baseScreen("edit_thread", "title.edit_thread")
	sc.Lines = []Line{{Key: "msg.edit_prompt"}, Line{}}
	if errKey != "" {
		sc.Lines = append(sc.Lines, Line{Key: errKey, Kind: "err"}, Line{})
	}
	sc.Form = &Form{
		Fields: []FormField{
			{Name: "title", LabelKey: "field.title", Value: title},
			{Name: "body", LabelKey: "field.body", Multiline: true, Value: string(body)},
		},
		Submit: "submit",
	}
	sc.Hint = "hint.editor"
	s.sendScreen(sc)
}

func (s *session) showEditThreadErr(errKey string) { s.showEditThread(errKey) }

func imagesLabelKey(on bool) string {
	if on {
		return "menu.images_on"
	}
	return "menu.images_off"
}
