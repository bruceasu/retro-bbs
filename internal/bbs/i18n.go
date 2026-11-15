package bbs

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

//go:embed i18n.json
var i18nJSON []byte

var i18nData map[string]map[string]string

func initI18n() error {
	if err := json.Unmarshal(i18nJSON, &i18nData); err != nil {
		return fmt.Errorf("parse i18n.json: %w", err)
	}
	en, ok := i18nData["en"]
	if !ok || len(en) == 0 {
		return fmt.Errorf("i18n.json: missing or empty 'en' pack")
	}
	for lang, pack := range i18nData {
		if lang == "en" {
			continue
		}
		for k := range pack {
			if _, ok := en[k]; !ok {
				return fmt.Errorf("i18n.json: key %q exists in %q but not in en", k, lang)
			}
		}
		for k := range en {
			if _, ok := pack[k]; !ok {
				return fmt.Errorf("i18n.json: key %q exists in en but not in %q", k, lang)
			}
		}
	}
	log.Printf("i18n: %d keys, %d languages", len(en), len(i18nData))
	return nil
}

// T 返回英文默认翻译；找不到时记日志并返回 key 本身。
func T(key string) string {
	if key == "" {
		return ""
	}
	if s, ok := i18nData["en"][key]; ok {
		return s
	}
	log.Printf("i18n: missing key %q", key)
	return key
}

// fillDefaults 给 Screen 里所有 Key 补上英文 Default。业务代码无需改动。
func fillDefaults(sc *Screen) {
	if sc.Title != "" && sc.TitleDefault == "" {
		sc.TitleDefault = T(sc.Title)
	}
	if sc.Hint != "" && sc.HintDefault == "" {
		sc.HintDefault = T(sc.Hint)
	}
	for i := range sc.Lines {
		if sc.Lines[i].Key != "" && sc.Lines[i].Default == "" {
			sc.Lines[i].Default = T(sc.Lines[i].Key)
		}
	}
	for i := range sc.Menu {
		if sc.Menu[i].LabelKey != "" && sc.Menu[i].Default == "" {
			sc.Menu[i].Default = T(sc.Menu[i].LabelKey)
		}
	}
	if sc.Form != nil {
		for i := range sc.Form.Fields {
			if sc.Form.Fields[i].LabelKey != "" && sc.Form.Fields[i].Default == "" {
				sc.Form.Fields[i].Default = T(sc.Form.Fields[i].LabelKey)
			}
		}
	}
}

// handleI18nJSON 把内嵌的 i18n.json 原样返回给客户端。
func handleI18nJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(i18nJSON)
}
