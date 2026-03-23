package deploy

import (
	"strings"
	"unicode"
)

func sanitizeDashedName(value string, maxLen int) string {
	lowered := strings.ToLower(value)
	var builder strings.Builder
	prevDash := false

	for _, r := range lowered {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if builder.Len() == 0 || prevDash {
				continue
			}
			builder.WriteByte('-')
			prevDash = true
		}
	}

	name := strings.Trim(builder.String(), "-")
	if maxLen > 0 && len(name) > maxLen {
		name = strings.Trim(name[:maxLen], "-")
	}
	return name
}
