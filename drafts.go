package main

import (
	"encoding/json"
	"os"
	"time"
)

func (s *session) handleSaveDraft(valueJSON string) {
	var in struct {
		Screen   string            `json:"screen"`
		Fields   map[string]string `json:"fields"`
		DraftID  string            `json:"draft_id"`
		BoardID  string            `json:"board_id"`
		ThreadID string            `json:"thread_id"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &in); err != nil {
		s.sendError(0, "BAD_DRAFT", "invalid draft payload")
		return
	}
	if s.userID == "" {
		s.sendError(0, "AUTH", "login required")
		return
	}

	if in.BoardID == "" {
		in.BoardID = s.boardID
	}
	if in.ThreadID == "" {
		in.ThreadID = s.threadID
	}
	if in.BoardID == "" && in.ThreadID == "" {
		s.sendOutput("cannot save draft: no board context\r\n")
		return
	}

	body := in.Fields["body"]
	if len(body) > cfg.Drafts.MaxSize {
		s.sendOutput("draft too large\r\n")
		return
	}
	id := in.DraftID
	if id == "" {
		id = newID("d")
	}
	rel, err := writeDraftFile(s.userID, id, []byte(body))
	if err != nil {
		s.sendOutput("save draft failed\r\n")
		return
	}
	_, err = db.Exec(
		`INSERT INTO drafts(id, user_id, board_id, thread_id, title, file_path, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   board_id=excluded.board_id, thread_id=excluded.thread_id,
		   title=excluded.title, file_path=excluded.file_path, updated_at=excluded.updated_at`,
		id, s.userID, in.BoardID, in.ThreadID, in.Fields["title"], rel, time.Now().Unix(),
	)
	if err != nil {
		s.sendOutput("save draft failed\r\n")
		return
	}

	ok := true
	raw, _ := json.Marshal(map[string]string{"draft_id": id})
	s.sendRaw(Message{Type: "response", OK: &ok, Data: raw})
	s.sendOutput("draft saved: " + id + "\r\n")
}

func (s *session) removeDraft(id string) {
	var rel string
	_ = db.QueryRow(`SELECT file_path FROM drafts WHERE id = ? AND user_id = ?`, id, s.userID).Scan(&rel)
	_, _ = db.Exec(`DELETE FROM drafts WHERE id = ? AND user_id = ?`, id, s.userID)
	if rel != "" {
		_ = os.Remove(rel)
	}
}

func (s *session) removeDraftsForContext(boardID, threadID string) {
	var rels []string
	rows, err := db.Query(
		`SELECT file_path FROM drafts WHERE user_id = ? AND board_id = ? AND thread_id = ?`,
		s.userID, boardID, threadID,
	)
	if err != nil {
		return
	}
	for rows.Next() {
		var rel string
		_ = rows.Scan(&rel)
		if rel != "" {
			rels = append(rels, rel)
		}
	}
	rows.Close()

	_, _ = db.Exec(
		`DELETE FROM drafts WHERE user_id = ? AND board_id = ? AND thread_id = ?`,
		s.userID, boardID, threadID,
	)
	for _, rel := range rels {
		_ = os.Remove(rel)
	}
}

func (s *session) openDraft(id string) {
	var boardID, threadID, title, rel string
	err := db.QueryRow(
		`SELECT board_id, thread_id, title, file_path FROM drafts WHERE id = ? AND user_id = ?`,
		id, s.userID,
	).Scan(&boardID, &threadID, &title, &rel)
	if err != nil {
		s.showDraftList()
		return
	}
	body, _ := readFile(rel)
	s.draftID = id
	s.prevScreen = "draft_list"

	if threadID != "" {
		s.threadID = threadID
		if boardID == "" {
			_ = db.QueryRow(`SELECT board_id FROM threads WHERE id = ?`, threadID).Scan(&boardID)
		}
		if boardID == "" {
			s.removeDraft(id)
			s.draftID = ""
			s.threadID = ""
			s.showDraftList()
			s.sendOutput("draft discarded: thread no longer exists\r\n")
			return
		}
		s.boardID = boardID
		s.showReply("", string(body), id)
	} else {
		if boardID == "" {
			s.removeDraft(id)
			s.draftID = ""
			s.showDraftList()
			s.sendOutput("draft discarded: missing board context\r\n")
			return
		}
		s.boardID = boardID
		s.showNewThread("", title, string(body), id)
	}
}

func (s *session) deleteDraft(id string) {
	s.removeDraft(id)
	if s.draftID == id {
		s.draftID = ""
	}

	from := s.prevScreen
	s.prevScreen = ""
	current := s.screen

	switch current {
	case "new_thread", "reply":
		switch from {
		case "draft_list":
			s.showDraftList()
		case "thread_read":
			if s.threadID != "" {
				s.showThreadRead()
			} else {
				s.showHome("", "")
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
	default:
		s.showDraftList()
	}
}
