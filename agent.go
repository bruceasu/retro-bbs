package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// ---------- API Key ----------

func generateAPIKey() (string, string, error) {
	b1 := make([]byte, 6)
	if _, err := rand.Read(b1); err != nil {
		return "", "", err
	}
	b2 := make([]byte, 16)
	if _, err := rand.Read(b2); err != nil {
		return "", "", err
	}
	plain := "agent_" + hex.EncodeToString(b1) + "_" + hex.EncodeToString(b2)
	hash, err := hashPassword(plain)
	if err != nil {
		return "", "", err
	}
	return plain, hash, nil
}

// verifyAgentKey 遍历所有 is_agent=1 用户，用 Argon2id 比较。
// Agent 数量少（<50），性能可接受。
// TODO: 若 Agent 数量增长，加 api_key_prefix 字段做前缀索引。
func verifyAgentKey(plain string) (agentID, username string, ok bool) {
	if plain == "" {
		return "", "", false
	}
	rows, err := db.Query(`SELECT id, username, api_key_hash FROM users WHERE is_agent = 1 AND status = 'active' AND api_key_hash != ''`)
	if err != nil {
		return "", "", false
	}
	defer rows.Close()
	for rows.Next() {
		var id, uname, hash string
		_ = rows.Scan(&id, &uname, &hash)
		if verifyPassword(plain, hash) {
			return id, uname, true
		}
	}
	return "", "", false
}

// ---------- 核心：Agent 创建回复 ----------

// createAgentReply 是 Agent 写操作的唯一业务入口。
// MCP 写工具调用它，不再经过 HTTP 中转。
//
// 校验顺序：
//  1. cfg.Agent.Enabled
//  2. agent 存在且 is_agent=1 且 status=active
//  3. thread 存在且 hidden=0，board 存在且 hidden=0
//  4. askID（若给出）属于该 agent、该 thread、且状态 pending
//  5. 每 agent 每 thread 回复数上限
//  6. 每 thread 所有 agent 总回复数上限
//  7. 落库、索引、标记 ask、审计、递归扫描 @mention
func createAgentReply(agentID, threadID, body, askID string) (postID string, err error) {
	if !cfg.Agent.Enabled {
		return "", fmt.Errorf("agent mode disabled")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("body is required")
	}
	if len(body) > cfg.Posts.MaxSize {
		return "", fmt.Errorf("body too large (max %d bytes)", cfg.Posts.MaxSize)
	}

	var agentUsername string
	var isAgent int
	if err := db.QueryRow(
		`SELECT username, is_agent FROM users WHERE id = ? AND status = 'active'`,
		agentID,
	).Scan(&agentUsername, &isAgent); err != nil || isAgent != 1 {
		return "", fmt.Errorf("invalid agent")
	}

	var threadHidden int
	var boardID string
	if err := db.QueryRow(
		`SELECT hidden, board_id FROM threads WHERE id = ?`, threadID,
	).Scan(&threadHidden, &boardID); err != nil {
		return "", fmt.Errorf("thread not found")
	}
	if threadHidden != 0 {
		return "", fmt.Errorf("thread not found")
	}
	var boardHidden int
	_ = db.QueryRow(`SELECT hidden FROM boards WHERE id = ?`, boardID).Scan(&boardHidden)
	if boardHidden != 0 {
		return "", fmt.Errorf("thread not found")
	}

	if askID != "" {
		var askAgentID, askStatus, askThreadID string
		if err := db.QueryRow(
			`SELECT agent_id, status, thread_id FROM agent_asks WHERE id = ?`, askID,
		).Scan(&askAgentID, &askStatus, &askThreadID); err != nil {
			return "", fmt.Errorf("ask not found")
		}
		if askAgentID != agentID || askThreadID != threadID || askStatus != "pending" {
			return "", fmt.Errorf("invalid ask")
		}
	}

	var agentThreadReplies int
	_ = db.QueryRow(
		`SELECT COUNT(*) FROM posts WHERE thread_id = ? AND author_id = ? AND hidden = 0`,
		threadID, agentID,
	).Scan(&agentThreadReplies)
	if agentThreadReplies >= cfg.Agent.MaxRepliesPerAgentPerThread {
		return "", fmt.Errorf("agent reply limit reached for this thread (%d)", cfg.Agent.MaxRepliesPerAgentPerThread)
	}

	var totalAgentReplies int
	_ = db.QueryRow(
		`SELECT COUNT(*) FROM posts p JOIN users u ON u.id = p.author_id
		 WHERE p.thread_id = ? AND u.is_agent = 1 AND p.hidden = 0`,
		threadID,
	).Scan(&totalAgentReplies)
	if totalAgentReplies >= cfg.Agent.MaxTotalAgentRepliesPerThread {
		return "", fmt.Errorf("thread agent reply limit reached (%d)", cfg.Agent.MaxTotalAgentRepliesPerThread)
	}

	postID = newID("p")
	rel, err := writeReplyFile(threadID, postID, []byte(body))
	if err != nil {
		log.Printf("agent reply write file: %v", err)
		return "", fmt.Errorf("save failed")
	}
	now := time.Now().Unix()
	_, err = db.Exec(
		`INSERT INTO posts(id, thread_id, author_id, file_path, created_at, updated_at, hidden)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`,
		postID, threadID, agentID, rel, now, now,
	)
	if err != nil {
		_ = os.Remove(rel)
		return "", fmt.Errorf("save failed")
	}
	_, _ = db.Exec(`UPDATE threads SET reply_count = reply_count + 1, updated_at = ? WHERE id = ?`, now, threadID)
	if err := indexPost(postID); err != nil {
		log.Printf("index agent post %s: %v", postID, err)
	}

	if askID != "" {
		_, _ = db.Exec(
			`UPDATE agent_asks SET status = 'answered', answered_at = ? WHERE id = ?`,
			now, askID,
		)
	}

	actionID := newID("act")
	_, _ = db.Exec(
		`INSERT INTO agent_actions(id, agent_id, action, target_id, ask_id, created_at)
		 VALUES (?, ?, 'reply', ?, ?, ?)`,
		actionID, agentID, postID, askID, now,
	)

	// Agent 回复里的 @ 也触发 ask（受同样的限制）
	scanMentions(body, threadID, postID, agentID, "agent")

	return postID, nil
}

