package model

import (
	"time"
)

// DomainCheckResult represents the result of a domain check
type DomainCheckResult struct {
	Domain           string    `json:"domain"`
	Region           string    `json:"region"`
	StatusCode       int       `json:"status_code"`
	ResponseTime     int       `json:"response_time_ms"` // in milliseconds
	Available        bool      `json:"available"`
	CheckedAt        time.Time `json:"checked_at"`
	ErrorCode        int       `db:"error_code" json:"error_code"`
	TotalTime        int       `db:"total_time" json:"total_time"`
	ErrorDescription string    `db:"error_description" json:"error_description"`
}

// DomainMonitorRequest represents a request to check domain status
type DomainMonitorRequest struct {
	Domain  string   `json:"domain"`
	Regions []string `json:"regions,omitempty"` // Optional specific regions
}

// DomainMonitorResponse represents the response from domain monitoring
type DomainMonitorResponse struct {
	Domain  string                        `json:"domain"`
	Results map[string]*DomainCheckResult `json:"results"` // Map of region to result
}
