package notification

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"sync"
	"time"

	"domain-detection-go/internal/service"
	"domain-detection-go/pkg/model"

	"github.com/jmoiron/sqlx"
)

// EmailConfig holds the configuration for email service
type EmailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
}

// EmailService manages email notifications
type EmailService struct {
	config        EmailConfig
	db            *sqlx.DB
	promptService *service.TelegramPromptService
	notifyLock    sync.Mutex
	notifyCache   map[string]time.Time
}

// NewEmailService creates a new email service
func NewEmailService(config EmailConfig, db *sqlx.DB, promptService *service.TelegramPromptService) *EmailService {
	return &EmailService{
		config:        config,
		db:            db,
		promptService: promptService,
		notifyCache:   make(map[string]time.Time),
	}
}

// AddEmailConfig adds a new email notification configuration
func (s *EmailService) AddEmailConfig(
	userID int,
	emailAddress,
	emailName string,
	language string,
	notifyOnDown,
	notifyOnUp bool,
	isActive bool,
	monitorRegions []string,
) (int, error) {
	var configID int

	if language == "" {
		language = "en"
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	err = tx.QueryRow(`
        INSERT INTO email_configs
        (user_id, email_address, email_name, language, notify_on_down, notify_on_up, is_active, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
        RETURNING id
    `, userID, emailAddress, emailName, language, notifyOnDown, notifyOnUp, isActive).Scan(&configID)

	if err != nil {
		return 0, fmt.Errorf("failed to add email configuration: %w", err)
	}

	// Add regions if specified
	if len(monitorRegions) > 0 {
		stmt, err := tx.Prepare(`
            INSERT INTO email_config_regions (email_config_id, region_code)
            VALUES ($1, $2)
        `)
		if err != nil {
			return 0, fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, region := range monitorRegions {
			var exists bool
			err = tx.Get(&exists, "SELECT EXISTS(SELECT 1 FROM regions WHERE code = $1)", region)
			if err != nil {
				return 0, fmt.Errorf("failed to verify region %s: %w", region, err)
			}
			if !exists {
				return 0, fmt.Errorf("region code not found: %s", region)
			}

			_, err = stmt.Exec(configID, region)
			if err != nil {
				return 0, fmt.Errorf("failed to add region %s: %w", region, err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return configID, nil
}

// GetEmailConfigsForUser retrieves all email configurations for a user
func (s *EmailService) GetEmailConfigsForUser(userID int) ([]model.EmailConfig, error) {
	var configs []model.EmailConfig

	err := s.db.Select(&configs, `
        SELECT id, user_id, email_address, email_name, language, is_active, notify_on_down, notify_on_up, created_at, updated_at
        FROM email_configs
        WHERE user_id = $1
        ORDER BY created_at DESC
    `, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to get email configurations: %w", err)
	}

	// Get regions for each config
	for i := range configs {
		var regions []string
		err := s.db.Select(&regions, `
            SELECT region_code
            FROM email_config_regions
            WHERE email_config_id = $1
        `, configs[i].ID)

		if err != nil {
			return nil, fmt.Errorf("failed to get regions for config %d: %w", configs[i].ID, err)
		}

		configs[i].MonitorRegions = regions
	}

	return configs, nil
}

// UpdateEmailConfig updates an email configuration
func (s *EmailService) UpdateEmailConfig(
	configID,
	userID int,
	emailAddress,
	emailName string,
	language string,
	notifyOnDown,
	notifyOnUp bool,
	isActive bool,
	monitorRegions []string,
) error {
	if language == "" {
		language = "en"
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	_, err = tx.Exec(`
        UPDATE email_configs
        SET email_address = $1,
            email_name = $2,
            language = $3,
            notify_on_down = $4,
            notify_on_up = $5,
            is_active = $6,
            updated_at = NOW()
        WHERE id = $7 AND user_id = $8
    `, emailAddress, emailName, language, notifyOnDown, notifyOnUp, isActive, configID, userID)

	if err != nil {
		return fmt.Errorf("failed to update email configuration: %w", err)
	}

	// Delete existing regions
	_, err = tx.Exec(`DELETE FROM email_config_regions WHERE email_config_id = $1`, configID)
	if err != nil {
		return fmt.Errorf("failed to clear existing regions: %w", err)
	}

	// Add new regions if specified
	if len(monitorRegions) > 0 {
		stmt, err := tx.Prepare(`
            INSERT INTO email_config_regions (email_config_id, region_code)
            VALUES ($1, $2)
        `)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, region := range monitorRegions {
			var exists bool
			err = tx.Get(&exists, "SELECT EXISTS(SELECT 1 FROM regions WHERE code = $1)", region)
			if err != nil {
				return fmt.Errorf("failed to verify region %s: %w", region, err)
			}
			if !exists {
				return fmt.Errorf("region code not found: %s", region)
			}

			_, err = stmt.Exec(configID, region)
			if err != nil {
				return fmt.Errorf("failed to add region %s: %w", region, err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteEmailConfig deletes an email configuration
func (s *EmailService) DeleteEmailConfig(configID, userID int) error {
	_, err := s.db.Exec(`
        DELETE FROM email_configs
        WHERE id = $1 AND user_id = $2
    `, configID, userID)

	if err != nil {
		return fmt.Errorf("failed to delete email configuration: %w", err)
	}

	return nil
}

// SendDomainStatusNotification sends email notification about domain status change
func (s *EmailService) SendDomainStatusNotification(domain model.Domain, statusChanged bool) error {
	var configs []struct {
		ID             int      `db:"id"`
		EmailAddress   string   `db:"email_address"`
		EmailName      string   `db:"email_name"`
		Language       string   `db:"language"`
		IsActive       bool     `db:"is_active"`
		NotifyOnUp     bool     `db:"notify_on_up"`
		NotifyOnDown   bool     `db:"notify_on_down"`
		MonitorRegions []string `db:"monitor_regions"`
	}

	err := s.db.Select(&configs, `
        SELECT ec.id, ec.email_address, ec.email_name, ec.language, ec.is_active, ec.notify_on_up, ec.notify_on_down
        FROM email_configs ec
        WHERE ec.user_id = $1
    `, domain.UserID)

	if err != nil {
		log.Printf("Failed to get email configurations for user %d: %v", domain.UserID, err)
		return fmt.Errorf("failed to get email configurations for user: %w", err)
	}

	// Get regions for each config
	for i := range configs {
		var regions []string
		err := s.db.Select(&regions, `
            SELECT region_code
            FROM email_config_regions
            WHERE email_config_id = $1
        `, configs[i].ID)

		if err != nil {
			log.Printf("Failed to get regions for config %d: %v", configs[i].ID, err)
			continue
		}

		configs[i].MonitorRegions = regions
	}

	if len(configs) == 0 {
		log.Printf("No email configurations for user %d", domain.UserID)
		return nil
	}

	// Determine notification type
	notificationType := "status"
	if !domain.Available() {
		notificationType = "down"
	} else if statusChanged {
		notificationType = "up"
	}

	// Check suppression
	s.notifyLock.Lock()
	defer s.notifyLock.Unlock()

	suppressionDuration := time.Duration(domain.Interval) * time.Minute
	if !domain.Available() || statusChanged {
		suppressionDuration = suppressionDuration / 2
	}

	minSuppression := 2 * time.Minute
	if suppressionDuration < minSuppression {
		suppressionDuration = minSuppression
	}

	cacheKey := fmt.Sprintf("%d:%s", domain.ID, notificationType)
	now := time.Now()
	if lastSent, exists := s.notifyCache[cacheKey]; exists {
		timeSinceLast := now.Sub(lastSent)
		if timeSinceLast < suppressionDuration {
			log.Printf("Skipping email notification for domain %s (%s): last sent %s ago, suppression duration: %s",
				domain.Name, notificationType, timeSinceLast, suppressionDuration)
			return nil
		}
	}

	// Format time
	loc, err := time.LoadLocation(TIMEZONE_LOCATION)
	if err != nil {
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	formattedTime := domain.LastCheck.In(loc).Format("2006-01-02 15:04:05")

	// Send to all configured emails
	for _, config := range configs {
		if !config.IsActive {
			log.Printf("Skipping email notification for domain %s to %s: Email config is inactive",
				domain.Name, config.EmailAddress)
			continue
		}

		// Check region filtering
		if len(config.MonitorRegions) > 0 {
			regionMatches := false
			for _, region := range config.MonitorRegions {
				if region == domain.Region {
					regionMatches = true
					break
				}
			}

			if !regionMatches {
				log.Printf("Skipping email notification for domain %s to %s: Domain region %s not in monitor regions %v",
					domain.Name, config.EmailAddress, domain.Region, config.MonitorRegions)
				continue
			}
		}

		// Check notification preferences
		if notificationType == "up" && !config.NotifyOnUp {
			log.Printf("Skipping 'back up' email notification for domain %s to %s: notify_on_up is disabled",
				domain.Name, config.EmailAddress)
			continue
		}

		if notificationType == "down" && !config.NotifyOnDown {
			log.Printf("Skipping 'down' email notification for domain %s to %s: notify_on_down is disabled",
				domain.Name, config.EmailAddress)
			continue
		}

		// Check notification history
		var lastNotification time.Time
		err := s.db.Get(&lastNotification, `
            SELECT MAX(notified_at) 
            FROM notification_history
            WHERE domain_id = $1 AND email_config_id = $2 AND notification_type = $3
        `, domain.ID, config.ID, notificationType)

		if err == nil && !lastNotification.IsZero() {
			if now.Sub(lastNotification) < suppressionDuration {
				log.Printf("Skipping email notification to %s for domain %s: last sent at %s (suppression: %s)",
					config.EmailAddress, domain.Name, lastNotification, suppressionDuration)
				continue
			}
		}

		// Send email
		subject, body := s.formatEmailMessage(notificationType, config.Language, domain, formattedTime)

		if err := s.sendEmail(config.EmailAddress, config.EmailName, subject, body); err != nil {
			log.Printf("Failed to send email notification to %s: %v", config.EmailAddress, err)
			continue
		}

		// Record notification in database
		_, err = s.db.Exec(`
            INSERT INTO notification_history
            (domain_id, email_config_id, status_code, error_code, error_description, notified_at, notification_type)
            VALUES ($1, $2, $3, $4, $5, NOW(), $6)
        `, domain.ID, config.ID, domain.LastStatus, domain.ErrorCode, domain.ErrorDescription, notificationType)

		if err != nil {
			log.Printf("Failed to record email notification history: %v", err)
		}

		s.notifyCache[cacheKey] = now
	}

	return nil
}

// formatEmailMessage formats the email subject and body
func (s *EmailService) formatEmailMessage(notificationType, language string, domain model.Domain, formattedTime string) (string, string) {
	var subject, bodyTemplate string

	switch notificationType {
	case "down":
		subject = fmt.Sprintf("🔴 Domain %s is DOWN", domain.Name)
		bodyTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Domain Down Alert</title>
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
    <div style="max-width: 600px; margin: 0 auto; padding: 20px;">
        <h2 style="color: #e74c3c;">🔴 Domain Alert</h2>
        <p><strong>Domain {{.Domain}} is currently DOWN</strong></p>
        <div style="background-color: #f8f9fa; padding: 15px; border-radius: 5px; margin: 20px 0;">
            <p><strong>Status Code:</strong> {{.Status}}</p>
            <p><strong>Error:</strong> {{.Error}}</p>
            <p><strong>Response Time:</strong> {{.ResponseTime}}ms</p>
            <p><strong>Last Check:</strong> {{.LastCheck}} (UTC+8)</p>
        </div>
        <p style="color: #666; font-size: 12px;">This is an automated message from your Domain Monitoring Service.</p>
    </div>
</body>
</html>`
	case "up":
		subject = fmt.Sprintf("🟢 Domain %s is back UP", domain.Name)
		bodyTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Domain Recovery Alert</title>
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
    <div style="max-width: 600px; margin: 0 auto; padding: 20px;">
        <h2 style="color: #27ae60;">🟢 Domain Recovery</h2>
        <p><strong>Domain {{.Domain}} is back online!</strong></p>
        <div style="background-color: #f8f9fa; padding: 15px; border-radius: 5px; margin: 20px 0;">
            <p><strong>Status Code:</strong> {{.Status}}</p>
            <p><strong>Response Time:</strong> {{.ResponseTime}}ms</p>
            <p><strong>Last Check:</strong> {{.LastCheck}} (UTC+8)</p>
        </div>
        <p style="color: #666; font-size: 12px;">This is an automated message from your Domain Monitoring Service.</p>
    </div>
</body>
</html>`
	default:
		subject = fmt.Sprintf("📊 Domain %s status update", domain.Name)
		bodyTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Domain Status Update</title>
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
    <div style="max-width: 600px; margin: 0 auto; padding: 20px;">
        <h2 style="color: #3498db;">📊 Domain Status Update</h2>
        <p><strong>Domain {{.Domain}} status update</strong></p>
        <div style="background-color: #f8f9fa; padding: 15px; border-radius: 5px; margin: 20px 0;">
            <p><strong>Status Code:</strong> {{.Status}}</p>
            <p><strong>Response Time:</strong> {{.ResponseTime}}ms</p>
            <p><strong>Last Check:</strong> {{.LastCheck}} (UTC+8)</p>
        </div>
        <p style="color: #666; font-size: 12px;">This is an automated message from your Domain Monitoring Service.</p>
    </div>
</body>
</html>`
	}

	// Execute template
	tmpl, err := template.New("email").Parse(bodyTemplate)
	if err != nil {
		log.Printf("Error parsing email template: %v", err)
		return subject, "Error generating email content"
	}

	data := struct {
		Domain       string
		Status       int
		Error        string
		ResponseTime int
		LastCheck    string
	}{
		Domain:       domain.Name,
		Status:       domain.LastStatus,
		Error:        domain.ErrorDescription,
		ResponseTime: domain.TotalTime,
		LastCheck:    formattedTime,
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		log.Printf("Error executing email template: %v", err)
		return subject, "Error generating email content"
	}

	return subject, body.String()
}

// sendEmail sends an email using SMTP
func (s *EmailService) sendEmail(toEmail, toName, subject, body string) error {
	from := s.config.FromEmail
	to := []string{toEmail}

	// Create message with proper headers
	msg := []byte("From: " + from + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	// SMTP authentication
	auth := smtp.PlainAuth("", s.config.SMTPUsername, s.config.SMTPPassword, s.config.SMTPHost)

	// Send email using smtp.SendMail (handles STARTTLS automatically)
	serverAddr := s.config.SMTPHost + ":" + s.config.SMTPPort
	err := smtp.SendMail(serverAddr, auth, from, to, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Email sent successfully to %s", toEmail)
	return nil
}

// SendTestEmail sends a test email
func (s *EmailService) SendTestEmail(config model.EmailConfig) error {
	if !config.IsActive {
		return fmt.Errorf("email configuration is not active")
	}

	subject := "🧪 Test Email from Domain Monitor"
	// Format time in UTC+8
	loc, err := time.LoadLocation(TIMEZONE_LOCATION)
	if err != nil {
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	formattedTime := time.Now().In(loc).Format("2006-01-02 15:04:05")

	body := `
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>Test Email</title>
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
	<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
		<h2 style="color: #3498db;">🧪 Test Email</h2>
		<p>This is a test email from your Domain Monitoring Service.</p>
		<p>If you're receiving this email, your email notifications are configured correctly.</p>
		<div style="background-color: #f8f9fa; padding: 15px; border-radius: 5px; margin: 20px 0;">
			<p><strong>Configuration:</strong> ` + config.EmailName + `</p>
			<p><strong>Email:</strong> ` + config.EmailAddress + `</p>
			<p><strong>Language:</strong> ` + config.Language + `</p>
		</div>
		<p style="color: #666; font-size: 12px;">Sent at: ` + formattedTime + ` (UTC+8)</p>
	</div>
</body>
</html>`

	return s.sendEmail(config.EmailAddress, config.EmailName, subject, body)
}
