package auth

import (
	"domain-detection-go/pkg/model"
	"errors"
	"time"
)

// RegisterUser handles user registration
func (s *AuthService) RegisterUser(req model.RegistrationRequest) (int64, error) {
	// Check if username already exists
	var count int
	err := s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE username = $1", req.Username)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, errors.New("username already exists")
	}

	// Check if email already exists
	err = s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE email = $1", req.Email)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, errors.New("email already exists")
	}

	// Hash password
	hashedPassword, err := HashPassword(req.Password)
	if err != nil {
		return 0, err
	}

	// Insert new user (without region)
	var userID int64
	err = s.db.QueryRow(
		`INSERT INTO users (username, password_hash, email, two_factor_enabled, created_at, updated_at) 
         VALUES ($1, $2, $3, $4, $5, $5) 
         RETURNING id`,
		req.Username, hashedPassword, req.Email, false, time.Now()).Scan(&userID)
	if err != nil {
		return 0, err
	}

	return userID, nil
}

// GetRegions fetches all active regions from the database
func (s *AuthService) GetRegions() ([]model.Region, error) {
	var regions []model.Region
	err := s.db.Select(&regions, "SELECT code, name, is_active FROM regions WHERE is_active = TRUE ORDER BY name")
	return regions, err
}
