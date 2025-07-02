package model

import "time"

// EmailConfig represents a user's email notification configuration
type EmailConfig struct {
	ID             int       `json:"id" db:"id"`
	UserID         int       `json:"user_id" db:"user_id"`
	EmailAddress   string    `json:"email_address" db:"email_address"`
	EmailName      string    `json:"email_name" db:"email_name"`
	Language       string    `json:"language" db:"language"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	NotifyOnDown   bool      `json:"notify_on_down" db:"notify_on_down"`
	NotifyOnUp     bool      `json:"notify_on_up" db:"notify_on_up"`
	MonitorRegions []string  `json:"monitor_regions"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// EmailConfigRequest represents a request to add/update email configuration
type EmailConfigRequest struct {
	EmailAddress   string   `json:"email_address" binding:"required,email"`
	EmailName      string   `json:"email_name"`
	Language       string   `json:"language"`
	NotifyOnDown   bool     `json:"notify_on_down"`
	NotifyOnUp     bool     `json:"notify_on_up"`
	IsActive       bool     `json:"active"`
	MonitorRegions []string `json:"monitor_regions"`
}
