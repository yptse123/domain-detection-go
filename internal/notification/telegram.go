package notification

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"domain-detection-go/pkg/model"

	"github.com/jmoiron/sqlx"
)

// Add this constant at the top of your file
const TIMEZONE_LOCATION = "Asia/Hong_Kong" // UTC+8

// TelegramConfig holds the configuration for Telegram API
type TelegramConfig struct {
	APIToken string
	BaseURL  string
}

// TelegramService manages interactions with the Telegram Bot API
type TelegramService struct {
	config      TelegramConfig
	db          *sqlx.DB
	httpClient  *http.Client
	rateLimiter <-chan time.Time
	notifyLock  sync.Mutex
	notifyCache map[string]time.Time // Cache to track recent notifications
	cacheTTL    time.Duration        // How long to suppress duplicate notifications
}

// NewTelegramService creates a new telegram service
func NewTelegramService(config TelegramConfig, db *sqlx.DB) *TelegramService {
	// Set defaults if not provided
	if config.BaseURL == "" {
		config.BaseURL = "https://api.telegram.org/bot"
	}

	return &TelegramService{
		config:      config,
		db:          db,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rateLimiter: time.Tick(500 * time.Millisecond), // Max 2 API calls per second
		notifyCache: make(map[string]time.Time),
		// cacheTTL:    1 * time.Hour, // Default: suppress same notifications for 1 hour
	}
}

