package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

var i18nData map[string]map[string]string

// loadI18n 从服务端 HTTP 拉取 i18n.json。
// ws://host/ws → http://host/i18n.json
func loadI18n(wsURL string) {
	base := wsURL
	if strings.HasPrefix(base, "ws://") {
		base = "http://" + base[5:]
	} else if strings.HasPrefix(base, "wss://") {
		base = "https://" + base[6:]
	}
	base = strings.TrimSuffix(base, "/ws")
	url := base + "/i18n.json"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("i18n: load %s failed: %v", url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("i18n: load %s status %d", url, resp.StatusCode)
		return
	}
	var data map[string]map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("i18n: parse %s failed: %v", url, err)
		return
	}
	i18nData = data
}

// tr 翻译 key。查找顺序：lang → en → fallback → key 本身。
func tr(lang, key, fallback string, args ...string) string {
	if key == "" {
		return ""
	}
	if pack, ok := i18nData[lang]; ok {
		if s, ok := pack[key]; ok {
			return formatT(s, args)
		}
	}
	if pack, ok := i18nData["en"]; ok {
		if s, ok := pack[key]; ok {
			return formatT(s, args)
		}
	}
	if fallback != "" {
		return formatT(fallback, args)
	}
	return key
}

func formatT(s string, args []string) string {
	if len(args) == 0 {
		return s
	}
	out := s
	for i, a := range args {
		out = replaceOnce(out, "{"+itoa(i)+"}", a)
	}
	return out
}

func replaceOnce(s, from, to string) string {
	idx := indexOf(s, from)
	if idx < 0 {
		return s
	}
	return s[:idx] + to + s[idx+len(from):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
