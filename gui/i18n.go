package gui

import "fmt"

// Language codes stored in preferences.
const (
	LangEnglish = "en"
	LangPersian = "fa"
)

// lang is the active UI language; set before the UI is built.
var lang = LangEnglish

// T translates an English UI string; unknown strings stay English.
var missing map[string]bool // test hook: collects untranslated strings

func T(s string) string {
	if missing != nil && faStrings[s] == "" && s != "" {
		missing[s] = true
	}
	if lang == LangPersian {
		if v, ok := faStrings[s]; ok {
			return v
		}
	}
	return s
}

// Tf translates a format string, then formats it.
func Tf(format string, args ...any) string { return fmt.Sprintf(T(format), args...) }
