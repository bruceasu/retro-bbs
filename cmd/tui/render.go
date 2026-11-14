package main

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[90m"
	ansiGreen  = "\x1b[32m"
	ansiCyan   = "\x1b[36m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiBlue   = "\x1b[34m"
	ansiItalic = "\x1b[3m"
	ansiUnder  = "\x1b[4m"
	ansiSel    = "\x1b[1;30;42m"
)

func kindColor(kind string) string {
	switch kind {
	case "err":
		return ansiRed
	case "ok":
		return ansiGreen
	case "info":
		return ansiCyan
	case "author":
		return ansiDim
	case "h1":
		return ansiBold + ansiCyan
	case "h2":
		return ansiBold + ansiYellow
	case "quote":
		return ansiGreen
	case "code", "hr":
		return ansiDim
	}
	return ""
}

func spanStyle(style string) string {
	switch style {
	case "bold":
		return ansiBold
	case "italic":
		return ansiItalic
	case "code":
		return ansiCyan
	case "link":
		return ansiUnder + ansiBlue
	}
	return ""
}

func renderLine(line Line, lang string) string {
	color := kindColor(line.Kind)
	reset := ""
	if color != "" {
		reset = ansiReset
	}
	if len(line.Spans) > 0 {
		var b []byte
		b = append(b, color...)
		for _, sp := range line.Spans {
			st := spanStyle(sp.Style)
			b = append(b, st...)
			b = append(b, sp.Text...)
			b = append(b, reset...)
		}
		b = append(b, reset...)
		return string(b)
	}
	var text string
	if line.Key != "" {
		text = tr(lang, line.Key, line.Default, line.Args...)
	} else {
		text = line.Raw
	}
	return color + text + reset
}
