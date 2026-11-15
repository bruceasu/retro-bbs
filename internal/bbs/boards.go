package bbs

import (
	"fmt"
	"strconv"
)

func (s *session) baseScreen(name, titleKey string) Screen {
	return Screen{Screen: name, Title: titleKey, ImagesOn: s.imagesOn}
}

func (s *session) showHome(flashKey, flashArg string) {
	s.screen = "home"
	s.threadID = ""
	sc := s.baseScreen("home", "title.home")
	if s.username != "" {
		sc.Lines = append(sc.Lines,
			Line{Key: "msg.logged_in_as", Args: []string{s.nickname}, Kind: "ok"}, Line{})
	}
	if flashKey != "" {
		sc.Lines = append(sc.Lines,
			Line{Key: flashKey, Args: []string{flashArg}, Kind: "ok"}, Line{})
	}
	sc.Lines = append(sc.Lines, Line{Key: "msg.welcome"}, Line{})

	menu := []MenuItem{
		{Key: "b", LabelKey: "menu.boards", Action: "boards"},
		{Key: "s", LabelKey: "menu.search", Action: "search"},
		{Key: "d", LabelKey: "menu.drafts", Action: "drafts"},
		{Key: "g", LabelKey: "menu.language", Action: "lang"},
		{Key: "h", LabelKey: "menu.help", Action: "help"},
	}
	if s.userID == "" {
		menu = append(menu,
			MenuItem{Key: "l", LabelKey: "menu.login", Action: "login"},
			MenuItem{Key: "r", LabelKey: "menu.register", Action: "register"},
		)
	} else {
		menu = append(menu,
			MenuItem{Key: "p", LabelKey: "menu.profile", Action: "profile"},
		)
		if s.isAdmin {
			menu = append(menu, MenuItem{Key: "a", LabelKey: "menu.admin", Action: "admin"})
		}
		menu = append(menu, MenuItem{Key: "o", LabelKey: "menu.logout", Action: "logout"})
	}
	menu = append(menu, MenuItem{Key: "q", LabelKey: "menu.quit", Action: "quit"})
	sc.Menu = menu
	sc.Hint = "hint.main"
	s.sendScreen(sc)
}

