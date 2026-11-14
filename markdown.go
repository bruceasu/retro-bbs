package main

import (
	"strings"
	"unicode"
)

// markdownToLines 把 Markdown 文本解析为 []Line。
// imagesOn 控制 ![](url) 是否显示 URL；关闭时只显示 [image: alt]。
func markdownToLines(md string, imagesOn bool) []Line {
	var out []Line
	lines := strings.Split(md, "\n")

	inCode := false
	var codeLang string

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")

		// 围栏代码块
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if !inCode {
				inCode = true
				codeLang = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				label := "code"
				if codeLang != "" {
					label = codeLang
				}
				out = append(out, Line{Raw: "--- " + label + " ---", Kind: "code"})
			} else {
				inCode = false
				codeLang = ""
				out = append(out, Line{Raw: "-----------", Kind: "code"})
			}
			continue
		}
		if inCode {
			out = append(out, Line{Raw: "  " + line, Kind: "code"})
			continue
		}

		trim := strings.TrimSpace(line)

		// 空行
		if trim == "" {
			out = append(out, Line{})
			continue
		}

		// 水平线
		if isHR(trim) {
			out = append(out, Line{Raw: strings.Repeat("─", 40), Kind: "hr"})
			continue
		}

		// 标题
		if strings.HasPrefix(trim, "### ") {
			out = append(out, Line{Raw: strings.TrimPrefix(trim, "### "), Kind: "h2"})
			continue
		}
		if strings.HasPrefix(trim, "## ") {
			out = append(out, Line{Raw: strings.TrimPrefix(trim, "## "), Kind: "h2"})
			continue
		}
		if strings.HasPrefix(trim, "# ") {
			out = append(out, Line{Raw: strings.TrimPrefix(trim, "# "), Kind: "h1"})
			continue
		}

		// 引用
		if strings.HasPrefix(trim, "> ") {
			spans := parseInline(strings.TrimPrefix(trim, "> "), imagesOn)
			out = append(out, Line{Spans: spans, Kind: "quote"})
			continue
		}

		// 列表
		if m, ok := listPrefix(trim); ok {
			content := strings.TrimSpace(trim[len(m):])
			spans := parseInline(content, imagesOn)
			out = append(out, Line{Spans: prependSpan(Span{Text: m}, spans), Kind: ""})
			continue
		}

		// 普通段落
		out = append(out, Line{Spans: parseInline(trim, imagesOn)})
	}
	return out
}

func isHR(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			if unicode.IsSpace(rune(s[i])) {
				continue
			}
			return false
		}
	}
	return true
}

func listPrefix(s string) (string, bool) {
	// 无序：- * +
	if len(s) >= 2 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ' {
		return "  • ", true
	}
	// 有序：1. 2) 等
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s) && (s[i] == '.' || s[i] == ')') && i+1 < len(s) && s[i+1] == ' ' {
		return "  " + s[:i+1] + " ", true
	}
	return "", false
}

func prependSpan(first Span, rest []Span) []Span {
	out := make([]Span, 0, len(rest)+1)
	out = append(out, first)
	out = append(out, rest...)
	return out
}

// parseInline 处理 **bold**、*italic*、`code`、[text](url)、![alt](url)
func parseInline(s string, imagesOn bool) []Span {
	var spans []Span
	var buf strings.Builder

	flush := func() {
		if buf.Len() > 0 {
			spans = append(spans, Span{Text: buf.String()})
			buf.Reset()
		}
	}

	i := 0
	for i < len(s) {
		// 图片 ![](url) 或 ![alt](url)
		if s[i] == '!' && i+1 < len(s) && s[i+1] == '[' {
			if alt, url, n, ok := parseLink(s[i+1:]); ok {
				flush()
				label := "[image: " + alt + "]"
				if imagesOn && url != "" {
					label += " " + url
				}
				spans = append(spans, Span{Text: label, Style: "link", URL: url})
				i += 1 + n
				continue
			}
		}
		// 链接 [text](url)
		if s[i] == '[' {
			if text, url, n, ok := parseLink(s[i:]); ok {
				flush()
				spans = append(spans, Span{Text: text, Style: "link", URL: url})
				i += n
				continue
			}
		}
		// 行内代码 `code`
		if s[i] == '`' {
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				flush()
				spans = append(spans, Span{Text: s[i+1 : i+1+j], Style: "code"})
				i += j + 2
				continue
			}
		}
		// 粗体 **x**
		if strings.HasPrefix(s[i:], "**") {
			if j := strings.Index(s[i+2:], "**"); j >= 0 {
				flush()
				spans = append(spans, Span{Text: s[i+2 : i+2+j], Style: "bold"})
				i += j + 4
				continue
			}
		}
		// 斜体 *x*（排除 ** 已处理）
		if s[i] == '*' && !strings.HasPrefix(s[i:], "**") {
			if j := strings.IndexByte(s[i+1:], '*'); j >= 0 {
				flush()
				spans = append(spans, Span{Text: s[i+1 : i+1+j], Style: "italic"})
				i += j + 2
				continue
			}
		}
		buf.WriteByte(s[i])
		i++
	}
	flush()
	if len(spans) == 0 {
		return []Span{{Text: s}}
	}
	return spans
}

// parseLink 解析 [text](url)，返回 text,url,consumed,ok
func parseLink(s string) (string, string, int, bool) {
	if len(s) < 4 || s[0] != '[' {
		return "", "", 0, false
	}
	closeBracket := strings.IndexByte(s, ']')
	if closeBracket < 0 || closeBracket+1 >= len(s) || s[closeBracket+1] != '(' {
		return "", "", 0, false
	}
	closeParen := strings.IndexByte(s[closeBracket+2:], ')')
	if closeParen < 0 {
		return "", "", 0, false
	}
	text := s[1:closeBracket]
	url := s[closeBracket+2 : closeBracket+2+closeParen]
	return text, url, closeBracket + 2 + closeParen + 1, true
}