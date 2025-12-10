// notify-service/internal/email/templates/new_login.go
package templates

import (
	_ "embed"
	"html/template"
	"strings"
	"time"
)


var newLoginTmpl = template.Must(template.New("new_login").Parse(newLoginHTML))

// NewLoginData holds the data for the new login detected email.
type NewLoginData struct {
	UserName         string // Template uses {{.UserName}} (capital U)
	Timestamp        string // formatted RFC3339 or human-readable. Template uses {{.Timestamp}} (capital T)
	IPAddress        string // Template uses {{.IPAddress}} (capital I, P)
	DeviceOS         string // Template uses {{.DeviceOS}} (capital D, O, S)
	UserAgentSnippet string // Template uses {{.UserAgentSnippet}} (capital U, A, S)
	LogoURL          string // Template uses {{.LogoURL}} (capital L, U)
	Year             int    // Template uses {{.Year}} (capital Y)
}

// RenderNewLoginEmail renders the new login detected email HTML.
func RenderNewLoginEmail(data NewLoginData) (string, error) {
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://www.musterbox.org/icon.png" // Removed trailing spaces
	}
	var buf strings.Builder
	err := newLoginTmpl.Execute(&buf, data)
	return buf.String(), err
}