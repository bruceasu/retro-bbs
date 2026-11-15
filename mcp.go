package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ---------- Server ----------

// newMCPServer 返回 5 个只读 + 3 个写工具。
// 写工具在调用时从 ctx 提取 Bearer token 做身份认证。
func newMCPServer() *server.MCPServer {
	s := server.NewMCPServer(
		"retro-bbs", "1.0.0",
		server.WithToolCapabilities(true),
	)

	// ---- 只读工具 ----
	readOnly := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	}

	s.AddTool(
		mcp.NewTool("list_boards",
			append([]mcp.ToolOption{
				mcp.WithDescription("List all public boards on this BBS. Returns id, name, description, and thread count."),
			}, readOnly...)...,
		),
		handleMCPListBoards,
	)

	s.AddTool(
		mcp.NewTool("search_posts",
			append([]mcp.ToolOption{
				mcp.WithDescription("Search threads and replies by keyword. Returns ranked results with highlighted snippets. Chinese/Japanese are indexed character-by-character."),
				mcp.WithString("query", mcp.Required(), mcp.Description("Search query.")),
				mcp.WithNumber("limit", mcp.Description("Max results (1-100, default 20).")),
			}, readOnly...)...,
		),
		handleMCPSearchPosts,
	)

	s.AddTool(
		mcp.NewTool("get_thread",
			append([]mcp.ToolOption{
				mcp.WithDescription("Get a thread with its body and replies. Body is raw Markdown."),
				mcp.WithString("thread_id", mcp.Required(), mcp.Description("Thread ID (t_...).")),
			}, readOnly...)...,
		),
		handleMCPGetThread,
	)

	s.AddTool(
		mcp.NewTool("get_post",
			append([]mcp.ToolOption{
				mcp.WithDescription("Get a single thread or reply by ID."),
				mcp.WithString("id", mcp.Required(), mcp.Description("Post ID: t_... (thread) or p_... (reply).")),
			}, readOnly...)...,
		),
		handleMCPGetPost,
	)

	s.AddTool(
		mcp.NewTool("get_user_profile",
			append([]mcp.ToolOption{
				mcp.WithDescription("Get a user's public profile (no private data)."),
				mcp.WithString("username", mcp.Required(), mcp.Description("Username.")),
			}, readOnly...)...,
		),
		handleMCPGetUserProfile,
	)

	// ---- 写工具（需要 Bearer token） ----
	writeOpts := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	}

	s.AddTool(
		mcp.NewTool("get_agent_info",
			append([]mcp.ToolOption{
				mcp.WithDescription("Get the current agent's identity. Requires Authorization: Bearer <agent_key>."),
			}, writeOpts...)...,
		),
		handleMCPGetAgentInfo,
	)

	s.AddTool(
		mcp.NewTool("list_pending_asks",
			append([]mcp.ToolOption{
				mcp.WithDescription("List threads where this agent was summoned with @username. Poll this periodically. Requires Authorization: Bearer <agent_key>."),
				mcp.WithNumber("limit", mcp.Description("Max results (default 10, max 50).")),
			}, writeOpts...)...,
		),
		handleMCPListPendingAsks,
	)

	s.AddTool(
		mcp.NewTool("reply_to_thread",
			append([]mcp.ToolOption{
				mcp.WithDescription("Reply to a thread. Requires Authorization: Bearer <agent_key>. If responding to a pending ask, pass ask_id to mark it answered."),
				mcp.WithString("thread_id", mcp.Required(), mcp.Description("Thread ID (t_...).")),
				mcp.WithString("body", mcp.Required(), mcp.Description("Reply body in Markdown.")),
				mcp.WithString("ask_id", mcp.Description("Optional ask_id from list_pending_asks.")),
			}, writeOpts...)...,
		),
		handleMCPReplyToThread,
	)

	return s
}

// ---------- 参数辅助 ----------

