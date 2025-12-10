package templates

import (
	_ "embed"
	"html/template"
	"strings"
	"time"
)

//go:embed otp.html
var otpHTML string

var otpTmpl = template.Must(template.New("otp").Parse(otpHTML))

// OTPData holds all possible fields for generic OTP emails.
// Optional fields can be empty — template handles gracefully.
type OTPData struct {
	OTP           string // Required
	Purpose       string // e.g. "login", "pin_update", "funding_verification"
	Year          int    // Auto-set if 0
	LogoURL       string // Auto-set if empty
	Subject       string // Auto-set if empty
	HeaderTitle   string // Auto-set if empty
	Description   string // Auto-set if empty
	ExpiryMinutes int    // Auto-set if 0 (defaults to 10)
}

func RenderOTPEmail(otp string) (string, error) {
	return RenderOTPEmailWithData(OTPData{
		OTP:     otp,
		Purpose: "login", // default
	})
}

func RenderOTPEmailWithData(data OTPData) (string, error) {
	// Auto-fill defaults
	if data.Year == 0 {
		data.Year = time.Now().Year()
	}
	if data.LogoURL == "" {
		data.LogoURL = "https://musterbox.org/icon.png"
	}
	if data.Purpose == "" {
		data.Purpose = "login" // fallback
	}

	// Set dynamic subject/header/description based on purpose
	if data.Subject == "" {
		data.Subject = GetSubject(data.Purpose) 
	}
	if data.HeaderTitle == "" {
		data.HeaderTitle = getHeaderTitle(data.Purpose)
	}
	if data.Description == "" {
		data.Description = getDescription(data.Purpose)
	}
	if data.ExpiryMinutes == 0 {
		data.ExpiryMinutes = 10 // default
	}

	var buf strings.Builder
	err := otpTmpl.Execute(&buf, data)
	return buf.String(), err
}

// ———————————————————————————————————————
// Helper Functions
// ———————————————————————————————————————


func GetSubject(purpose string) string {
	switch purpose {
	case "pin_update":
		return "PIN Update Verification Code"
	case "funding_verification", "funding":
		return "Funding Verification Code"
	case "withdrawal":
		return "Withdrawal Verification Code"
	case "login", "pin_recovery":
		return "PIN Recovery Verification Code"
	default:
		return "Verification Code"
	}
}

func getHeaderTitle(purpose string) string {
	switch purpose {
	case "pin_update":
		return "PIN Update Code"
	case "funding_verification", "funding":
		return "Funding Verification"
	case "withdrawal":
		return "Withdrawal Code"
	case "login":
		return "Login Verification Code"
	default:
		return "Verification Code"
	}
}

func getDescription(purpose string) string {
	switch purpose {
	case "pin_update":
		return "Your one-time code to update your PIN is:"
	case "funding_verification", "funding":
		return "Your one-time code to verify your funding request is:"
	case "withdrawal":
		return "Your one-time code to authorize your withdrawal is:"
	case "login":
		return "Your one-time login code is:"
	default:
		return "Your one-time verification code is:"
	}
}