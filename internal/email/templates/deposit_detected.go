// notify-service/internal/email/templates/deposit_detected.go
package templates

import (
	_ "embed"
	"html/template"
	"strings"
	"time"
)


var depositDetectedTmpl = template.Must(template.New("deposit_detected").Parse(depositDetectedHTML))

type DepositDetectedData struct {
	UserName     string
	Amount       string // e.g., "0.5"
	Currency     string // e.g., "SOL"
	NewBalance   string // e.g., "2.3"
	TxID         string
	Timestamp    string
	LogoURL      string
	Year         int
}

func RenderDepositDetectedEmail(data DepositDetectedData) (string, error) {
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://www.musterbox.org/icon.png"
	}
	var buf strings.Builder
	err := depositDetectedTmpl.Execute(&buf, data)
	return buf.String(), err
}