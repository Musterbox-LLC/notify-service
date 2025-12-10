// notify-service/internal/notification/seed.go

package notification

import (
	"encoding/json"
	"fmt"
	"log"

	"gorm.io/gorm"
	"notify-service/pkg/models"
)

// helper converts []string -> datatypes.JSON safely
func jsonList(values []string) []byte {
	b, _ := json.Marshal(values)
	return b
}

// seedSystemNotificationTemplates populates the database with default system templates
func seedSystemNotificationTemplates(db *gorm.DB) error {

	templates := []models.SystemNotificationTemplate{
		{
			EventKey:     "user.login.success",
			Name:         "Login Successful",
			Enabled:      true,
			Heading:      "👋 Welcome back, {{user_name}}!",
			Title:        "Login Successful",
			Message:      "You signed in at {{timestamp}} from {{device_os}} ({{ip_address}}).",
			Type:         "success",
			Icon:         "unlock",
			TemplateVars: jsonList([]string{"user_name", "timestamp", "device_os", "ip_address"}),
		},
		{
			EventKey:     "user.login.failed",
			Name:         "Login Failed",
			Enabled:      true,
			Heading:      "⚠️ Suspicious Login Attempt",
			Title:        "Login Failed",
			Message:      "{{attempt_count}} failed attempts from {{ip_address}}. Account locked for {{lock_duration}} minutes.",
			Type:         "warning",
			Icon:         "shield-alert",
			TemplateVars: jsonList([]string{"user_name", "attempt_count", "ip_address", "lock_duration", "timestamp"}),
		},
		{
			EventKey:     "wallet.deposit.completed",
			Name:         "Deposit Confirmed",
			Enabled:      true,
			Heading:      "💰 Deposit of {{amount}} {{currency}} received",
			Title:        "Deposit Success",
			Message:      "Your deposit has been credited. New balance: {{new_balance}} {{currency}}.",
			Type:         "success",
			Icon:         "arrow-down-circle",
			TemplateVars: jsonList([]string{"user_name", "amount", "currency", "new_balance", "reference", "timestamp"}),
		},
		{
			EventKey:     "wallet.withdraw.requested",
			Name:         "Withdrawal Requested",
			Enabled:      true,
			Heading:      "📤 Withdrawal request for {{amount}} {{currency}}",
			Title:        "Withdrawal Initiated",
			Message:      "We're processing your withdrawal. Funds will reflect in {{estimated_time}}.",
			Type:         "info",
			Icon:         "arrow-up-circle",
			TemplateVars: jsonList([]string{"user_name", "amount", "currency", "estimated_time", "reference", "timestamp"}),
		},
		{
			EventKey:     "wallet.withdraw.completed",
			Name:         "Withdrawal Completed",
			Enabled:      true,
			Heading:      "✅ Withdrawal of {{amount}} {{currency}} sent",
			Title:        "Withdrawal Success",
			Message:      "Funds sent to {{destination}}. Transaction ID: {{txid}}.",
			Type:         "success",
			Icon:         "check-circle",
			TemplateVars: jsonList([]string{"user_name", "amount", "currency", "destination", "txid", "timestamp"}),
		},
		{
			EventKey:     "kyc.submitted",
			Name:         "KYC Submitted",
			Enabled:      true,
			Heading:      "📄 KYC documents submitted",
			Title:        "KYC In Review",
			Message:      "Your verification is being processed. We'll notify you shortly.",
			Type:         "info",
			Icon:         "file-text",
			TemplateVars: jsonList([]string{"user_name", "timestamp"}),
		},
		{
			EventKey:     "kyc.approved",
			Name:         "KYC Approved",
			Enabled:      true,
			Heading:      "🎉 KYC approved, {{user_name}}!",
			Title:        "Account Verified",
			Message:      "You can now deposit, withdraw, and play full games.",
			Type:         "success",
			Icon:         "user-check",
			TemplateVars: jsonList([]string{"user_name", "timestamp"}),
		},
		{
			EventKey:     "kyc.rejected",
			Name:         "KYC Rejected",
			Enabled:      true,
			Heading:      "❌ KYC rejected",
			Title:        "Verification Failed",
			Message:      "Reason: {{rejection_reason}}. You may resubmit with corrections.",
			Type:         "error",
			Icon:         "user-x",
			TemplateVars: jsonList([]string{"user_name", "rejection_reason", "timestamp"}),
		},
		{
			EventKey:     "account.suspended",
			Name:         "Account Suspended",
			Enabled:      true,
			Heading:      "🔒 Account suspended",
			Title:        "Action Required",
			Message:      "Your account was suspended at {{timestamp}}. Reason: {{reason}}.",
			Type:         "error",
			Icon:         "lock",
			TemplateVars: jsonList([]string{"user_name", "reason", "timestamp"}),
		},
		{
			EventKey:     "account.suspension.lifted",
			Name:         "Suspension Lifted",
			Enabled:      true,
			Heading:      "🔓 Suspension lifted",
			Title:        "Account Restored",
			Message:      "Your account is now active again as of {{timestamp}}.",
			Type:         "success",
			Icon:         "unlock",
			TemplateVars: jsonList([]string{"user_name", "timestamp"}),
		},
		{
			EventKey:     "match.created",
			Name:         "Match Created",
			Enabled:      true,
			Heading:      "🎮 New match: {{game_name}}",
			Title:        "Match Ready",
			Message:      "You're scheduled to play vs {{opponent_name}} at {{start_time}}.",
			Type:         "info",
			Icon:         "gamepad-2",
			TemplateVars: jsonList([]string{"user_name", "opponent_name", "game_name", "start_time", "match_id"}),
		},
		{
			EventKey:     "match.result",
			Name:         "Match Result",
			Enabled:      true,
			Heading:      "{{result}} in {{game_name}}!",
			Title:        "Match Completed",
			Message:      "You {{result}} vs {{opponent_name}}. XP: +{{xp_change}}.",
			Type:         "success",
			Icon:         "trophy",
			TemplateVars: jsonList([]string{"user_name", "opponent_name", "game_name", "result", "xp_change", "match_id", "timestamp"}),
		},
		{
			EventKey:     "quiz.completed",
			Name:         "Quiz Completed",
			Enabled:      true,
			Heading:      "🧠 Quiz completed: {{score}}/{{total}}",
			Title:        "Quiz Result",
			Message:      "You earned {{xp_earned}} XP and {{reward}}.",
			Type:         "success",
			Icon:         "clipboard-check",
			TemplateVars: jsonList([]string{"user_name", "score", "total", "xp_earned", "reward", "quiz_id", "timestamp"}),
		},
		{
			EventKey:     "post.liked",
			Name:         "Post Liked",
			Enabled:      true,
			Heading:      "❤️ Your post was liked!",
			Title:        "Social Engagement",
			Message:      "{{liker_name}} liked your post: '{{post_snippet}}...'",
			Type:         "info",
			Icon:         "heart",
			TemplateVars: jsonList([]string{"user_name", "liker_name", "post_snippet", "timestamp"}),
		},
		// --- NEW TEMPLATE FOR DYNAMIC PROFILE UPDATES ---
		{
			EventKey:     "profile.updated",
			Name:         "Profile Updated",
			Enabled:      true,
			Heading:      "✏️ Profile Updated",
			Title:        "Your Profile Changed",
			Message:      "{{message}}", // Use the dynamic message passed from the profile service
			Type:         "info",
			Icon:         "user",
			TemplateVars: jsonList([]string{"user_name", "timestamp", "message"}), // Include 'message' variable
		},
		// --- NEW TEMPLATE FOR EMAIL UPDATES ---
		{
			EventKey:     "profile.email.updated",
			Name:         "Email Updated",
			Enabled:      true,
			Heading:      "📧 Email Address Updated",
			Title:        "Your Email Changed",
			Message:      "{{message}}", // Use the dynamic message passed from the profile service
			Type:         "info",
			Icon:         "mail",
			TemplateVars: jsonList([]string{"user_name", "timestamp", "message"}), // Include 'message' variable
		},
		// --- NEW TEMPLATE FOR IMAGE UPDATES (Kept for specificity if needed later) ---
		{
			EventKey:     "profile.image.updated",
			Name:         "Profile Image Updated",
			Enabled:      true,
			Heading:      "🖼️ Profile Image Updated",
			Title:        "Your Photo Changed",
			Message:      "{{message}}", // Use the dynamic message passed from the profile service
			Type:         "info",
			Icon:         "image",
			TemplateVars: jsonList([]string{"user_name", "timestamp", "message"}), // Include 'message' variable
		},
		{
			EventKey:     "pin.set",
			Name:         "PIN Set",
			Enabled:      true,
			Heading:      "🔒 PIN Created",
			Title:        "Wallet Security Set",
			Message:      "Your wallet PIN was created on {{timestamp}} from device {{device_id}}.",
			Type:         "success",
			Icon:         "lock",
			TemplateVars: jsonList([]string{"user_name", "timestamp", "device_id"}),
		},
		{
			EventKey:     "pin.updated",
			Name:         "PIN Updated",
			Enabled:      true,
			Heading:      "🔐 PIN Changed",
			Title:        "Wallet PIN Updated",
			Message:      "Your wallet PIN was changed on {{timestamp}} from device {{device_id}}.",
			Type:         "info",
			Icon:         "key",
			TemplateVars: jsonList([]string{"user_name", "timestamp", "device_id"}),
		},
		{
			EventKey:     "pin.recovered",
			Name:         "PIN Recovered",
			Enabled:      true,
			Heading:      "🔓 PIN Recovered",
			Title:        "Wallet PIN Reset",
			Message:      "Your wallet PIN was reset on {{timestamp}} from device {{device_id}}.",
			Type:         "warning",
			Icon:         "key-round",
			TemplateVars: jsonList([]string{"user_name", "timestamp", "device_id"}),
		},
		// Add to templates slice in seedSystemNotificationTemplates():
		{
			EventKey:     "wallet.deposit.detected",
			Name:         "Deposit Detected",
			Enabled:      true,
			Heading:      "💰 Incoming deposit: {{amount}} {{currency}}",
			Title:        "Deposit Detected",
			Message:      "An external deposit of {{amount}} {{currency}} was detected and confirmed. New balance: {{new_balance}} {{currency}}. Transaction: {{txid}}.",
			Type:         "success",
			Icon:         "arrow-down-circle",
			TemplateVars: jsonList([]string{"user_name", "amount", "currency", "new_balance", "txid", "timestamp"}),
		},
		{
			EventKey:     "conversion.sol_to_fiat.completed",
			Name:         "SOL → Fiat Conversion Completed",
			Enabled:      true,
			Heading:      "💱 Converted {{sol_amount}} SOL → {{fiat_amount}} {{fiat_currency}}",
			Title:        "Conversion Successful",
			Message:      "Your SOL-to-fiat conversion is complete. {{fiat_amount}} {{fiat_currency}} has been added to your balance. Fee: {{fee_amount}} SOL.",
			Type:         "success",
			Icon:         "repeat",
			TemplateVars: jsonList([]string{"user_name", "sol_amount", "fiat_amount", "fiat_currency", "fee_amount", "txid", "timestamp"}),
		},
		{
			EventKey:     "conversion.fiat_to_sol.completed",
			Name:         "Fiat → SOL Conversion Completed",
			Enabled:      true,
			Heading:      "💱 Converted {{fiat_amount}} {{fiat_currency}} → {{sol_amount}} SOL",
			Title:        "Conversion Successful",
			Message:      "Your fiat-to-SOL conversion is complete. {{sol_amount}} SOL has been deposited to your wallet. Fee: {{fee_amount}} {{fiat_currency}}.",
			Type:         "success",
			Icon:         "repeat",
			TemplateVars: jsonList([]string{"user_name", "fiat_amount", "fiat_currency", "sol_amount", "fee_amount", "txid", "timestamp"}),
		},
	}

	for _, t := range templates {
		var count int64
		db.Model(&models.SystemNotificationTemplate{}).
			Where("event_key = ?", t.EventKey).
			Count(&count)

		if count == 0 {
			if err := db.Create(&t).Error; err != nil {
				return fmt.Errorf("failed to seed template %s: %w", t.EventKey, err)
			}
			log.Printf("✅ Seeded system template: %s", t.EventKey)
		}
	}
	return nil
}
