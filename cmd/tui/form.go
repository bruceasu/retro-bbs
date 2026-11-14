package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type formAction int

const (
	formNone formAction = iota
	formSubmit
	formCancel
	formSaveDraft
	formDeleteDraft
)

// editorDoneMsg 由 tea.ExecProcess 回调返回，携带外部编辑器的内容。
type editorDoneMsg struct {
	content string
	ok      bool
}

type formState struct {
	screen   string
	submit   string
	canDraft bool
	fields   []formField
	focus    int
	draftID  string

	width  int
	height int
}

type formField struct {
	name         string
	labelKey     string
	defaultLabel string
	secret       bool
	multiline    bool
	input        textinput.Model
	area         textarea.Model
}

func newFormState(sc *Screen, width, height int) *formState {
	fs := &formState{
		screen:   sc.Screen,
		submit:   sc.Form.Submit,
		canDraft: sc.Form.CanDraft,
		width:    width,
		height:   height,
	}
	for _, f := range sc.Form.Fields {
		if f.LabelKey == "" {
			if f.Name == "draft_id" {
				fs.draftID = f.Value
			}
			continue
		}
		ff := formField{
			name:         f.Name,
			labelKey:     f.LabelKey,
			defaultLabel: f.Default,
			secret:       f.Secret,
			multiline:    f.Multiline,
		}
		if f.Multiline {
			ta := textarea.New()
			ta.SetValue(f.Value)
			ta.CharLimit = 0
			ta.ShowLineNumbers = false
			ta.Prompt = ""
			ta.Placeholder = ""
			ta.Blur()
			ff.area = ta
		} else {
			ti := textinput.New()
			ti.SetValue(f.Value)
			if f.Secret {
				ti.EchoMode = textinput.EchoPassword
				ti.EchoCharacter = '•'
			}
			ti.CharLimit = 0
			ti.Prompt = ""
			ti.Blur()
			ff.input = ti
		}
		fs.fields = append(fs.fields, ff)
	}
	fs.SetSize(width, height)
	fs.focusField(0)
	return fs
}

func (fs *formState) focusField(i int) {
	if i < 0 || i >= len(fs.fields) {
		return
	}
	for j := range fs.fields {
		if fs.fields[j].multiline {
			if j == i {
				fs.fields[j].area.Focus()
			} else {
				fs.fields[j].area.Blur()
			}
		} else {
			if j == i {
				fs.fields[j].input.Focus()
			} else {
				fs.fields[j].input.Blur()
			}
		}
	}
	fs.focus = i
}

func (fs *formState) SetSize(width, height int) {
	fs.width = width
	fs.height = height
	fieldWidth := width - 6
	if fieldWidth < 20 {
		fieldWidth = 20
	}
	areaHeight := height / 2
	if areaHeight < 6 {
		areaHeight = 6
	}
	if areaHeight > 20 {
		areaHeight = 20
	}
	for i := range fs.fields {
		if fs.fields[i].multiline {
			fs.fields[i].area.SetWidth(fieldWidth)
			fs.fields[i].area.SetHeight(areaHeight)
		}
	}
}

