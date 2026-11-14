package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type connMsg struct{ msg Message }
type wsClosedMsg struct{}
type flashClearMsg struct{}

type Model struct {
	client *WSClient

	screen   *Screen
	selected int
	lang     string
	form     *formState

	threadList   list.Model
	readViewport viewport.Model

	cachedBoards []MenuItem

	width, height int

	lastEscAt time.Time

	quit       bool
	lastOutput string
	flashUntil time.Time
}

func NewModel(client *WSClient, lang string) Model {
	if lang == "" {
		lang = "en"
	}
	return Model{client: client, lang: lang}
}

func (m Model) Init() tea.Cmd {
	debugf("Init lang=%s", m.lang)
	return waitForMessage(m.client)
}

func waitForMessage(c *WSClient) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-c.Inbox():
			if !ok {
				return wsClosedMsg{}
			}
			return connMsg{msg}
		case <-c.Closed():
			return wsClosedMsg{}
		}
	}
}

func tickFlash() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return flashClearMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(flashClearMsg); ok {
		if time.Now().After(m.flashUntil) {
			m.lastOutput = ""
		}
		return m, nil
	}

	if ed, ok := msg.(editorDoneMsg); ok {
		if m.form != nil {
			cmd := m.form.UpdateMsg(ed)
			return m, cmd
		}
		return m, nil
	}

	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		if m.form != nil {
			m.form.SetSize(ws.Width, ws.Height)
		}
		if m.screen != nil {
			switch m.screen.Screen {
			case "thread_list":
				w := ws.Width - 8
				if w < 30 {
					w = 30
				}
				h := ws.Height - 10
				if h < 5 {
					h = 5
				}
				m.threadList.SetWidth(w)
				m.threadList.SetHeight(h)
			case "thread_read":
				w := ws.Width - 8
				if w < 30 {
					w = 30
				}
				h := ws.Height - 10
				if h < 5 {
					h = 5
				}
				m.readViewport.Width = w
				m.readViewport.Height = h
			}
		}
		return m, nil
	}

	switch v := msg.(type) {
	case connMsg:
		debugf("recv type=%s", v.msg.Type)
		switch v.msg.Type {
		case "screen":
			var sc Screen
			if err := json.Unmarshal(v.msg.Data, &sc); err == nil {
				if sc.Screen == "boards" {
					m.cachedBoards = sc.Menu
				}
				m.screen = &sc
				m.selected = 0

				if sc.Screen == "bye" {
					m.client.Close()
					m.quit = true
					return m, tea.Quit
				}

				if sc.Form != nil {
					m.form = newFormState(&sc, m.width, m.height)
					debugf("screen=%s form fields=%d draftID=%s", sc.Screen, len(m.form.fields), m.form.draftID)
				} else {
					m.form = nil
					debugf("screen=%s menu=%d", sc.Screen, len(sc.Menu))
				}

				switch sc.Screen {
				case "thread_list":
					m.threadList = buildThreadList(sc.Menu, m.width, m.height)
				case "thread_read":
					m.readViewport = buildReadViewport(sc.Lines, m.lang, m.width, m.height)
				}
			}
		case "response":
			// 服务端错误响应：3 秒 flash 提示，不再是静默
			if v.msg.OK != nil && !*v.msg.OK && v.msg.Error != nil {
				m.lastOutput = "[error " + v.msg.Error.Code + "] " + v.msg.Error.Message
				m.flashUntil = time.Now().Add(3 * time.Second)
				debugf("server error: %s %s", v.msg.Error.Code, v.msg.Error.Message)
			}
			if m.form != nil && len(v.msg.Data) > 0 {
				var data struct {
					DraftID string `json:"draft_id"`
				}
				if err := json.Unmarshal(v.msg.Data, &data); err == nil && data.DraftID != "" {
					m.form.draftID = data.DraftID
					debugf("form draft_id=%s", data.DraftID)
				}
			}
		case "output":
			m.lastOutput = strings.TrimRight(v.msg.Text, "\r\n")
			m.flashUntil = time.Now().Add(3 * time.Second)
			return m, tea.Batch(waitForMessage(m.client), tickFlash())
		}
		return m, waitForMessage(m.client)

	case wsClosedMsg:
		m.quit = true
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(v)
	}

	// 非按键消息转给 list / viewport（cursor blink 等）
	if m.screen != nil && m.form == nil {
		switch m.screen.Screen {
		case "thread_list":
			var cmd tea.Cmd
			m.threadList, cmd = m.threadList.Update(msg)
			return m, cmd
		case "thread_read":
			var cmd tea.Cmd
			m.readViewport, cmd = m.readViewport.Update(msg)
			return m, cmd
		}
	}
	if m.form != nil {
		cmd := m.form.UpdateMsg(msg)
		return m, cmd
	}
	return m, nil
}

