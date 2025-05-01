package auth

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"domain-detection-go/pkg/model"
)

// AuthService handles authentication operations
type AuthService struct {
	db            *sqlx.DB
	jwtSecret     []byte
	encryptionKey string
}

// NewAuthService creates a new authentication service
func NewAuthService(db *sqlx.DB, jwtSecret, encryptionKey string) *AuthService {
	return &AuthService{
		db:            db,
		jwtSecret:     []byte(jwtSecret),
		encryptionKey: encryptionKey,
	}
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPassword compares password with hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateJWT creates a new JWT token for authenticated users
func (s *AuthService) GenerateJWT(userID int, username, region string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["user_id"] = userID
	claims["username"] = username
	claims["region"] = region
	claims["exp"] = time.Now().Add(time.Hour * 24).Unix()

	return token.SignedString(s.jwtSecret)
}

// Login authenticates a user and handles 2FA if enabled
func (s *AuthService) Login(creds model.UserCredentials) (*model.User, string, error) {
	var user model.User

	err := s.db.Get(&user, "SELECT * FROM users WHERE username = $1", creds.Username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", errors.New("invalid username or password")
		}
		return nil, "", err
	}

	// Check password
	if !CheckPassword(creds.Password, user.PasswordHash) {
		return nil, "", errors.New("invalid username or password")
	}

	// Check if 2FA is enabled
	if user.TwoFactorEnabled {
		// If 2FA is enabled, validate the TOTP code
		if creds.TOTPCode == "" {
			return &user, "", errors.New("2fa_required")
		}

		// Decrypt the secret
		secret, err := DecryptTOTPSecret(user.TwoFactorSecret, s.encryptionKey)
		if err != nil {
			return nil, "", errors.New("error processing 2FA")
		}

		// Validate the TOTP code
		if !ValidateTOTP(secret, creds.TOTPCode) {
			return nil, "", errors.New("invalid 2FA code")
		}
	}

	// Generate JWT token
	token, err := s.GenerateJWT(user.ID, user.Username, user.Region)
	if err != nil {
		return nil, "", err
	}

	return &user, token, nil
}

// SetupTwoFactor initializes 2FA for a user
func (s *AuthService) SetupTwoFactor(userID int) (*model.TwoFactorSetupResponse, error) {
	// Add debugging
	log.Printf("Setting up 2FA for user ID: %d", userID)

	var user model.User
	err := s.db.Get(&user, "SELECT * FROM users WHERE id = $1", userID)
	if err != nil {
		log.Printf("Error fetching user: %v", err)
		return nil, err
	}

	// Generate a new TOTP secret
	secret, err := GenerateTOTPSecret()
	if err != nil {
		log.Printf("Error generating TOTP secret: %v", err)
		return nil, err
	}

	// Reset two_factor_enabled if it might be causing issues
	_, err = s.db.Exec("UPDATE users SET two_factor_enabled = false WHERE id = $1", userID)
	if err != nil {
		log.Printf("Error resetting 2FA flag: %v", err)
		// Continue anyway - not critical
	}

	// Encrypt the secret before storing it
	encryptedSecret, err := EncryptTOTPSecret(secret, s.encryptionKey)
	if err != nil {
		log.Printf("Error encrypting TOTP secret: %v", err)
		return nil, err
	}

	// Store the new secret - note we're using sql.NullString
	_, err = s.db.Exec("UPDATE users SET two_factor_secret = $1 WHERE id = $2",
		sql.NullString{String: encryptedSecret, Valid: true}, userID)
	if err != nil {
		log.Printf("Error updating user with new 2FA secret: %v", err)
		return nil, err
	}

	// Generate QR code URL
	qrCodeURL := GenerateTOTPQRCodeURL(secret, user.Email, "DomainDetection")

	return &model.TwoFactorSetupResponse{
		Secret:    secret,
		QRCodeURL: qrCodeURL,
	}, nil
}

// VerifyAndEnableTwoFactor verifies the 2FA code and enables 2FA if valid
func (s *AuthService) VerifyAndEnableTwoFactor(userID int, code string) error {
	var user model.User

	err := s.db.Get(&user, "SELECT * FROM users WHERE id = $1", userID)
	if err != nil {
		return err
	}

	// Check if we have a valid secret
	if !user.TwoFactorSecret.Valid {
		return errors.New("two-factor authentication is not set up")
	}

	// Decrypt the secret
	secret, err := DecryptTOTPSecret(user.TwoFactorSecret, s.encryptionKey)
	if err != nil {
		return err
	}

	// Validate the TOTP code
	if !ValidateTOTP(secret, code) {
		return errors.New("invalid 2FA code")
	}

	// Enable 2FA for the user
	_, err = s.db.Exec("UPDATE users SET two_factor_enabled = true WHERE id = $1", userID)
	if err != nil {
		return err
	}

	return nil
}

// DisableTwoFactor disables 2FA for a user
func (s *AuthService) DisableTwoFactor(userID int) error {
	_, err := s.db.Exec("UPDATE users SET two_factor_enabled = false, two_factor_secret = NULL WHERE id = $1", userID)
	return err
}

// GetUserByID fetches a user by their ID
func (s *AuthService) GetUserByID(userID int) (*model.User, error) {
	var user model.User
	err := s.db.Get(&user, "SELECT * FROM users WHERE id = $1", userID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
