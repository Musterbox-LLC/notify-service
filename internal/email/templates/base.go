package templates

import (
	_ "embed"
)

//go:embed verification.html
var VerificationHTML string

//go:embed password_reset.html
var PasswordResetHTML string

//go:embed otp.html
var OtpHTML string

//go:embed new_login.html
var newLoginHTML string

//go:embed deposit_detected.html
var depositDetectedHTML string

//go:embed withdraw_completed.html
var withdrawCompletedHTML string

//go:embed conversion_sol_to_fiat.html
var conversionSolToFiatHTML string

//go:embed conversion_fiat_to_sol.html
var conversionFiatToSolHTML string