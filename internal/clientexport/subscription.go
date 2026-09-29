package clientexport

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
)

// SubscriptionURL preserves the exact generated URL, including escaped tokens.
// Validation does not fetch it or normalize credentials into a different URL.
func SubscriptionURL(id, revision, raw string) (*Document, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Opaque != "" || u.Fragment != "" ||
		strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, errors.New("subscription export requires a valid HTTPS URL")
	}
	return NewDocument(id, revision, "subscription", "subscription.txt", raw)
}
