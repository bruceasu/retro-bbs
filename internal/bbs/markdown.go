package bbs

import (
	"strings"
	"unicode"
)

func markdownToLines(md string, imagesOn bool) []Line {
	var out []Line
	lines := strings.Split(md, "\n")

	inCode := false
	var codeLang string

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")

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
		if trim == "" {
			out = append(out, Line{})
			continue
		}
		if isHR(trim) {
			out = append(out, Line{Raw: strings.Repeat("─", 40), Kind: "hr"})
			continue
		}
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
		if strings.HasPrefix(trim, "> ") {
			spans := parseInline(strings.TrimPrefix(trim, "> "), imagesOn)
			out = append(out, Line{Spans: spans, Kind: "quote"})
			continue
		}
		if m, ok := listPrefix(trim); ok {
			content := strings.TrimSpace(trim[len(m):])
			spans := parseInline(content, imagesOn)
			out = append(out, Line{Spans: prependSpan(Span{Text: m}, spans), Kind: ""})
			continue
		}
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
	if len(s) >= 2 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ' {
		return "  • ", true
	}
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

// isSafeURL 只允许 http:// https:// mailto: 和相对路径。
// 防止 javascript: data: file: 等危险 scheme。
func isSafeURL(u string) bool {
	u = strings.TrimSpace(strings.ToLower(u))
	if u == "" {
		return false
	}
	// 显式 scheme 检查
	if idx := strings.Index(u, ":"); idx > 0 {
		// 可能是 scheme（: 前面没有 /）
		if !strings.Contains(u[:idx], "/") {
			scheme := u[:idx]
			switch scheme {
			case "http", "https", "mailto":
				return true
			default:
				return false
			}
		}
	}
	// 无 scheme：相对路径允许
	return true
}

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
		// 图片 ![alt](url)
		if s[i] == '!' && i+1 < len(s) && s[i+1] == '[' {
			if alt, url, n, ok := parseLink(s[i+1:]); ok {
				flush()
				label := "[image: " + alt + "]"
				if imagesOn && url != "" && isSafeURL(url) {
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
				if isSafeURL(url) {
					spans = append(spans, Span{Text: text, Style: "link", URL: url})
				} else {
					// 不安全 URL：只显示文本，不显示 URL
					spans = append(spans, Span{Text: text})
				}
				i += n
				continue
			}
		}
		if s[i] == '`' {
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				flush()
				spans = append(spans, Span{Text: s[i+1 : i+1+j], Style: "code"})
				i += j + 2
				continue
			}
		}
		if strings.HasPrefix(s[i:], "**") {
			if j := strings.Index(s[i+2:], "**"); j >= 0 {
				flush()
				spans = append(spans, Span{Text: s[i+2 : i+2+j], Style: "bold"})
				i += j + 4
				continue
			}
		}
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
