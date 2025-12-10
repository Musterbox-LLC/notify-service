// notify-service/internal/email/templates/conversion_fiat_to_sol.go
package templates

import (
	_ "embed"
	"html/template"
	"strings"
	"time"
)


var conversionFiatToSolTmpl = template.Must(template.New("conversion_fiat_to_sol").Parse(conversionFiatToSolHTML))

type ConversionFiatToSolData struct {
	UserName       string
	FiatAmount     string // e.g., "10,000.00"
	FiatCurrency   string // e.g., "NGN"
	SOLAmount      string // e.g., "0.0667"
	FeeAmountFiat  string // e.g., "150.00"
	ExchangeRate   string // e.g., "150,000.00"
	TxID           string
	Timestamp      string
	LogoURL        string
	Year           int
}

func RenderConversionFiatToSolEmail(data ConversionFiatToSolData) (string, error) {
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://www.musterbox.org/icon.png"
	}
	var buf strings.Builder
	err := conversionFiatToSolTmpl.Execute(&buf, data)
	return buf.String(), err
}