func mcpArgs(req mcp.CallToolRequest) map[string]any {
	if req.Params.Arguments == nil {
		return nil
	}
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	raw, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func mcpArgString(req mcp.CallToolRequest, key string) string {
	args := mcpArgs(req)
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func mcpArgInt(req mcp.CallToolRequest, key string, def int) int {
	args := mcpArgs(req)
	if args == nil {
		return def
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return def
}

func mcpResultJSON(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// ---------- Context token ----------

type agentKeyContextKey struct{}

func tokenFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(agentKeyContextKey{}).(string); ok {
		return v
	}
	return ""
}

// requireAgent 从 ctx 提取 token 并验证，返回 (agentID, username, err)。
func requireAgent(ctx context.Context) (string, string, error) {
	key := tokenFromContext(ctx)
	if key == "" {
		return "", "", fmt.Errorf("authentication required: set Authorization: Bearer <agent_key>")
	}
	agentID, username, ok := verifyAgentKey(key)
	if !ok {
		return "", "", fmt.Errorf("invalid agent key")
	}
	return agentID, username, nil
}

// ---------- 只读工具实现 ----------

func handleMCPListBoards(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rows, err := db.Query(`SELECT id, name, description FROM boards WHERE hidden = 0 ORDER BY id`)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer rows.Close()

	type boardRow struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		ThreadCount int    `json:"thread_count"`
	}
	boards := []boardRow{}
	for rows.Next() {
		var b boardRow
		_ = rows.Scan(&b.ID, &b.Name, &b.Description)
		_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE board_id = ? AND hidden = 0`, b.ID).Scan(&b.ThreadCount)
		boards = append(boards, b)
	}
	return mcpResultJSON(map[string]any{"boards": boards})
}

func handleMCPSearchPosts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := mcpArgString(req, "query")
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}
	limit := mcpArgInt(req, "limit", 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	words := strings.Fields(query)
	for i, w := range words {
		w = strings.ReplaceAll(w, `"`, "")
		w = strings.ReplaceAll(w, `'`, "")
		w = strings.ReplaceAll(w, `(`, "")
		w = strings.ReplaceAll(w, `)`, "")
		w = strings.ReplaceAll(w, `*`, "")
		words[i] = w
	}
	q := strings.Join(words, " ")
	if q == "" {
		return mcpResultJSON(map[string]any{"total": 0, "results": []any{}})
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM search_index WHERE search_index MATCH ?`, q).Scan(&total); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	rows, err := db.Query(`
		SELECT kind, ref_id, thread_id,
		       COALESCE(snippet(search_index, -1, '<b>', '</b>', '...', 64), '') AS snip,
		       updated_at
		FROM search_index
		WHERE search_index MATCH ?
		ORDER BY rank
		LIMIT ?`, q, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer rows.Close()

	type hit struct {
		Kind      string `json:"kind"`
		ThreadID  string `json:"thread_id"`
		PostID    string `json:"post_id,omitempty"`
		Title     string `json:"title"`
		Author    string `json:"author"`
		UpdatedAt int64  `json:"updated_at"`
		Snippet   string `json:"snippet"`
	}
	results := []hit{}
	for rows.Next() {
		var h hit
		var kind, refID, threadID, snip string
		var updated int64
		if err := rows.Scan(&kind, &refID, &threadID, &snip, &updated); err != nil {
			continue
		}
		h.Kind = kind
		h.ThreadID = threadID
		h.UpdatedAt = updated
		h.Snippet = snip
		if kind == "thread" {
			_ = db.QueryRow(`SELECT title FROM threads WHERE id = ?`, refID).Scan(&h.Title)
			_ = db.QueryRow(`SELECT u.username FROM threads t JOIN users u ON u.id = t.author_id WHERE t.id = ?`, refID).Scan(&h.Author)
		} else {
			h.PostID = refID
			var threadTitle string
			_ = db.QueryRow(`SELECT title FROM threads WHERE id = ?`, threadID).Scan(&threadTitle)
			if threadTitle != "" {
				h.Title = "re: " + threadTitle
			} else {
				h.Title = "re: (unknown thread)"
			}
			_ = db.QueryRow(`SELECT u.username FROM posts p JOIN users u ON u.id = p.author_id WHERE p.id = ?`, refID).Scan(&h.Author)
		}
		results = append(results, h)
	}

	return mcpResultJSON(map[string]any{"total": total, "results": results})
}

func handleMCPGetThread(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	threadID := mcpArgString(req, "thread_id")
	if threadID == "" {
		return mcp.NewToolResultError("thread_id is required"), nil
	}

	var title, boardID, authorID, filePath string
	var createdAt, updatedAt int64
	var hidden int
	err := db.QueryRow(
		`SELECT title, board_id, author_id, file_path, created_at, updated_at, hidden
		 FROM threads WHERE id = ?`, threadID,
	).Scan(&title, &boardID, &authorID, &filePath, &createdAt, &updatedAt, &hidden)
	if err != nil || hidden != 0 {
		return mcp.NewToolResultError("thread not found"), nil
	}

	var boardName string
	var boardHidden int
	_ = db.QueryRow(`SELECT name, hidden FROM boards WHERE id = ?`, boardID).Scan(&boardName, &boardHidden)
	if boardName == "" || boardHidden != 0 {
		return mcp.NewToolResultError("thread not found"), nil
	}

	var author string
	_ = db.QueryRow(`SELECT username FROM users WHERE id = ?`, authorID).Scan(&author)

	body, _ := os.ReadFile(filePath)

	type replyJSON struct {
		ID        string `json:"id"`
		Author    string `json:"author"`
		CreatedAt int64  `json:"created_at"`
		Body      string `json:"body"`
	}
	replies := []replyJSON{}
	rows, err := db.Query(
		`SELECT p.id, u.username, p.file_path, p.created_at
		 FROM posts p JOIN users u ON u.id = p.author_id
		 WHERE p.thread_id = ? AND p.hidden = 0
		 ORDER BY p.created_at ASC LIMIT 200`, threadID)
	if err == nil {
		for rows.Next() {
			var pid, uname, pfile string
			var pcreated int64
			_ = rows.Scan(&pid, &uname, &pfile, &pcreated)
			pbody, _ := os.ReadFile(pfile)
			replies = append(replies, replyJSON{
				ID:        pid,
				Author:    uname,
				CreatedAt: pcreated,
				Body:      string(pbody),
			})
		}
		rows.Close()
	}

	var actualCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts WHERE thread_id = ? AND hidden = 0`, threadID).Scan(&actualCount)

	return mcpResultJSON(map[string]any{
		"id":         threadID,
		"board_id":   boardID,
		"board_name": boardName,
		"title":      title,
		"author":     author,
		"created_at": createdAt,
		"updated_at": updatedAt,
		"body":       string(body),
		"replies":    replies,
		"truncated":  actualCount > len(replies),
	})
}

func handleMCPGetPost(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := mcpArgString(req, "id")
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	if strings.HasPrefix(id, "t_") {
		var title, authorID, filePath, boardID string
		var createdAt int64
		var hidden int
		err := db.QueryRow(
			`SELECT title, author_id, file_path, board_id, created_at, hidden
			 FROM threads WHERE id = ?`, id,
		).Scan(&title, &authorID, &filePath, &boardID, &createdAt, &hidden)
		if err != nil || hidden != 0 {
			return mcp.NewToolResultError("post not found"), nil
		}
		var boardHidden int
		_ = db.QueryRow(`SELECT hidden FROM boards WHERE id = ?`, boardID).Scan(&boardHidden)
		if boardHidden != 0 {
			return mcp.NewToolResultError("post not found"), nil
		}
		var author string
		_ = db.QueryRow(`SELECT username FROM users WHERE id = ?`, authorID).Scan(&author)
		body, _ := os.ReadFile(filePath)
		return mcpResultJSON(map[string]any{
			"id": id, "kind": "thread", "thread_id": id,
			"title": title, "author": author, "created_at": createdAt, "body": string(body),
		})
	}

	if strings.HasPrefix(id, "p_") {
		var threadID, authorID, filePath string
		var createdAt int64
		var hidden int
		err := db.QueryRow(
			`SELECT thread_id, author_id, file_path, created_at, hidden
			 FROM posts WHERE id = ?`, id,
		).Scan(&threadID, &authorID, &filePath, &createdAt, &hidden)
		if err != nil || hidden != 0 {
			return mcp.NewToolResultError("post not found"), nil
		}
		var threadHidden int
		var threadTitle, boardID string
		_ = db.QueryRow(`SELECT hidden, title, board_id FROM threads WHERE id = ?`, threadID).
			Scan(&threadHidden, &threadTitle, &boardID)
		if threadHidden != 0 {
			return mcp.NewToolResultError("post not found"), nil
		}
		var boardHidden int
		_ = db.QueryRow(`SELECT hidden FROM boards WHERE id = ?`, boardID).Scan(&boardHidden)
		if boardHidden != 0 {
			return mcp.NewToolResultError("post not found"), nil
		}
		var author string
		_ = db.QueryRow(`SELECT username FROM users WHERE id = ?`, authorID).Scan(&author)
		body, _ := os.ReadFile(filePath)
		return mcpResultJSON(map[string]any{
			"id": id, "kind": "post", "thread_id": threadID,
			"title": threadTitle, "author": author, "created_at": createdAt, "body": string(body),
		})
	}

	return mcp.NewToolResultError("id must start with t_ or p_"), nil
}

func handleMCPGetUserProfile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	username := mcpArgString(req, "username")
	if username == "" {
		return mcp.NewToolResultError("username is required"), nil
	}

	var id, status string
	var joinedAt int64
	err := db.QueryRow(
		`SELECT id, created_at, status FROM users WHERE username = ?`, username,
	).Scan(&id, &joinedAt, &status)
	if err != nil || status != "active" {
		return mcp.NewToolResultError("user not found"), nil
	}

	var nickname, bio string
	_ = db.QueryRow(`SELECT nickname, bio FROM profiles WHERE user_id = ?`, id).Scan(&nickname, &bio)
	if nickname == "" {
		nickname = username
	}

	var threadCount, replyCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM threads WHERE author_id = ? AND hidden = 0`, id).Scan(&threadCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts WHERE author_id = ? AND hidden = 0`, id).Scan(&replyCount)

	return mcpResultJSON(map[string]any{
		"username": username, "nickname": nickname, "bio": bio, "joined_at": joinedAt,
		"thread_count": threadCount, "reply_count": replyCount,
	})
}