// ---------- 管理员 UI：Agent 管理 ----------

func (s *session) showAdminAgents() {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.screen = "admin_agents"
	rows, err := db.Query(
		`SELECT username, agent_display_name, agent_owner_id FROM users WHERE is_agent = 1 ORDER BY username`)
	if err != nil {
		s.sendError(0, "DB", err.Error())
		return
	}
	defer rows.Close()

	sc := s.baseScreen("admin_agents", "title.admin_agents")
	sc.Lines = []Line{{Key: "msg.admin_agents_prompt"}, Line{}}

	key := 1
	var menu []MenuItem
	for rows.Next() {
		var uname, display, ownerID string
		_ = rows.Scan(&uname, &display, &ownerID)
		var ownerName string
		_ = db.QueryRow(`SELECT username FROM users WHERE id = ?`, ownerID).Scan(&ownerName)
		label := fmt.Sprintf("%-20s %-20s owner=%s", uname, display, ownerName)
		menu = append(menu, MenuItem{
			Key:    itoa(key),
			Label:  label,
			Action: "agent_view:" + uname,
		})
		key++
	}
	if len(menu) == 0 {
		sc.Lines = append(sc.Lines, Line{Key: "msg.no_agents", Kind: "author"}, Line{})
	}
	menu = append(menu,
		MenuItem{Key: "n", LabelKey: "menu.agent_new", Action: "agent_new"},
		MenuItem{Key: "q", LabelKey: "menu.back_home", Action: "admin"},
	)
	sc.Menu = menu
	sc.Hint = "hint.admin_agents"
	s.sendScreen(sc)
}

func (s *session) showAgentNew(errKey string) {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.screen = "agent_new"
	sc := s.baseScreen("agent_new", "title.agent_new")
	sc.Lines = []Line{{Key: "msg.agent_new_prompt"}, Line{}}
	if errKey != "" {
		sc.Lines = append(sc.Lines, Line{Key: errKey, Kind: "err"}, Line{})
	}
	sc.Form = &Form{Fields: []FormField{
		{Name: "username", LabelKey: "field.agent_username"},
		{Name: "display_name", LabelKey: "field.agent_display_name"},
	}, Submit: "submit"}
	sc.Hint = "hint.editor"
	s.sendScreen(sc)
}