func isJunkKey(k tea.KeyMsg) bool {
	if len(k.Runes) == 0 {
		return false
	}
	for _, r := range k.Runes {
		if r >= 0x20 && r != 0x7f {
			return false
		}
	}
	return true
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	debugf("key=%q type=%d form=%v screen=%v", k.String(), k.Type, m.form != nil, m.screenName())

	if k.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if isJunkKey(k) {
		return m, nil
	}

	if m.form != nil {
		return m.handleFormKey(k)
	}

	if m.screen == nil {
		if k.Type == tea.KeyRunes && string(k.Runes) == "q" {
			return m, tea.Quit
		}
		return m, nil
	}

	switch m.screen.Screen {
	case "thread_list":
		return m.handleThreadListKey(k)
	case "thread_read":
		return m.handleThreadReadKey(k)
	default:
		return m.handleMenuKey(k)
	}
}

func (m Model) screenName() string {
	if m.screen == nil {
		return ""
	}
	return m.screen.Screen
}

// ---------- 表单 ----------

func (m Model) handleFormKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	action, cmd := m.form.UpdateKey(k)
	switch action {
	case formCancel:
		m.form = nil
		m.send("back", "")
	case formSubmit:
		vals := m.form.collect()
		b, _ := json.Marshal(vals)
		m.send("submit", string(b))
	case formSaveDraft:
		vals := m.form.collect()
		payload := map[string]any{
			"screen":    m.screen.Screen,
			"fields":    vals,
			"draft_id":  m.form.draftID,
			"board_id":  "",
			"thread_id": "",
		}
		b, _ := json.Marshal(payload)
		m.send("save_draft", string(b))
	case formDeleteDraft:
		if m.form.draftID != "" {
			m.send("command", "draft_del:"+m.form.draftID)
		}
	}
	return m, cmd
}