// ---------- 写工具实现 ----------

func handleMCPGetAgentInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, username, err := requireAgent(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	var displayName string
	_ = db.QueryRow(`SELECT agent_display_name FROM users WHERE id = ?`, agentID).Scan(&displayName)
	return mcpResultJSON(map[string]any{
		"agent_id":     agentID,
		"username":     username,
		"display_name": displayName,
		"limits": map[string]any{
			"max_replies_per_thread":  cfg.Agent.MaxRepliesPerAgentPerThread,
			"max_total_agent_replies": cfg.Agent.MaxTotalAgentRepliesPerThread,
		},
	})
}

func handleMCPListPendingAsks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, _, err := requireAgent(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	limit := mcpArgInt(req, "limit", 10)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	rows, err := db.Query(
		`SELECT id, thread_id, post_id, question, caller_kind, created_at
		 FROM agent_asks WHERE agent_id = ? AND status = 'pending'
		 ORDER BY created_at ASC LIMIT ?`,
		agentID, limit,
	)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer rows.Close()

	type askRow struct {
		AskID      string `json:"ask_id"`
		ThreadID   string `json:"thread_id"`
		PostID     string `json:"post_id"`
		Question   string `json:"question"`
		CallerKind string `json:"caller_kind"`
		CreatedAt  int64  `json:"created_at"`
	}
	asks := []askRow{}
	for rows.Next() {
		var a askRow
		_ = rows.Scan(&a.AskID, &a.ThreadID, &a.PostID, &a.Question, &a.CallerKind, &a.CreatedAt)
		asks = append(asks, a)
	}
	return mcpResultJSON(map[string]any{"asks": asks})
}

