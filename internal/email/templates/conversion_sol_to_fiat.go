// notify-service/internal/email/templates/conversion_sol_to_fiat.go
package templates

import (
	_ "embed"
	"html/template"
	"strings"
	"time"
)


var conversionSolToFiatTmpl = template.Must(template.New("conversion_sol_to_fiat").Parse(conversionSolToFiatHTML))

type ConversionSolToFiatData struct {
	UserName      string
	SOLAmount     string // e.g., "0.5"
	FiatAmount    string // e.g., "75,000.00"
	FiatCurrency  string // e.g., "NGN"
	FeeAmountSOL  string // e.g., "0.005"
	ExchangeRate  string // e.g., "150,000.00"
	TxID          string
	Timestamp     string
	LogoURL       string
	Year          int
}

func RenderConversionSolToFiatEmail(data ConversionSolToFiatData) (string, error) {
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://www.musterbox.org/icon.png"
	}
	var buf strings.Builder
	err := conversionSolToFiatTmpl.Execute(&buf, data)
	return buf.String(), err
}