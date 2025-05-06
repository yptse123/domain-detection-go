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
	ID        int       `json:"id" db:"id"`
	UserID    int       `json:"user_id" db:"user_id"`
	ChatID    string    `json:"chat_id" db:"chat_id"`
	ChatName  string    `json:"chat_name" db:"chat_name"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// TelegramConfigRequest represents a request to add/update Telegram configuration
type TelegramConfigRequest struct {
	ChatID       string `json:"chat_id"`
	ChatName     string `json:"chat_name"`
	NotifyOnDown bool   `json:"notify_on_down"`
	NotifyOnUp   bool   `json:"notify_on_up"`
	IsActive     bool   `json:"active"` // Note: maps from 'active' in JSON to IsActive in Go
}
