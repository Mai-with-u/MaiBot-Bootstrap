package app

import "testing"

func TestResolveLocale(t *testing.T) {
	t.Setenv("MAIBOT_LANG", "zh_CN.UTF-8")
	if got := resolveLocale("auto"); got != "zh-CN" {
		t.Fatalf("resolveLocale(auto) = %q, want zh-CN", got)
	}

	t.Setenv("MAIBOT_LANG", "")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := resolveLocale("auto"); got != "en" {
		t.Fatalf("resolveLocale(auto) = %q, want en", got)
	}

	if got := resolveLocale("zh"); got != "zh-CN" {
		t.Fatalf("resolveLocale(zh) = %q, want zh-CN", got)
	}
}

func TestTranslatorFallback(t *testing.T) {
	tr := newTUII18n("en")
	if tr.T("action.quit") == "action.quit" {
		t.Fatalf("translator did not return localized value")
	}
	if tr.T("unknown.key") != "unknown.key" {
		t.Fatalf("unknown key fallback mismatch")
	}
}
