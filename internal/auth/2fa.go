package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/pquerna/otp/totp"
)

// TOTPSecretSize is the size of the TOTP secret
const TOTPSecretSize = 20

// GenerateTOTPSecret generates a new random TOTP secret
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, TOTPSecretSize)
	_, err := rand.Read(secret)
	if err != nil {
		return "", err
	}

	// Convert to base32 (requirements for Google Authenticator)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// GenerateTOTPQRCodeURL generates a URL for QR code that can be scanned by Google Authenticator
func GenerateTOTPQRCodeURL(secret, accountName, issuer string) string {
	// Create a TOTP key with standard parameters
	secret = strings.TrimSpace(secret)

	// Create a standard TOTP URL
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?algorithm=SHA1&digits=6&issuer=%s&period=30&secret=%s",
		url.QueryEscape(issuer),
		url.QueryEscape(accountName),
		url.QueryEscape(issuer),
		secret, // Use the exact same secret here
	)
}

// ValidateTOTP checks if the provided TOTP code is valid for the given secret
func ValidateTOTP(secret sql.NullString, code string) bool {
	// Validate the TOTP code
	secretStr := secret.String
	return totp.Validate(code, secretStr)
}

// EncryptTOTPSecret encrypts the TOTP secret before storing in database
func EncryptTOTPSecret(secret, encryptionKey string) (string, error) {
	// Create a fixed-size key from the encryption key using SHA-256
	hash := sha256.Sum256([]byte(encryptionKey))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return "", err
	}

	// The IV needs to be unique, but not secure
	// Using a fixed IV is not recommended in production
	iv := make([]byte, aes.BlockSize)
	for i := 0; i < len(iv); i++ {
		iv[i] = byte(i)
	}

	// Pad the secret to be a multiple of the block size
	paddedSecret := padSecret(secret, aes.BlockSize)

	// Encrypt the secret
	encrypted := make([]byte, len(paddedSecret))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(encrypted, []byte(paddedSecret))

	// Return hex encoded string
	return hex.EncodeToString(encrypted), nil
}

// DecryptTOTPSecret decrypts the TOTP secret from database
func DecryptTOTPSecret(encryptedSecret sql.NullString, encryptionKey string) (sql.NullString, error) {
	if !encryptedSecret.Valid {
		return sql.NullString{Valid: false}, nil
	}

	// Decode the hex string
	encrypted, err := hex.DecodeString(encryptedSecret.String)
	if err != nil {
		return sql.NullString{Valid: false}, err
	}

	// Create cipher block
	hash := sha256.Sum256([]byte(encryptionKey))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return sql.NullString{Valid: false}, err
	}

	// Same IV as used in encryption
	iv := make([]byte, aes.BlockSize)
	for i := 0; i < len(iv); i++ {
		iv[i] = byte(i)
	}

	// Decrypt the secret
	decrypted := make([]byte, len(encrypted))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, encrypted)

	// Remove padding
	unpaddedSecret := unpadSecret(decrypted)

	return sql.NullString{String: string(unpaddedSecret), Valid: true}, nil
}

// Helper function to pad the secret to a multiple of blockSize
func padSecret(secret string, blockSize int) []byte {
	padding := blockSize - (len(secret) % blockSize)
	padtext := make([]byte, padding)
	for i := 0; i < padding; i++ {
		padtext[i] = byte(padding)
	}
	return append([]byte(secret), padtext...)
}

// Helper function to remove padding
func unpadSecret(data []byte) []byte {
	length := len(data)
	if length == 0 {
		return data
	}

	padding := int(data[length-1])
	if padding > length {
		return data // Invalid padding
	}

	return data[:length-padding]
}
