package main

import "encoding/json"

type Message struct {
	ID    int64           `json:"id,omitempty"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data,omitempty"`
	OK    *bool           `json:"ok,omitempty"`
	Error *ErrorInfo      `json:"error,omitempty"`
	Text  string          `json:"text,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Screen struct {
	Screen       string     `json:"screen"`
	Title        string     `json:"title"`
	TitleDefault string     `json:"title_default,omitempty"`
	Lines        []Line     `json:"lines,omitempty"`
	Menu         []MenuItem `json:"menu,omitempty"`
	Form         *Form      `json:"form,omitempty"`
	Hint         string     `json:"hint,omitempty"`
	HintDefault  string     `json:"hint_default,omitempty"`
	ImagesOn     bool       `json:"images_on,omitempty"`

	Page      int  `json:"page,omitempty"`
	TotalPage int  `json:"total_page,omitempty"`
	CanPrev   bool `json:"can_prev,omitempty"`
	CanNext   bool `json:"can_next,omitempty"`
}

type Line struct {
	Key     string   `json:"key,omitempty"`
	Args    []string `json:"args,omitempty"`
	Default string   `json:"default,omitempty"`
	Raw     string   `json:"raw,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Spans   []Span   `json:"spans,omitempty"`
}

type Span struct {
	Text  string `json:"text"`
	Style string `json:"style,omitempty"`
	URL   string `json:"url,omitempty"`
}

type MenuItem struct {
	Key      string `json:"key,omitempty"`
	LabelKey string `json:"label_key,omitempty"`
	Default  string `json:"default,omitempty"`
	Label    string `json:"label,omitempty"`
	Action   string `json:"action"`
}

type Form struct {
	Fields   []FormField `json:"fields"`
	Submit   string      `json:"submit"`
	CanDraft bool        `json:"can_draft,omitempty"`
}

type FormField struct {
	Name      string `json:"name"`
	LabelKey  string `json:"label_key"`
	Default   string `json:"default,omitempty"`
	Secret    bool   `json:"secret,omitempty"`
	Value     string `json:"value,omitempty"`
	Multiline bool   `json:"multiline,omitempty"`
}
