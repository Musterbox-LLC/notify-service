// internal/email/templates/verification.go
package templates

import (
	_ "embed"
	
	"html/template"
	"strings"
	"time"
)

//go:embed verification.html
var verificationHTML string

var verificationTmpl = template.Must(template.New("verification").Parse(verificationHTML))

type VerificationData struct {
	VerifyURL string
	Year      int
	LogoURL   string
}

func RenderEmailVerification(data VerificationData) (string, error) {
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://temp-admin.musterbox.org/icon.png"
	}

	var buf strings.Builder
	err := verificationTmpl.Execute(&buf, data)
	return buf.String(), err
}