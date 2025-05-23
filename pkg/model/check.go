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

type UpTrendCheckResult []struct {
	Type       string `json:"Type"`
	Id         int64  `json:"Id"`
	Attributes struct {
		MonitorGuid       string  `json:"MonitorGuid"`
		Timestamp         string  `json:"Timestamp"`
		ErrorCode         int     `json:"ErrorCode"`
		TotalTime         float64 `json:"TotalTime"`
		ResolveTime       float64 `json:"ResolveTime"`
		ConnectionTime    float64 `json:"ConnectionTime"`
		DownloadTime      float64 `json:"DownloadTime"`
		TotalBytes        int     `json:"TotalBytes"`
		ResolvedIpAddress string  `json:"ResolvedIpAddress"`
		ErrorLevel        string  `json:"ErrorLevel"`
		ErrorDescription  string  `json:"ErrorDescription"`
		ErrorMessage      string  `json:"ErrorMessage"`
		StagingMode       bool    `json:"StagingMode"`
		ServerId          int     `json:"ServerId"`
		HttpStatusCode    int     `json:"HttpStatusCode"`
		IsPartialCheck    bool    `json:"IsPartialCheck"`
		IsConcurrentCheck bool    `json:"IsConcurrentCheck"`
	} `json:"Attributes"`
	Relationships []struct {
		Id    int    `json:"Id"`
		Type  string `json:"Type"`
		Links struct {
			Self string `json:"Self"`
		} `json:"Links"`
	} `json:"Relationships"`
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
