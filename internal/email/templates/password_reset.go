// internal/email/templates/password_reset.go
package templates

import (
	_ "embed"
	"fmt"
	"html/template"
	"log" // <--- Add this import
	"strings"
	"time"
)

//go:embed password_reset.html
var passwordResetHTML string

var passwordResetTmpl = template.Must(template.New("password_reset").Parse(passwordResetHTML))

type PasswordResetData struct {
	ResetLink string
	Year      int
	LogoURL   string
}

func RenderPasswordResetEmail(data PasswordResetData) (string, error) {
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://www.musterbox.org/icon.png"
	}

	// Debug: Print template length and first 100 chars
	// log.Printf("📧 [DEBUG] Password Reset Template Length: %d", len(passwordResetHTML))
	if len(passwordResetHTML) < 100 {
		// log.Printf("📧 [DEBUG] Password Reset Template Content: %q", passwordResetHTML)
	}

	var buf strings.Builder
	err := passwordResetTmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("❌ [ERROR] Password Reset Template Execution Failed: %v", err)
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	log.Printf("📧 [DEBUG] Password Reset Rendered Length: %d", buf.Len())
	if buf.Len() < 100 {
		log.Printf("📧 [DEBUG] Password Reset Rendered Content: %q", buf.String())
	}

	return buf.String(), nil
}