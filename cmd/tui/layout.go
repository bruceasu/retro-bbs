package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ---------- list item ----------

type listItem struct {
	key    string
	label  string
	action string
}

func (i listItem) Title() string {
	if i.key != "" {
		return "[" + strings.ToUpper(i.key) + "] " + i.label
	}
	return i.label
}
func (i listItem) Description() string { return "" }
func (i listItem) FilterValue() string { return i.label }

// buildThreadList 把服务端下发的 Menu 转成 bubbles/list。
func buildThreadList(menu []MenuItem, width, height int) list.Model {
	items := make([]list.Item, 0, len(menu))
	for _, it := range menu {
		items = append(items, listItem{
			key:    it.Key,
			label:  menuItemLabel(it),
			action: it.Action,
		})
	}
	w := width - 8
	if w < 30 {
		w = 30
	}
	h := height - 10
	if h < 5 {
		h = 5
	}
	l := list.New(items, list.NewDefaultDelegate(), w, h)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetFilteringEnabled(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	// 去掉 vim 风格，只保留方向键和 PgUp/PgDn/Home/End
	// l.KeyMap.CursorUp.SetKeys("up")
	// l.KeyMap.CursorDown.SetKeys("down")
	// l.KeyMap.NextPage.SetKeys("right", "pgdown")
	// l.KeyMap.PrevPage.SetKeys("left", "pgup")
	// l.KeyMap.GoToStart.SetKeys("home")
	// l.KeyMap.GoToEnd.SetKeys("end")

	return l
}

// menuItemLabel 服务端 Label 已经含动态数据（作者/时间），优先用它。
func menuItemLabel(it MenuItem) string {
	if it.Label != "" {
		return it.Label
	}
	return it.Default
}

// buildReadViewport 把 thread_read 的 Lines 渲染到 viewport。
func buildReadViewport(lines []Line, lang string, width, height int) viewport.Model {
	w := width - 8
	if w < 30 {
		w = 30
	}
	h := height - 10
	if h < 5 {
		h = 5
	}
	vp := viewport.New(w, h)
	vp.SetContent(buildReadContent(lines, lang))
	return vp
}

func buildReadContent(lines []Line, lang string) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderLine(line, lang))
	}
	return b.String()
}

// ---------- 通用布局 ----------

func renderHeader(title string) string {
	return styleAppTitle.Render("  RETRO BBS") + "  " + styleAppSubtitle.Render("· "+title)
}

func renderStatusBar(hints [][2]string) string {
	var parts []string
	for _, h := range hints {
		parts = append(parts, styleStatusKey.Render(h[0])+" "+h[1])
	}
	return styleStatusBar.Render("  " + strings.Join(parts, "   "))
}

func fitToHeight(s string, height int) string {
	used := lipgloss.Height(s)
	if used < height {
		s += strings.Repeat("\n", height-used)
	}
	return s
}

func renderList(menu []MenuItem, selected, maxLines int, lang string) string {
	var b strings.Builder
	count := 0
	for i, it := range menu {
		if maxLines > 0 && count >= maxLines-1 && len(menu)-i > 1 {
			b.WriteString(styleHint.Render("  ... "+itoa(len(menu)-i)+" more") + "\n")
			count++
			break
		}
		key := ""
		if it.Key != "" {
			key = "[" + strings.ToUpper(it.Key) + "] "
		}
		label := it.Label
		if it.LabelKey != "" {
			label = tr(lang, it.LabelKey, it.Default)
		}
		if i == selected {
			b.WriteString(styleSelected.Render("▶ "+key+label) + "\n")
		} else {
			b.WriteString("  " + key + label + "\n")
		}
		count++
	}
	return b.String()
}
