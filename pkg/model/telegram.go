package model

import "time"

// TelegramBot represents a Telegram bot details
type TelegramBot struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// TelegramConfig represents a user's Telegram notification configuration
type TelegramConfig struct {
	ID             int       `json:"id" db:"id"`
	UserID         int       `json:"user_id" db:"user_id"`
	ChatID         string    `json:"chat_id" db:"chat_id"`
	ChatName       string    `json:"chat_name" db:"chat_name"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	NotifyOnDown   bool      `json:"notify_on_down" db:"notify_on_down"`
	NotifyOnUp     bool      `json:"notify_on_up" db:"notify_on_up"`
	MonitorRegions []string  `json:"monitor_regions"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// TelegramConfigRequest represents a request to add/update Telegram configuration
type TelegramConfigRequest struct {
	ChatID         string   `json:"chat_id" binding:"required"`
	ChatName       string   `json:"chat_name"`
	Language       string   `json:"language"` // Add this field
	NotifyOnDown   bool     `json:"notify_on_down"`
	NotifyOnUp     bool     `json:"notify_on_up"`
	IsActive       bool     `json:"active"`
	MonitorRegions []string `json:"monitor_regions"`
}

// TelegramPrompt represents a localized message template
type TelegramPrompt struct {
	ID          int       `json:"id" db:"id"`
	Key         string    `json:"key" db:"key"`
	Language    string    `json:"language" db:"language"`
	Message     string    `json:"message" db:"message"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// TelegramPromptRequest for creating/updating prompts
type TelegramPromptRequest struct {
	Key         string `json:"key" binding:"required"`
	Language    string `json:"language" binding:"required"`
	Message     string `json:"message" binding:"required"`
	Description string `json:"description"`
}

// TelegramPromptResponse for paginated results
type TelegramPromptResponse struct {
	Prompts    []TelegramPrompt `json:"prompts"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	PerPage    int              `json:"per_page"`
	TotalPages int              `json:"total_pages"`
}
