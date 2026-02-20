package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type localizer struct {
	locale string
	items  map[string]string
}

func newLocalizer(preferred string) *localizer {
	locale := resolveLocale(preferred)
	dict := loadCatalog(locale)
	if dict == nil {
		locale = "en"
		dict = loadCatalog(locale)
	}
	if dict == nil {
		dict = map[string]string{}
	}
	return &localizer{locale: locale, items: dict}
}

func (t *localizer) T(key string) string {
	if t == nil {
		return key
	}
	if v, ok := t.items[key]; ok {
		return v
	}
	if v := fallbackCatalog(key); v != "" {
		return v
	}
	return key
}

type tuiI18n = localizer

func newTUII18n(preferred string) *tuiI18n {
	return newLocalizer(preferred)
}

func resolveLocale(preferred string) string {
	p := strings.ToLower(strings.TrimSpace(preferred))
	if p != "" && p != "auto" {
		if strings.HasPrefix(p, "zh") {
			return "zh-CN"
		}
		return "en"
	}
	lang := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(os.Getenv("MAIBOT_LANG")),
		strings.TrimSpace(os.Getenv("LC_ALL")),
		strings.TrimSpace(os.Getenv("LANG")),
	}, " ")))
	if strings.Contains(lang, "zh") {
		return "zh-CN"
	}
	return "en"
}

func fallbackCatalog(key string) string {
	if key == "" {
		return ""
	}
	cat := loadCatalog("en")
	if cat == nil {
		return ""
	}
	return cat[key]
}

func loadCatalog(locale string) map[string]string {
	path := resolveI18nPath(locale)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var catalog map[string]string
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil
	}
	return catalog
}

func resolveI18nPath(locale string) string {
	if locale == "" {
		return ""
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	base := filepath.Join(filepath.Dir(file), "i18n")
	return filepath.Join(base, locale+".json")
}
