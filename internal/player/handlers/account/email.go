package account

import "strings"

// MaskEmail obscures an email address for display, e.g. "r*****k@gmail.com".
func MaskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]

	if len(local) <= 2 {
		return local[:1] + "*" + domain
	}
	return string(local[0]) + strings.Repeat("*", len(local)-2) + string(local[len(local)-1]) + domain
}