func (s *session) showLang() {
	s.screen = "lang"
	sc := s.baseScreen("lang", "title.language")
	sc.Lines = []Line{{Key: "msg.choose_language"}, Line{}}
	sc.Menu = []MenuItem{
		{Key: "1", Label: "English", Action: "lang:en"},
		{Key: "2", Label: "中文", Action: "lang:zh"},
		{Key: "b", LabelKey: "menu.back_home", Action: "home"},
	}
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func (s *session) showLogin(errMsg string) {
	s.screen = "login"
	s.lastErr = errMsg
	sc := s.baseScreen("login", "title.login")
	sc.Lines = []Line{{Key: "msg.login_prompt"}, Line{}}
	if errMsg != "" {
		sc.Lines = append(sc.Lines, Line{Key: errMsg, Kind: "err"}, Line{})
	}
	sc.Form = &Form{Fields: []FormField{
		{Name: "username", LabelKey: "field.username"},
		{Name: "password", LabelKey: "field.password", Secret: true},
	}, Submit: "submit"}
	sc.Hint = "hint.login"
	s.sendScreen(sc)
}

func (s *session) showRegister(errMsg string) {
	s.screen = "register"
	s.lastErr = errMsg
	sc := s.baseScreen("register", "title.register")
	sc.Lines = []Line{{Key: "msg.register_prompt"}, Line{}}
	if errMsg != "" {
		sc.Lines = append(sc.Lines, Line{Key: errMsg, Kind: "err"}, Line{})
	}
	sc.Form = &Form{Fields: []FormField{
		{Name: "username", LabelKey: "field.username"},
		{Name: "password", LabelKey: "field.password_rule", Secret: true},
	}, Submit: "submit"}
	sc.Hint = "hint.login"
	s.sendScreen(sc)
}

func (s *session) showBoards() {
	s.screen = "boards"
	s.boardID = ""
	s.threadID = ""

	rows, err := db.Query(`SELECT id, name, description FROM boards WHERE hidden = 0 ORDER BY id`)
	if err != nil {
		s.sendOutput("DB error: " + err.Error() + "\r\n")
		return
	}
	defer rows.Close()

	sc := s.baseScreen("boards", "title.boards")
	sc.Lines = []Line{{Key: "msg.choose_board"}, Line{}}

	key := 1
	var menu []MenuItem
	for rows.Next() {
		var id, name, desc string
		_ = rows.Scan(&id, &name, &desc)
		menu = append(menu, MenuItem{
			Key:    strconv.Itoa(key),
			Label:  fmt.Sprintf("%-8s - %s", name, desc),
			Action: "board:" + id,
		})
		key++
	}
	menu = append(menu, MenuItem{Key: "b", LabelKey: "menu.back_home", Action: "home"})
	sc.Menu = menu
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func (s *session) showHelp() {
	s.screen = "help"
	sc := s.baseScreen("help", "title.help")
	sc.Lines = []Line{
		{Key: "help.keys_title"}, Line{},
		{Key: "help.updown"}, {Key: "help.enter"}, {Key: "help.esc"},
		{Key: "help.leftright"}, {Key: "help.n"}, {Key: "help.r"}, {Key: "help.e"},
		{Key: "help.search"}, {Key: "help.bhq"}, {Key: "help.lang"}, {Key: "help.images"}, Line{},
		{Key: "help.markdown"}, {Key: "help.editor"}, Line{},
		{Key: "help.max_size", Args: []string{strconv.Itoa(cfg.Posts.MaxSize)}},
	}
	sc.Menu = []MenuItem{{Key: "b", LabelKey: "menu.back_home", Action: "home"}}
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func (s *session) showDraftList() {
	s.screen = "draft_list"
	sc := s.baseScreen("draft_list", "title.drafts")
	sc.Lines = []Line{{Key: "msg.choose_draft"}, Line{}}

	if s.userID == "" {
		sc.Lines = append(sc.Lines, Line{Key: "error.login_required", Kind: "err"})
		sc.Menu = []MenuItem{
			{Key: "l", LabelKey: "menu.login", Action: "login"},
			{Key: "b", LabelKey: "menu.back_home", Action: "home"},
		}
		sc.Hint = "hint.select"
		s.sendScreen(sc)
		return
	}

	rows, err := db.Query(
		`SELECT id, board_id, thread_id, title, updated_at FROM drafts WHERE user_id = ? ORDER BY updated_at DESC`,
		s.userID,
	)
	if err != nil {
		s.sendOutput("DB error: " + err.Error() + "\r\n")
		return
	}
	defer rows.Close()

	key := 1
	var menu []MenuItem
	for rows.Next() {
		var id, boardID, threadID, title string
		var updated int64
		_ = rows.Scan(&id, &boardID, &threadID, &title, &updated)
		label := "draft"
		if title != "" {
			label = trunc(title, 40)
		} else if threadID != "" {
			label = "reply to thread"
		}
		menu = append(menu, MenuItem{
			Key:    strconv.Itoa(key),
			Label:  fmt.Sprintf("%-40s %s", label, fmtRelTime(updated)),
			Action: "draft_open:" + id,
		})
		key++
	}
	if len(menu) == 0 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.no_drafts"})
	}
	menu = append(menu, MenuItem{Key: "b", LabelKey: "menu.back_home", Action: "home"})
	sc.Menu = menu
	sc.Hint = "hint.draft_list"
	s.sendScreen(sc)
}

// ---------- 搜索屏 ----------

func (s *session) showSearch(prefill string) {
	s.screen = "search"
	sc := s.baseScreen("search", "title.search")
	sc.Lines = []Line{
		{Key: "msg.search_prompt"}, Line{},
		{Key: "msg.search_note", Kind: "author"}, Line{},
	}
	sc.Form = &Form{
		Fields: []FormField{
			{Name: "query", LabelKey: "field.search_query", Value: prefill},
		},
		Submit: "submit",
	}
	sc.Hint = "hint.search"
	s.sendScreen(sc)
}

func (s *session) showSearchResults(query string, page int) {
	s.screen = "search_results"
	s.searchQuery = query
	if page < 1 {
		page = 1
	}

	results, total, err := searchPosts(query, searchPageSize, (page-1)*searchPageSize)
	if err != nil {
		s.sendOutput("search error: " + err.Error() + "\r\n")
		s.showSearch(query)
		return
	}

	totalPage := (total + searchPageSize - 1) / searchPageSize
	if totalPage < 1 {
		totalPage = 1
	}
	if page > totalPage {
		page = totalPage
	}
	s.searchPage = page

	sc := s.baseScreen("search_results", "title.search_results")
	sc.Lines = []Line{
		{Key: "msg.search_query", Args: []string{query}, Kind: "info"},
		{Key: "msg.search_total", Args: []string{strconv.Itoa(total)}, Kind: "author"},
		{Key: "msg.page", Args: []string{strconv.Itoa(page), strconv.Itoa(totalPage)}, Kind: "author"},
		Line{},
	}

	key := 1
	var menu []MenuItem
	for _, r := range results {
		kindLabel := "T"
		if r.Kind == "post" {
			kindLabel = "R"
		}
		label := fmt.Sprintf("[%s] %-40s %s  %s",
			kindLabel, trunc(r.Title, 40), r.Author, fmtRelTime(r.Updated))
		menu = append(menu, MenuItem{
			Key:    strconv.Itoa(key),
			Label:  label,
			Action: "thread:" + r.ThreadID,
		})
		key++
	}
	if len(menu) == 0 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.no_results", Kind: "author"}, Line{})
	}
	menu = append(menu,
		MenuItem{Key: "s", LabelKey: "menu.search_again", Action: "search_again"},
		MenuItem{Key: "b", LabelKey: "menu.back_home", Action: "home"},
	)

	sc.Menu = menu
	sc.Hint = "hint.search_results"
	sc.Page = page
	sc.TotalPage = totalPage
	sc.CanPrev = page > 1
	sc.CanNext = page < totalPage
	s.sendScreen(sc)
}
