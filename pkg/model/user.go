package model

import (
	"database/sql"
	"time"
)

// User represents a merchant user in the system
// User represents an application user
type User struct {
	ID               int            `json:"id" db:"id"`
	Username         string         `json:"username" db:"username"`
	PasswordHash     string         `json:"-" db:"password_hash"`
	Email            string         `json:"email" db:"email"`
	TwoFactorEnabled bool           `json:"two_factor_enabled" db:"two_factor_enabled"`
	TwoFactorSecret  sql.NullString `json:"-" db:"two_factor_secret"`
	CreatedAt        time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at" db:"updated_at"`
	Region           sql.NullString `json:"region" db:"region"` // Changed to sql.NullString
}

// UserCredentials is used for login requests
type UserCredentials struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TOTPCode string `json:"totp_code"`
}

// TwoFactorSetupResponse contains info for QR code setup
type TwoFactorSetupResponse struct {
	Secret    string `json:"secret"`
	QRCodeURL string `json:"qrcode_url"`
}

// TwoFactorVerifyRequest is used to verify and enable 2FA
type TwoFactorVerifyRequest struct {
	TOTPCode string `json:"totp_code" binding:"required"`
}

// RegistrationRequest represents the payload for user registration
type RegistrationRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
	Email    string `json:"email" binding:"required,email"`
	// Region   string `json:"region" binding:"required"`
}

// RegistrationResponse represents the success response after registration
type RegistrationResponse struct {
	Message string `json:"message"`
	UserID  int64  `json:"user_id"`
}

// PasswordUpdateRequest represents the request to update a user's password
type PasswordUpdateRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}
