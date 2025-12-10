// notify-service/internal/email/templates/withdraw_completed.go
package templates

import (
	_ "embed"
	"html/template"
	"strings"
	"time"
)

var withdrawCompletedTmpl = template.Must(template.New("withdraw_completed").Parse(withdrawCompletedHTML))

type WithdrawCompletedData struct {
	UserName     string
	Amount       string // e.g., "50.00"
	Currency     string // e.g., "NGN"
	Destination  string // e.g., "Bank: Zenith ••••1234"
	TxID         string // e.g., "5xKJ..."
	FeeAmount    string // e.g., "1.50"
	Timestamp    string
	LogoURL      string
	Year         int
}

func RenderWithdrawCompletedEmail(data WithdrawCompletedData) (string, error) {
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://www.musterbox.org/icon.png"
	}
	var buf strings.Builder
	err := withdrawCompletedTmpl.Execute(&buf, data)
	return buf.String(), err
}