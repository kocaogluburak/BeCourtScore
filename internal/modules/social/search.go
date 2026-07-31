package social

import "strings"

// escapeLike escapes \, %, and _ for use with LIKE ... ESCAPE '\'.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// maskEmail returns a privacy-preserving form like "b***@gmail.com".
func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	runes := []rune(local)
	if len(runes) == 0 {
		return "***" + domain
	}
	if len(runes) == 1 {
		return string(runes[0]) + "***" + domain
	}
	return string(runes[0]) + "***" + domain
}

func looksLikeEmail(q string) bool {
	return strings.Contains(q, "@")
}