// UpdateKey 处理按键。新语义：
//
//	Enter      = 单行提交 / 多行换行
//	Ctrl+G     = 提交
//	Ctrl+S     = 保存草稿（canDraft）
//	Ctrl+D     = 删除当前草稿（draftID 非空）
//	Ctrl+E     = 外部编辑器（多行）
//	Esc        = 取消
//	Tab / Shift+Tab = 切换字段
func (fs *formState) UpdateKey(k tea.KeyMsg) (formAction, tea.Cmd) {
	if len(fs.fields) == 0 {
		return formNone, nil
	}

	switch k.Type {
	case tea.KeyTab:
		fs.focusField((fs.focus + 1) % len(fs.fields))
		return formNone, nil
	case tea.KeyShiftTab:
		fs.focusField((fs.focus - 1 + len(fs.fields)) % len(fs.fields))
		return formNone, nil
	case tea.KeyEsc:
		return formCancel, nil
	case tea.KeyCtrlG:
		return formSubmit, nil
	case tea.KeyCtrlS:
		if fs.canDraft {
			return formSaveDraft, nil
		}
		return formNone, nil
	case tea.KeyCtrlD:
		if fs.draftID != "" {
			return formDeleteDraft, nil
		}
		return formNone, nil
	case tea.KeyCtrlE:
		f := &fs.fields[fs.focus]
		if f.multiline {
			return formNone, fs.openExternalEditor()
		}
		// 单行 Ctrl+E 交给 textinput 处理（emacs end-of-line）
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(k)
		return formNone, cmd
	case tea.KeyEnter:
		f := &fs.fields[fs.focus]
		if !f.multiline {
			return formSubmit, nil
		}
		var cmd tea.Cmd
		f.area, cmd = f.area.Update(k)
		return formNone, cmd
	}

	f := &fs.fields[fs.focus]
	var cmd tea.Cmd
	if f.multiline {
		f.area, cmd = f.area.Update(k)
	} else {
		f.input, cmd = f.input.Update(k)
	}
	return formNone, cmd
}

func (fs *formState) UpdateMsg(msg tea.Msg) tea.Cmd {
	if m, ok := msg.(editorDoneMsg); ok {
		fs.applyEditorResult(m)
		return nil
	}
	if len(fs.fields) == 0 {
		return nil
	}
	f := &fs.fields[fs.focus]
	var cmd tea.Cmd
	if f.multiline {
		f.area, cmd = f.area.Update(msg)
	} else {
		f.input, cmd = f.input.Update(msg)
	}
	return cmd
}

func (fs *formState) collect() map[string]string {
	out := make(map[string]string, len(fs.fields)+1)
	for _, f := range fs.fields {
		if f.multiline {
			out[f.name] = f.area.Value()
		} else {
			out[f.name] = f.input.Value()
		}
	}
	if fs.draftID != "" {
		out["draft_id"] = fs.draftID
	}
	return out
}

func (fs *formState) openExternalEditor() tea.Cmd {
	f := &fs.fields[fs.focus]
	if !f.multiline {
		return nil
	}

	editor := resolveEditor()
	content := f.area.Value()

	tmpFile := filepath.Join(os.TempDir(), "retro-bbs-editor-"+f.name+".md")
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		return nil
	}

	parts := splitCommand(editor)
	if len(parts) == 0 {
		_ = os.Remove(tmpFile)
		return nil
	}
	args := append(parts[1:], tmpFile)
	cmd := exec.Command(parts[0], args...)

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		data, readErr := os.ReadFile(tmpFile)
		_ = os.Remove(tmpFile)
		if err != nil || readErr != nil {
			return editorDoneMsg{ok: false}
		}
		return editorDoneMsg{content: string(data), ok: true}
	})
}

func (fs *formState) applyEditorResult(msg editorDoneMsg) {
	if !msg.ok {
		return
	}
	f := &fs.fields[fs.focus]
	if f.multiline {
		f.area.SetValue(msg.content)
	}
}

func resolveEditor() string {
	if p := loadPrefs(); p.Editor != "" {
		return p.Editor
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if _, err := exec.LookPath("notepad"); err == nil {
		return "notepad"
	}
	return "vi"
}

func splitCommand(s string) []string {
	return strings.Fields(s)
}

func (fs *formState) View(lang string) string {
	var b strings.Builder
	for i := range fs.fields {
		f := &fs.fields[i]
		focused := i == fs.focus
		label := tr(lang, f.labelKey, f.defaultLabel)

		if focused {
			b.WriteString(styleLabelActive.Render("> " + label))
		} else {
			b.WriteString(styleLabelInactive.Render("  " + label))
		}
		b.WriteString("\n")

		if f.multiline {
			b.WriteString("  ")
			view := f.area.View()
			view = strings.ReplaceAll(view, "\n", "\n  ")
			b.WriteString(view)
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(f.input.View())
			b.WriteString("\n")
		}
	}
	return b.String()
}