// SetupBot initializes the bot and returns its details
func (s *TelegramService) SetupBot() (model.TelegramBot, error) {
	<-s.rateLimiter // Rate limiting

	url := fmt.Sprintf("%s%s/getMe", s.config.BaseURL, s.config.APIToken)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return model.TelegramBot{}, fmt.Errorf("failed to connect to Telegram API: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return model.TelegramBot{}, fmt.Errorf("failed to read API response: %w", err)
	}

	var response struct {
		OK     bool `json:"ok"`
		Result struct {
			ID       int64  `json:"id"`
			IsBot    bool   `json:"is_bot"`
			Username string `json:"username"`
			Name     string `json:"first_name"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return model.TelegramBot{}, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !response.OK {
		return model.TelegramBot{}, fmt.Errorf("Telegram API error: %s", string(body))
	}

	return model.TelegramBot{
		ID:       response.Result.ID,
		Username: response.Result.Username,
		Name:     response.Result.Name,
	}, nil
}

// JoinGroup attempts to join a Telegram group via invite link
// Note: This cannot be done purely via API; the user must click the invitation link
func (s *TelegramService) JoinGroup(inviteLink string) (string, error) {
	// Extract the chat ID from the invite link if possible
	// This is a simplification - Telegram doesn't directly allow bots to join via API
	parts := strings.Split(inviteLink, "/")
	inviteCode := parts[len(parts)-1]

	log.Printf("Bot must be manually added to group with invite code: %s", inviteCode)
	log.Printf("Please provide instructions to the user on how to add the bot manually")

	return inviteCode, nil
}

// AddTelegramConfig adds a new Telegram notification configuration for a user
func (s *TelegramService) AddTelegramConfig(
	userID int,
	chatID,
	chatName string,
	notifyOnDown,
	notifyOnUp bool,
	isActive bool,
	monitorRegions []string,
) (int, error) {
	var configID int

	// Start a transaction
	tx, err := s.db.Beginx()
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}

	// Rollback transaction if any error occurs
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Insert the base config
	err = tx.QueryRow(`
        INSERT INTO telegram_configs
        (user_id, chat_id, chat_name, notify_on_down, notify_on_up, is_active, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
        RETURNING id
    `, userID, chatID, chatName, notifyOnDown, notifyOnUp, isActive).Scan(&configID)

	if err != nil {
		return 0, fmt.Errorf("failed to add Telegram configuration: %w", err)
	}

	// Add regions if specified
	if len(monitorRegions) > 0 {
		stmt, err := tx.Prepare(`
            INSERT INTO telegram_config_regions (telegram_config_id, region_code)
            VALUES ($1, $2)
        `)
		if err != nil {
			return 0, fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, region := range monitorRegions {
			// Verify region exists
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

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return configID, nil
}

// GetTelegramConfigsForUser retrieves all Telegram configurations for a user
func (s *TelegramService) GetTelegramConfigsForUser(userID int) ([]model.TelegramConfig, error) {
	var configs []model.TelegramConfig

	// Query base configurations
	err := s.db.Select(&configs, `
        SELECT id, user_id, chat_id, chat_name, is_active, notify_on_down, notify_on_up, created_at, updated_at
        FROM telegram_configs
        WHERE user_id = $1
        ORDER BY created_at DESC
    `, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to get Telegram configurations: %w", err)
	}

	// Get regions for each config
	for i := range configs {
		var regions []string
		err := s.db.Select(&regions, `
            SELECT region_code
            FROM telegram_config_regions
            WHERE telegram_config_id = $1
        `, configs[i].ID)

		if err != nil {
			return nil, fmt.Errorf("failed to get regions for config %d: %w", configs[i].ID, err)
		}

		configs[i].MonitorRegions = regions
	}

	return configs, nil
}

// UpdateTelegramConfig updates a Telegram configuration
func (s *TelegramService) UpdateTelegramConfig(
	configID,
	userID int,
	chatID,
	chatName string,
	notifyOnDown,
	notifyOnUp bool,
	isActive bool,
	monitorRegions []string,
) error {
	// Start a transaction
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Rollback transaction if any error occurs
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Update the config
	_, err = tx.Exec(`
        UPDATE telegram_configs
        SET chat_id = $1,
            chat_name = $2,
            notify_on_down = $3,
            notify_on_up = $4,
            is_active = $5,
            updated_at = NOW()
        WHERE id = $6 AND user_id = $7
    `, chatID, chatName, notifyOnDown, notifyOnUp, isActive, configID, userID)

	if err != nil {
		return fmt.Errorf("failed to update Telegram configuration: %w", err)
	}

	// Delete existing regions
	_, err = tx.Exec(`DELETE FROM telegram_config_regions WHERE telegram_config_id = $1`, configID)
	if err != nil {
		return fmt.Errorf("failed to clear existing regions: %w", err)
	}

	// Add new regions if specified
	if len(monitorRegions) > 0 {
		stmt, err := tx.Prepare(`
            INSERT INTO telegram_config_regions (telegram_config_id, region_code)
            VALUES ($1, $2)
        `)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, region := range monitorRegions {
			// Verify region exists
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

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteTelegramConfig deletes a Telegram configuration
func (s *TelegramService) DeleteTelegramConfig(configID, userID int) error {
	_, err := s.db.Exec(`
        DELETE FROM telegram_configs
        WHERE id = $1 AND user_id = $2
    `, configID, userID)

	if err != nil {
		return fmt.Errorf("failed to delete Telegram configuration: %w", err)
	}

	return nil
}

// SendDomainStatusNotification sends a notification about domain status change
func (s *TelegramService) SendDomainStatusNotification(domain model.Domain, statusChanged bool) error {
	// Get all active telegram configs for this domain's user
	var configs []struct {
		ID             int      `db:"id"`
		ChatID         string   `db:"chat_id"`
		ChatName       string   `db:"chat_name"`
		IsActive       bool     `db:"is_active"`
		NotifyOnUp     bool     `db:"notify_on_up"`
		NotifyOnDown   bool     `db:"notify_on_down"`
		MonitorRegions []string `db:"monitor_regions"`
	}

	// First get basic config info
	err := s.db.Select(&configs, `
        SELECT tc.id, tc.chat_id, tc.chat_name, tc.is_active, tc.notify_on_up, tc.notify_on_down
        FROM telegram_configs tc
        WHERE tc.user_id = $1
    `, domain.UserID)

	if err != nil {
		log.Printf("Failed to get Telegram configurations for user %d: %v", domain.UserID, err)
		return fmt.Errorf("failed to get Telegram configurations for user: %w", err)
	}

	// Then get the regions for each config
	for i := range configs {
		var regions []string
		err := s.db.Select(&regions, `
            SELECT region_code
            FROM telegram_config_regions
            WHERE telegram_config_id = $1
        `, configs[i].ID)

		if err != nil {
			log.Printf("Failed to get regions for config %d: %v", configs[i].ID, err)
			continue
		}

		configs[i].MonitorRegions = regions
	}

	if len(configs) == 0 {
		log.Printf("No Telegram configurations for user %d", domain.UserID)
		return nil
	}

	// Determine notification type
	notificationType := "status"
	if !domain.Available() {
		notificationType = "down"
	} else if statusChanged {
		notificationType = "up"
	}

	// Check if we should send notification based on history and rate limiting
	s.notifyLock.Lock()
	defer s.notifyLock.Unlock()

	// Calculate suppression duration based on domain's interval
	suppressionDuration := time.Duration(domain.Interval) * time.Minute

	// For UP/DOWN status changes, use a shorter suppression period (half of regular interval)
	if !domain.Available() || statusChanged {
		suppressionDuration = suppressionDuration / 2
	}

	// Set a minimum suppression time to avoid flooding
	minSuppression := 2 * time.Minute
	if suppressionDuration < minSuppression {
		suppressionDuration = minSuppression
	}

	// Check if we've recently sent the same notification
	cacheKey := fmt.Sprintf("%d:%s", domain.ID, notificationType)
	now := time.Now()
	if lastSent, exists := s.notifyCache[cacheKey]; exists {
		timeSinceLast := now.Sub(lastSent)
		if timeSinceLast < suppressionDuration {
			log.Printf("Skipping notification for domain %s (%s): last sent %s ago, suppression duration: %s",
				domain.Name, notificationType, timeSinceLast, suppressionDuration)
			return nil
		}
	}

	// Create a location object for UTC+8 (Asia/Hong_Kong)
	loc, err := time.LoadLocation(TIMEZONE_LOCATION)
	if err != nil {
		// Fallback to UTC+8 fixed offset if location name isn't available
		loc = time.FixedZone("UTC+8", 8*60*60)
	}

	// Format time in UTC+8
	formattedTime := domain.LastCheck.In(loc).Format("2006-01-02 15:04:05")

	// Format message based on domain status
	var message string
	if !domain.Available() {
		message = formatDownMessage(domain, formattedTime)
	} else if statusChanged {
		message = formatUpMessage(domain, formattedTime)
	} else {
		message = formatStatusUpdateMessage(domain, formattedTime)
	}

	// Send to all configured chats that match notification preferences
	for _, config := range configs {
		// Skip if telegram config is not active
		if !config.IsActive {
			log.Printf("Skipping notification for domain %s to chat %s: Telegram config is inactive",
				domain.Name, config.ChatName)
			continue
		}

		// Skip if region doesn't match (if regions are specified)
		if len(config.MonitorRegions) > 0 {
			regionMatches := false
			for _, region := range config.MonitorRegions {
				if region == domain.Region {
					regionMatches = true
					break
				}
			}

			if !regionMatches {
				log.Printf("Skipping notification for domain %s to chat %s: Domain region %s not in monitor regions %v",
					domain.Name, config.ChatName, domain.Region, config.MonitorRegions)
				continue
			}
		}

		// Skip "up" notifications if notify_on_up is disabled
		if notificationType == "up" && !config.NotifyOnUp {
			log.Printf("Skipping 'back up' notification for domain %s to chat %s: notify_on_up is disabled",
				domain.Name, config.ChatName)
			continue
		}

		// Skip "down" notifications if notify_on_down is disabled
		if notificationType == "down" && !config.NotifyOnDown {
			log.Printf("Skipping 'down' notification for domain %s to chat %s: notify_on_down is disabled",
				domain.Name, config.ChatName)
			continue
		}

		// Check notification history in database
		var lastNotification time.Time
		err := s.db.Get(&lastNotification, `
            SELECT MAX(notified_at) 
            FROM notification_history
            WHERE domain_id = $1 AND telegram_config_id = $2 AND notification_type = $3
        `, domain.ID, config.ID, notificationType)

		if err == nil && !lastNotification.IsZero() {
			// Skip if we've notified this chat about this domain recently
			if now.Sub(lastNotification) < suppressionDuration {
				log.Printf("Skipping notification to chat %s for domain %s: last sent at %s (suppression: %s)",
					config.ChatName, domain.Name, lastNotification, suppressionDuration)
				continue
			}
		}

		// Send message to this chat
		if err := s.sendTelegramMessage(config.ChatID, message); err != nil {
			log.Printf("Failed to send Telegram notification to chat %s: %v", config.ChatName, err)
			continue
		}

		// Record notification in database
		_, err = s.db.Exec(`
            INSERT INTO notification_history
            (domain_id, telegram_config_id, status_code, error_code, error_description, notified_at, notification_type)
            VALUES ($1, $2, $3, $4, $5, NOW(), $6)
        `, domain.ID, config.ID, domain.LastStatus, domain.ErrorCode, domain.ErrorDescription, notificationType)

		if err != nil {
			log.Printf("Failed to record notification history: %v", err)
		}

		// Update cache with current timestamp
		s.notifyCache[cacheKey] = now
	}

	return nil
}

// Helper functions to format messages
func formatDownMessage(domain model.Domain, formattedTime string) string {
	return fmt.Sprintf("🔴 Domain %s is DOWN\n\nStatus: %d\nError: %s\nResponse Time: %dms\nLast Check: %s (UTC+8)",
		domain.Name, domain.LastStatus, domain.ErrorDescription, domain.TotalTime,
		formattedTime)
}

func formatUpMessage(domain model.Domain, formattedTime string) string {
	return fmt.Sprintf("🟢 Domain %s is back UP\n\nStatus: %d\nResponse Time: %dms\nLast Check: %s (UTC+8)",
		domain.Name, domain.LastStatus, domain.TotalTime,
		formattedTime)
}

func formatStatusUpdateMessage(domain model.Domain, formattedTime string) string {
	statusEmoji := "🟢"
	if domain.TotalTime > 2000 {
		statusEmoji = "🟠" // Slow response
	}
	return fmt.Sprintf("%s Domain %s status update\n\nStatus: %d\nResponse Time: %dms\nLast Check: %s (UTC+8)",
		statusEmoji, domain.Name, domain.LastStatus, domain.TotalTime,
		formattedTime)
}

// sendTelegramMessage sends a text message to a specific Telegram chat
func (s *TelegramService) sendTelegramMessage(chatID, message string) error {
	<-s.rateLimiter // Rate limiting

	url := fmt.Sprintf("%s%s/sendMessage", s.config.BaseURL, s.config.APIToken)

	// Prepare request body
	requestBody := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := s.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)

		// Check for group migration error
		if resp.StatusCode == 400 {
			var errorResponse struct {
				OK          bool   `json:"ok"`
				ErrorCode   int    `json:"error_code"`
				Description string `json:"description"`
				Parameters  struct {
					MigrateToChatID int64 `json:"migrate_to_chat_id"`
				} `json:"parameters"`
			}

			if err := json.Unmarshal(body, &errorResponse); err == nil {
				if strings.Contains(errorResponse.Description, "upgraded to a supergroup") &&
					errorResponse.Parameters.MigrateToChatID != 0 {

					// Extract new chat ID
					newChatID := fmt.Sprintf("%d", errorResponse.Parameters.MigrateToChatID)
					log.Printf("Group migrated to supergroup. Old ID: %s, New ID: %s", chatID, newChatID)

					// Update the chat ID in database
					err := s.updateChatID(chatID, newChatID)
					if err != nil {
						log.Printf("Failed to update chat ID in database: %v", err)
					}

					// Try again with the new chat ID
					return s.sendTelegramMessage(newChatID, message)
				}
			}
		}

		return fmt.Errorf("Telegram API error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// Add this new method to update the chat ID in database
func (s *TelegramService) updateChatID(oldChatID, newChatID string) error {
	_, err := s.db.Exec(`
        UPDATE telegram_configs
        SET chat_id = $1, updated_at = NOW()
        WHERE chat_id = $2
    `, newChatID, oldChatID)

	if err != nil {
		return fmt.Errorf("failed to update chat ID: %w", err)
	}

	log.Printf("Successfully updated chat ID from %s to %s in database", oldChatID, newChatID)
	return nil
}

// SendTelegramMessageToConfig sends a message to a specific telegram configuration
func (s *TelegramService) SendTelegramMessageToConfig(config model.TelegramConfig, message string) error {
	// Check if the configuration is active
	if !config.IsActive {
		return errors.New("telegram configuration is not active")
	}

	// If you want to include a timestamp in test messages:
	if strings.Contains(message, "Test Message") {
		loc, err := time.LoadLocation(TIMEZONE_LOCATION)
		if err != nil {
			loc = time.FixedZone("UTC+8", 8*60*60)
		}

		now := time.Now().In(loc)
		message += fmt.Sprintf("\n\nSent at: %s (UTC+8)", now.Format("2006-01-02 15:04:05"))
	}

	// Send the message
	err := s.sendTelegramMessage(config.ChatID, message)
	if err != nil {
		return err
	}

	// Record notification in history
	now := time.Now()
	_, err = s.db.Exec(`
        INSERT INTO notification_history 
        (domain_id, telegram_config_id, notification_type, notified_at) 
        VALUES ($1, $2, $3, $4)
    `, 0, config.ID, "test", now)

	if err != nil {
		log.Printf("Failed to record test notification in history: %v", err)
		// Continue despite error in recording history
	}

	return nil
}
