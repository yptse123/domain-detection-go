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
	ID          int               `json:"id" db:"id"`
	PromptKey   string            `json:"prompt_key" db:"prompt_key"`
	Description string            `json:"description" db:"description"`
	Messages    map[string]string `json:"messages" db:"messages"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
}

// TelegramPromptRequest for creating/updating prompts
type TelegramPromptRequest struct {
	PromptKey   string `json:"prompt_key" binding:"required"`
	Description string `json:"description"`
	En          string `json:"en"`
	Zh          string `json:"zh"`
	Hi          string `json:"hi"`
	Id          string `json:"id"`
	Vi          string `json:"vi"`
	Ko          string `json:"ko"`
	Ja          string `json:"ja"`
	Th          string `json:"th"`
}

// ToMessages converts individual language fields to messages map
func (r *TelegramPromptRequest) ToMessages() map[string]string {
	messages := make(map[string]string)
	if r.En != "" {
		messages["en"] = r.En
	}
	if r.Zh != "" {
		messages["zh"] = r.Zh
	}
	if r.Hi != "" {
		messages["hi"] = r.Hi
	}
	if r.Id != "" {
		messages["id"] = r.Id
	}
	if r.Vi != "" {
		messages["vi"] = r.Vi
	}
	if r.Ko != "" {
		messages["ko"] = r.Ko
	}
	if r.Ja != "" {
		messages["ja"] = r.Ja
	}
	if r.Th != "" {
		messages["th"] = r.Th
	}
	return messages
}

// TelegramPromptResponse for paginated results
type TelegramPromptResponse struct {
	Prompts    []TelegramPrompt `json:"prompts"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	PerPage    int              `json:"per_page"`
	TotalPages int              `json:"total_pages"`
}