func handleMCPReplyToThread(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, agentUsername, err := requireAgent(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	threadID := mcpArgString(req, "thread_id")
	body := mcpArgString(req, "body")
	askID := mcpArgString(req, "ask_id")
	if threadID == "" || body == "" {
		return mcp.NewToolResultError("thread_id and body are required"), nil
	}

	postID, err := createAgentReply(agentID, threadID, body, askID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcpResultJSON(map[string]any{
		"post_id":   postID,
		"thread_id": threadID,
		"agent":     agentUsername,
		"url":       "/thread/" + threadID + "#" + postID,
	})
}

// ---------- HTTP handler ----------

// newMCPHTTPHandler 返回 /mcp 端点。
// 只读工具匿名可用；写工具需要 Authorization: Bearer <agent_key>。
func newMCPHTTPHandler() http.Handler {
	s := newMCPServer()
	streamServer := server.NewStreamableHTTPServer(s)
	return injectTokenMiddleware(streamServer)
}

// injectTokenMiddleware 把 Authorization Bearer token 注入 context。
// 不做校验——写工具内部自己校验。
func injectTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		var key string
		if strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
		ctx := context.WithValue(r.Context(), agentKeyContextKey{}, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// 保留 net import 供未来扩展（比如加 IP 白名单）
var _ = net.ParseIP
