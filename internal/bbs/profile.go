package bbs

import (
	"strings"
	"time"
)

func (s *session) loadProfile() (nickname, bio string) {
	_ = db.QueryRow(`SELECT nickname, bio FROM profiles WHERE user_id = ?`, s.userID).
		Scan(&nickname, &bio)
	return
}

func (s *session) showProfile() {
	s.screen = "profile"
	if s.userID == "" {
		s.showLogin("error.login_required")
		return
	}
	nickname, bio := s.loadProfile()
	if nickname == "" {
		nickname = s.username
	}

	var created int64
	_ = db.QueryRow(`SELECT created_at FROM users WHERE id = ?`, s.userID).Scan(&created)
	var threads, replies int
	_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE author_id = ?`, s.userID).Scan(&threads)
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts WHERE author_id = ?`, s.userID).Scan(&replies)

	sc := s.baseScreen("profile", "title.profile")
	sc.Lines = []Line{
		{Key: "msg.profile_username", Args: []string{s.username}, Kind: "info"},
		{Key: "msg.profile_nickname", Args: []string{nickname}, Kind: "info"},
		{Key: "msg.profile_joined", Args: []string{fmtTime(created)}, Kind: "author"},
		{Key: "msg.profile_posts", Args: []string{itoa(threads), itoa(replies)}, Kind: "author"},
		Line{},
	}
	if bio != "" {
		sc.Lines = append(sc.Lines, Line{Key: "msg.profile_bio"}, Line{})
		for _, l := range strings.Split(bio, "\n") {
			sc.Lines = append(sc.Lines, Line{Raw: l})
		}
	} else {
		sc.Lines = append(sc.Lines, Line{Key: "msg.profile_no_bio", Kind: "author"})
	}
	sc.Menu = []MenuItem{
		{Key: "e", LabelKey: "menu.edit_profile", Action: "edit_profile"},
		{Key: "b", LabelKey: "menu.back_home", Action: "home"},
	}
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func (s *session) showProfileEdit(errKey string) {
	s.screen = "profile_edit"
	if s.userID == "" {
		s.showLogin("error.login_required")
		return
	}
	nickname, bio := s.loadProfile()
	if nickname == "" {
		nickname = s.username
	}
	sc := s.baseScreen("profile_edit", "title.edit_profile")
	sc.Lines = []Line{{Key: "msg.profile_edit_prompt"}, Line{}}
	if errKey != "" {
		sc.Lines = append(sc.Lines, Line{Key: errKey, Kind: "err"}, Line{})
	}
	sc.Form = &Form{
		Fields: []FormField{
			{Name: "nickname", LabelKey: "field.nickname", Value: nickname},
			{Name: "bio", LabelKey: "field.bio", Multiline: true, Value: bio},
		},
		Submit: "submit",
	}
	sc.Hint = "hint.editor"
	s.sendScreen(sc)
}

func (s *session) submitProfile(nickname, bio string) {
	nickname = strings.TrimSpace(nickname)
	if len(nickname) == 0 {
		s.showProfileEdit("error.nickname_required")
		return
	}
	if len(nickname) > 64 {
		s.showProfileEdit("error.nickname_too_long")
		return
	}
	if len(bio) > 2048 {
		s.showProfileEdit("error.bio_too_long")
		return
	}
	_, err := db.Exec(
		`INSERT INTO profiles(user_id, nickname, bio) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET nickname=excluded.nickname, bio=excluded.bio`,
		s.userID, nickname, bio,
	)
	if err != nil {
		s.showProfileEdit("error.save_failed")
		return
	}
	s.nickname = nickname
	s.showProfile()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var _ = time.Now // keep time import if unused elsewhere