func (s *session) submitAgentNew(username, displayName string) {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	username = strings.ToLower(strings.TrimSpace(username))
	displayName = strings.TrimSpace(displayName)

	if !isValidAgentUsername(username) {
		s.showAgentNew("error.agent_username_invalid")
		return
	}
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 60 {
		s.showAgentNew("error.agent_display_too_long")
		return
	}
	var exists int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&exists)
	if exists > 0 {
		s.showAgentNew("error.username_taken")
		return
	}

	plain, hash, err := generateAPIKey()
	if err != nil {
		s.showAgentNew("error.save_failed")
		return
	}

	id := newID("u")
	_, err = db.Exec(
		`INSERT INTO users(id, username, password_hash, created_at, status, is_agent, agent_owner_id, api_key_hash, agent_display_name)
		 VALUES (?, ?, '', ?, 'active', 1, ?, ?, ?)`,
		id, username, time.Now().Unix(), s.userID, hash, displayName,
	)
	if err != nil {
		s.showAgentNew("error.save_failed")
		return
	}

	s.screen = "agent_created"
	sc := s.baseScreen("agent_created", "title.agent_created")
	sc.Lines = []Line{
		{Key: "msg.agent_created", Args: []string{username}, Kind: "ok"},
		Line{},
		{Key: "msg.agent_key_warning", Kind: "err"},
		Line{},
		{Raw: "  " + plain, Kind: "code"},
		Line{},
		{Key: "msg.agent_key_hint"},
		Line{},
	}
	sc.Menu = []MenuItem{
		{Key: "q", LabelKey: "menu.back_home", Action: "admin_agents"},
	}
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func isValidAgentUsername(u string) bool {
	if len(u) < 5 || len(u) > 32 {
		return false
	}
	if !strings.HasPrefix(u, "bot_") {
		return false
	}
	for _, r := range u {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func (s *session) showAgentView(username string) {
	if !s.isAdmin {
		s.showHome("", "")
		return
	}
	s.screen = "agent_view"
	var id, display, ownerID string
	var createdAt int64
	err := db.QueryRow(
		`SELECT id, agent_display_name, agent_owner_id, created_at FROM users WHERE username = ? AND is_agent = 1`,
		username,
	).Scan(&id, &display, &ownerID, &createdAt)
	if err != nil {
		s.showAdminAgents()
		return
	}
	var ownerName string
	_ = db.QueryRow(`SELECT username FROM users WHERE id = ?`, ownerID).Scan(&ownerName)

	var threadCount, replyCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE author_id = ?`, id).Scan(&threadCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts WHERE author_id = ?`, id).Scan(&replyCount)

	sc := s.baseScreen("agent_view", "title.agent_view")
	sc.Lines = []Line{
		{Key: "msg.agent_username", Args: []string{username}, Kind: "info"},
		{Key: "msg.agent_display", Args: []string{display}, Kind: "info"},
		{Key: "msg.agent_owner", Args: []string{ownerName}, Kind: "author"},
		{Key: "msg.agent_created_at", Args: []string{fmtTime(createdAt)}, Kind: "author"},
		{Key: "msg.agent_posts", Args: []string{itoa(threadCount), itoa(replyCount)}, Kind: "author"},
		Line{},
	}
	sc.Menu = []MenuItem{
		{Key: "r", LabelKey: "menu.agent_rotate_key", Action: "agent_rotate:" + username},
		{Key: "d", LabelKey: "menu.agent_delete", Action: "agent_delete:" + username},
		{Key: "q", LabelKey: "menu.back_home", Action: "admin_agents"},
	}
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func (s *session) rotateAgentKey(username string) {
	if !s.isAdmin {
		return
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = ? AND is_agent = 1`, username).Scan(&id); err != nil {
		s.showAdminAgents()
		return
	}
	plain, hash, err := generateAPIKey()
	if err != nil {
		s.showAdminAgents()
		return
	}
	_, _ = db.Exec(`UPDATE users SET api_key_hash = ? WHERE id = ?`, hash, id)

	s.screen = "agent_created"
	sc := s.baseScreen("agent_created", "title.agent_created")
	sc.Lines = []Line{
		{Key: "msg.agent_key_rotated", Args: []string{username}, Kind: "ok"},
		Line{},
		{Key: "msg.agent_key_warning", Kind: "err"},
		Line{},
		{Raw: "  " + plain, Kind: "code"},
		Line{},
		{Key: "msg.agent_key_hint"},
		Line{},
	}
	sc.Menu = []MenuItem{
		{Key: "q", LabelKey: "menu.back_home", Action: "admin_agents"},
	}
	sc.Hint = "hint.select"
	s.sendScreen(sc)
}

func (s *session) deleteAgent(username string) {
	if !s.isAdmin {
		return
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = ? AND is_agent = 1`, username).Scan(&id); err != nil {
		s.showAdminAgents()
		return
	}
	// 软删除：suspended + 清空 key，历史帖子保留
	_, _ = db.Exec(`UPDATE users SET status = 'suspended', api_key_hash = '' WHERE id = ?`, id)
	s.showAdminAgents()
}

// ---------- @mention 扫描 ----------

var mentionRE = regexp.MustCompile(`@([a-z][a-z0-9_]{2,31})`)

// scanMentions 扫描 body 中的 @agent，创建 pending ask。
// 只处理：is_agent=1、active、非调用者自己、未重复 ask。
func scanMentions(body, threadID, postID, callerID, callerKind string) int {
	if !cfg.Agent.Enabled {
		return 0
	}

	stripped := stripCodeAndQuotes(body)
	matches := mentionRE.FindAllStringSubmatch(stripped, -1)
	if len(matches) == 0 {
		return 0
	}

	created := 0
	seen := make(map[string]bool)
	for _, m := range matches {
		username := m[1]
		if seen[username] {
			continue
		}
		seen[username] = true

		var agentID string
		err := db.QueryRow(
			`SELECT id FROM users WHERE username = ? AND is_agent = 1 AND status = 'active'`,
			username,
		).Scan(&agentID)
		if err != nil || agentID == callerID {
			continue
		}

		if cfg.Agent.RequireHumanAnchor {
			var humanCount int
			_ = db.QueryRow(
				`SELECT
				   (SELECT COUNT(*) FROM threads t JOIN users u ON u.id = t.author_id
				    WHERE t.id = ? AND u.is_agent = 0)
				 + (SELECT COUNT(*) FROM posts p JOIN users u ON u.id = p.author_id
				    WHERE p.thread_id = ? AND u.is_agent = 0 AND p.hidden = 0)`,
				threadID, threadID,
			).Scan(&humanCount)
			if humanCount == 0 {
				continue
			}
		}

		var agentCount int
		_ = db.QueryRow(
			`SELECT COUNT(DISTINCT p.author_id) FROM posts p JOIN users u ON u.id = p.author_id
			 WHERE p.thread_id = ? AND u.is_agent = 1 AND p.hidden = 0`,
			threadID,
		).Scan(&agentCount)
		var existingInThread int
		_ = db.QueryRow(
			`SELECT COUNT(*) FROM posts WHERE thread_id = ? AND author_id = ? AND hidden = 0`,
			threadID, agentID,
		).Scan(&existingInThread)

		if agentCount >= cfg.Agent.MaxAgentsPerThread && existingInThread == 0 {
			continue
		}
		if existingInThread >= cfg.Agent.MaxRepliesPerAgentPerThread {
			continue
		}

		var totalAgentReplies int
		_ = db.QueryRow(
			`SELECT COUNT(*) FROM posts p JOIN users u ON u.id = p.author_id
			 WHERE p.thread_id = ? AND u.is_agent = 1 AND p.hidden = 0`,
			threadID,
		).Scan(&totalAgentReplies)
		if totalAgentReplies >= cfg.Agent.MaxTotalAgentRepliesPerThread {
			continue
		}

		var dup int
		_ = db.QueryRow(
			`SELECT COUNT(*) FROM agent_asks WHERE thread_id = ? AND agent_id = ? AND status = 'pending'`,
			threadID, agentID,
		).Scan(&dup)
		if dup > 0 {
			continue
		}

		askID := newID("ask")
		_, err = db.Exec(
			`INSERT INTO agent_asks(id, thread_id, post_id, agent_id, caller_id, caller_kind, question, status, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
			askID, threadID, postID, agentID, callerID, callerKind, body, time.Now().Unix(),
		)
		if err != nil {
			log.Printf("agent ask insert: %v", err)
			continue
		}
		created++
	}
	return created
}

// stripCodeAndQuotes 去掉代码块、行内代码、引用块，避免误判 @。
func stripCodeAndQuotes(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	inCode := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if strings.HasPrefix(trim, "> ") {
			continue
		}
		for {
			start := strings.Index(line, "`")
			if start < 0 {
				break
			}
			end := strings.Index(line[start+1:], "`")
			if end < 0 {
				break
			}
			line = line[:start] + line[start+1+end+1:]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// 抑制 "argon2" 未使用警告（hashPassword 在 auth.go，此处仅 import 以保持一致）
var _ = argon2.IDKey
