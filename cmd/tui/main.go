package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	url := os.Getenv("BBS_URL")
	if url == "" {
		url = "ws://localhost:8080/ws"
	}

	loadI18n(url)

	lang := detectTUILang()

	client, err := DialWS(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect %s: %v\n", url, err)
		os.Exit(1)
	}
	defer client.Close()

	p := tea.NewProgram(NewModel(client, lang), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}

func detectTUILang() string {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("BBS_LANG"))); v == "zh" || v == "en" {
		return v
	}
	prefs := loadPrefs()
	if prefs.Lang == "zh" || prefs.Lang == "en" {
		return prefs.Lang
	}
	for _, key := range []string{"LANG", "LANGUAGE", "LC_ALL", "LC_MESSAGES"} {
		if v := strings.ToLower(os.Getenv(key)); strings.HasPrefix(v, "zh") {
			return "zh"
		}
	}
	return "en"
}