// ---------- thread_list ----------
func (m Model) handleThreadListKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(k.Runes) > 0 {
		s := string(k.Runes)
		switch s {
		case "n", "N":
			m.send("command", "new_thread")
			return m, nil
		case "i", "I":
			m.send("command", "toggle_images")
			return m, nil
		case "p", "P":
			m.send("command", "profile")
			return m, nil
		case "H", "?":
			m.send("command", "help")
			return m, nil
		case "L":
			m.toggleLang()
			return m, nil
		case "b", "B":
			m.send("back", "")
			return m, nil
		case "q", "Q":
			m.client.Close()
			return m, tea.Quit
		case "[":
			m.send("page", "prev")
			return m, nil
		case "]":
			m.send("page", "next")
			return m, nil
		}
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			for _, it := range m.screen.Menu {
				if strings.EqualFold(it.Key, s) {
					m.send("select", it.Action)
					return m, nil
				}
			}
		}
	}

	switch k.Type {
	case tea.KeyEsc:
		if m.escDoublePress() {
			return m, nil
		}
		m.send("back", "")
		return m, nil
	case tea.KeyLeft:
		m.send("back", "")
		return m, nil
	case tea.KeyRight, tea.KeyEnter:
		it, ok := m.threadList.SelectedItem().(listItem)
		if ok {
			m.send("select", it.action)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.threadList, cmd = m.threadList.Update(k)
	m.selected = m.threadList.Index()
	return m, cmd
}

// ---------- thread_read ----------
func (m Model) handleThreadReadKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	sc := m.screen
	hasMenu := len(sc.Menu) > 0

	if len(k.Runes) > 0 {
		s := string(k.Runes)
		switch s {
		case "r", "R":
			m.send("command", "reply")
			return m, nil
		case "e", "E":
			m.send("command", "edit_thread")
			return m, nil
		case "i", "I":
			m.send("command", "toggle_images")
			return m, nil
		case "b", "B":
			m.send("back", "")
			return m, nil
		case "H", "?":
			m.send("command", "help")
			return m, nil
		case "L":
			m.toggleLang()
			return m, nil
		case "q", "Q":
			m.client.Close()
			return m, tea.Quit
		case "j":
			if hasMenu {
				m.selected = (m.selected + 1) % len(sc.Menu)
			}
			return m, nil
		case "k":
			if hasMenu {
				m.selected = (m.selected - 1 + len(sc.Menu)) % len(sc.Menu)
			}
			return m, nil
		}
	}

	switch k.Type {
	case tea.KeyEsc:
		if m.escDoublePress() {
			return m, nil
		}
		m.send("back", "")
		return m, nil
	case tea.KeyLeft:
		m.send("back", "")
		return m, nil
	case tea.KeyRight:
		if hasMenu {
			it := sc.Menu[m.selected]
			if strings.HasPrefix(it.Action, "lang:") {
				m.applyLang(it.Action[5:])
				return m, nil
			}
			m.send("select", it.Action)
		}
		return m, nil
	case tea.KeyUp:
		if hasMenu {
			m.selected = (m.selected - 1 + len(sc.Menu)) % len(sc.Menu)
		}
		return m, nil
	case tea.KeyDown:
		if hasMenu {
			m.selected = (m.selected + 1) % len(sc.Menu)
		}
		return m, nil
	case tea.KeyEnter:
		if hasMenu {
			it := sc.Menu[m.selected]
			if strings.HasPrefix(it.Action, "lang:") {
				m.applyLang(it.Action[5:])
				return m, nil
			}
			m.send("select", it.Action)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.readViewport, cmd = m.readViewport.Update(k)
	return m, cmd
}

// ---------- 其他菜单屏 ----------
func (m Model) handleMenuKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	sc := m.screen
	hasMenu := len(sc.Menu) > 0

	if k.Type == tea.KeyDelete {
		if sc.Screen == "draft_list" && hasMenu {
			it := sc.Menu[m.selected]
			if strings.HasPrefix(it.Action, "draft_open:") {
				id := strings.TrimPrefix(it.Action, "draft_open:")
				m.send("command", "draft_del:"+id)
			}
		}
		return m, nil
	}

	if len(k.Runes) > 0 {
		s := string(k.Runes)
		switch s {
		case "j":
			if hasMenu {
				m.selected = (m.selected + 1) % len(sc.Menu)
			}
			return m, nil
		case "k":
			if hasMenu {
				m.selected = (m.selected - 1 + len(sc.Menu)) % len(sc.Menu)
			}
			return m, nil
		case "n", "N":
			m.send("command", "new_thread")
			return m, nil
		case "r", "R":
			m.send("command", "reply")
			return m, nil
		case "e", "E":
			m.send("command", "edit_thread")
			return m, nil
		case "i", "I":
			m.send("command", "toggle_images")
			return m, nil
		case "p", "P":
			m.send("command", "profile")
			return m, nil
		case "H", "?":
			m.send("command", "help")
			return m, nil
		case "L":
			m.toggleLang()
			return m, nil
		case "b", "B":
			m.send("back", "")
			return m, nil
		case "q", "Q":
			m.client.Close()
			return m, tea.Quit
		case "[":
			m.send("page", "prev")
			return m, nil
		case "]":
			m.send("page", "next")
			return m, nil
		}
		for _, it := range sc.Menu {
			if it.Key != "" && strings.EqualFold(it.Key, s) {
				if strings.HasPrefix(it.Action, "lang:") {
					m.applyLang(it.Action[5:])
					return m, nil
				}
				m.send("select", it.Action)
				return m, nil
			}
		}
		return m, nil
	}

	switch k.Type {
	case tea.KeyUp:
		if hasMenu {
			m.selected = (m.selected - 1 + len(sc.Menu)) % len(sc.Menu)
		}
		return m, nil
	case tea.KeyDown:
		if hasMenu {
			m.selected = (m.selected + 1) % len(sc.Menu)
		}
		return m, nil
	case tea.KeyLeft:
		m.send("back", "")
		return m, nil
	case tea.KeyRight:
		if hasMenu {
			it := sc.Menu[m.selected]
			if strings.HasPrefix(it.Action, "lang:") {
				m.applyLang(it.Action[5:])
				return m, nil
			}
			m.send("select", it.Action)
		}
		return m, nil
	case tea.KeyEnter:
		if hasMenu {
			it := sc.Menu[m.selected]
			if strings.HasPrefix(it.Action, "lang:") {
				m.applyLang(it.Action[5:])
				return m, nil
			}
			m.send("select", it.Action)
		}
		return m, nil
	case tea.KeyEsc:
		if m.escDoublePress() {
			return m, nil
		}
		m.send("back", "")
		return m, nil
	}
	return m, nil
}

// ---------- helpers ----------

func (m *Model) escDoublePress() bool {
	now := time.Now()
	if now.Sub(m.lastEscAt) < 200*time.Millisecond {
		return true
	}
	m.lastEscAt = now
	return false
}

func (m *Model) toggleLang() {
	if m.lang == "zh" {
		m.applyLang("en")
	} else {
		m.applyLang("zh")
	}
}

func (m *Model) applyLang(code string) {
	if code != "zh" && code != "en" {
		return
	}
	if m.lang == code {
		return
	}
	m.lang = code
	prefs := loadPrefs()
	prefs.Lang = code
	savePrefs(prefs)
	debugf("lang -> %s (saved)", code)
}

func (m *Model) send(action, value string) {
	debugf("send %s=%s", action, value)
	if err := m.client.SendAction(action, value); err != nil {
		m.lastOutput = "send failed: " + err.Error()
		m.flashUntil = time.Now().Add(3 * time.Second)
		debugf("send failed: %v", err)
	}
}

// ---------- View ----------

func (m Model) View() string {
	if m.screen == nil {
		return "Connecting to RETRO BBS...\n\nPress q or Ctrl+C to quit.\n"
	}

	width := m.width
	height := m.height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	if m.form != nil {
		return m.viewForm(width, height)
	}

	header := renderHeader(tr(m.lang, m.screen.Title, m.screen.TitleDefault))
	statusBar := m.viewStatusBar()

	var body string
	switch m.screen.Screen {
	case "thread_list":
		body = m.viewThreadListBody(width, height)
	case "thread_read":
		body = m.viewThreadReadBody(width, height)
	default:
		body = m.viewDefaultBody(width, height)
	}

	out := header + "\n\n" + body + "\n" + statusBar
	return fitToHeight(out, height)
}

func (m Model) viewStatusBar() string {
	hints := [][2]string{
		{"↑↓", tr(m.lang, "ui.hint.move", "move")},
		{"Enter", tr(m.lang, "ui.hint.open", "open")},
		{"Esc", tr(m.lang, "ui.hint.back", "back")},
		{"Q", tr(m.lang, "ui.hint.quit", "quit")},
	}
	return renderStatusBar(hints)
}

func (m Model) viewDefaultBody(width, height int) string {
	var b strings.Builder
	for _, line := range m.screen.Lines {
		b.WriteString(renderLine(line, m.lang) + "\n")
	}
	if len(m.screen.Menu) > 0 {
		b.WriteString("\n")
		b.WriteString(renderList(m.screen.Menu, m.selected, 0, m.lang))
	}
	if m.lastOutput != "" && time.Now().Before(m.flashUntil) {
		b.WriteString("\n" + styleHint.Render(m.lastOutput))
	}

	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}
	return stylePane.Width(innerWidth).Render(b.String())
}

