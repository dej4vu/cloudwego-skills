package service

import "strings"

// normalizeLang 归一化语言码，未知语言回退到 en。
func normalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch lang {
	case "zh", "en", "ja":
		return lang
	}
	return "en"
}
