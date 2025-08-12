package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// DeepCheckOrder represents a deep check order in the database
type DeepCheckOrder struct {
	ID               int           `json:"id" db:"id"`
	OrderID          string        `json:"order_id" db:"order_id"`
	UserID           int           `json:"user_id" db:"user_id"`
	DomainID         int           `json:"domain_id" db:"domain_id"`
	DomainName       string        `json:"domain_name" db:"domain_name"`
	Status           string        `json:"status" db:"status"`
	CreatedAt        time.Time     `json:"created_at" db:"created_at"`
	CompletedAt      *time.Time    `json:"completed_at" db:"completed_at"`
	CallbackReceived bool          `json:"callback_received" db:"callback_received"`
	CallbackData     *CallbackData `json:"callback_data" db:"callback_data"`
}

// CallbackData represents the JSONB callback data
type CallbackData map[string]interface{}

// Value implements the driver.Valuer interface for database storage
func (c CallbackData) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for database retrieval
func (c *CallbackData) Scan(value interface{}) error {
	if value == nil {
		*c = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return json.Unmarshal([]byte(value.(string)), c)
	}

	return json.Unmarshal(bytes, c)
}