func (m Model) viewThreadListBody(width, height int) string {
	leftContent := m.renderBoardsPaneContent()
	rightContent := m.threadList.View()

	leftWidth := width / 4
	if leftWidth < 16 {
		leftWidth = 16
	}
	if leftWidth > 26 {
		leftWidth = 26
	}
	rightWidth := width - leftWidth - 2
	if rightWidth < 30 {
		rightWidth = 30
	}

	leftBox := stylePane.Width(leftWidth - 4).Render(leftContent)
	rightBox := stylePaneActive.Width(rightWidth - 4).Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", rightBox)
}
func (m Model) viewThreadReadBody(width, height int) string {
	var b strings.Builder
	b.WriteString(m.readViewport.View())

	if m.readViewport.YOffset > 0 || m.readViewport.YOffset < len(m.screen.Lines)-m.readViewport.Height {
		b.WriteString("\n")
		b.WriteString(styleHint.Render(
			itoa(m.readViewport.YOffset+1) + "/" + itoa(len(m.screen.Lines)) +
				"  " + tr(m.lang, "hint.thread_read_scroll",
				"PgUp/PgDn scroll · ↑↓ select · Enter open"),
		))
	}

	if len(m.screen.Menu) > 0 {
		b.WriteString("\n")
		b.WriteString(renderList(m.screen.Menu, m.selected, 0, m.lang))
	}
	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}
	return stylePane.Width(innerWidth).Render(b.String())
}

func (m Model) viewForm(width, height int) string {
	var b strings.Builder
	b.WriteString(styleAppTitle.Render("  RETRO BBS"))
	b.WriteString("  " + styleAppSubtitle.Render("· "+tr(m.lang, m.screen.Title, m.screen.TitleDefault)))
	b.WriteString("\n\n")

	for _, line := range m.screen.Lines {
		b.WriteString(renderLine(line, m.lang) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.form.View(m.lang))
	b.WriteString("\n")
	if m.lastOutput != "" && time.Now().Before(m.flashUntil) {
		b.WriteString(styleHint.Render(m.lastOutput))
		b.WriteString("\n")
	}
	hintKey := m.screen.Hint
	hintFallback := m.screen.HintDefault
	if hintKey == "" {
		hintKey = "hint.form"
		hintFallback = "Tab field · Enter submit/newline · Ctrl+G submit · Ctrl+S draft · Ctrl+D delete draft · Ctrl+E editor · Esc cancel"
	}
	b.WriteString(styleHint.Render(tr(m.lang, hintKey, hintFallback)))
	b.WriteString("\n")
	return fitToHeight(b.String(), height)
}

func (m Model) renderBoardsPaneContent() string {
	if len(m.cachedBoards) == 0 {
		return styleHint.Render("(no boards)")
	}
	current := m.currentBoardName()
	var b strings.Builder
	for _, it := range m.cachedBoards {
		if !strings.HasPrefix(it.Action, "board:") {
			continue
		}
		name := it.Label
		if idx := strings.Index(name, " -"); idx > 0 {
			name = name[:idx]
		}
		if strings.EqualFold(strings.TrimSpace(name), current) {
			b.WriteString(styleSelected.Render("▶ "+name) + "\n")
		} else {
			b.WriteString("  " + name + "\n")
		}
	}
	return b.String()
}

func (m Model) currentBoardName() string {
	if m.screen != nil {
		for _, line := range m.screen.Lines {
			if line.Kind == "h1" {
				return line.Raw
			}
		}
	}
	return ""
}
