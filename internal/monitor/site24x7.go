package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"domain-detection-go/pkg/model"
)

// Site24x7Config holds configuration for Site24x7 API
type Site24x7Config struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	BaseURL      string
}

// Site24x7Client is a client for the Site24x7 API
type Site24x7Client struct {
	config      Site24x7Config
	httpClient  *http.Client
	accessToken string
	tokenExpiry time.Time
	tokenMutex  sync.RWMutex
}

// TokenResponse represents the OAuth token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	APIDomain   string `json:"api_domain"`
}

// MonitorCreateRequest represents a monitor creation request
type MonitorCreateRequest struct {
	DisplayName           string   `json:"display_name"`
	Type                  string   `json:"type"`
	Website               string   `json:"website"`
	CheckFrequency        string   `json:"check_frequency"`
	Timeout               int      `json:"timeout"`
	HTTPMethod            string   `json:"http_method"`
	LocationProfileID     string   `json:"location_profile_id"`
	NotificationProfileID string   `json:"notification_profile_id"`
	ThresholdProfileID    string   `json:"threshold_profile_id"`
	UserGroupIDs          []string `json:"user_group_ids"`
	UseIPv6               bool     `json:"use_ipv6"`
	MatchCase             bool     `json:"match_case"`
	UserAgent             string   `json:"user_agent"`
	UseNameServer         bool     `json:"use_name_server"`
}

// MonitorCreateResponse represents a monitor creation response
type MonitorCreateResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		MonitorID             string   `json:"monitor_id"`
		DisplayName           string   `json:"display_name"`
		Type                  string   `json:"type"`
		Website               string   `json:"website"`
		CheckFrequency        string   `json:"check_frequency"`
		Timeout               int      `json:"timeout"`
		HTTPMethod            string   `json:"http_method"`
		LocationProfileID     string   `json:"location_profile_id"`
		NotificationProfileID string   `json:"notification_profile_id"`
		ThresholdProfileID    string   `json:"threshold_profile_id"`
		UserGroupIDs          []string `json:"user_group_ids"`
		UseIPv6               bool     `json:"use_ipv6"`
		MatchCase             bool     `json:"match_case"`
		UserAgent             string   `json:"user_agent"`
		UseNameServer         bool     `json:"use_name_server"`
	} `json:"data"`
}

// LogReportResponse represents the log report response
type LogReportResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Headers   map[string]string `json:"headers"`
		Hide      []string          `json:"hide"`
		Report    []LogEntry        `json:"report"`
		StaticCol []string          `json:"static_col"`
	} `json:"data"`
}

// LogEntry represents a single log entry
type LogEntry struct {
	CollectionTime     string `json:"collection_time"`
	LocationID         string `json:"location_id"`
	Availability       string `json:"availability"`
	ResponseCode       string `json:"response_code"`
	DNSTime            string `json:"dns_time"`
	ConnectionTime     string `json:"connection_time"`
	SSLTime            string `json:"ssl_time"`
	FirstByteTime      string `json:"firstbyte_time"`
	DownloadTime       string `json:"download_time"`
	ResponseTime       string `json:"response_time"`
	ContentLength      string `json:"content_length"`
	ResolvedIP         string `json:"resolved_ip"`
	Reason             string `json:"reason"`
	ProtocolUsed       string `json:"protocol_used"`
	HashFunction       string `json:"hash_function"`
	BulkEncryption     string `json:"bulk_encryption"`
	KeyExchange        string `json:"key_exchange"`
	RedirectCount      string `json:"redirect_count"`
	ASNName            string `json:"asn_name"`
	CT                 string `json:"ct"`
	DataCollectionType string `json:"data_collection_type"`
	LocIP              string `json:"locIp"`
	APMKey             string `json:"apmkey"`
	NameServer         string `json:"nameserver"`
}

// NewSite24x7Client creates a new client for the Site24x7 API
func NewSite24x7Client(config Site24x7Config) *Site24x7Client {
	return &Site24x7Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// getAccessToken gets a valid access token, refreshing if necessary
func (c *Site24x7Client) getAccessToken() (string, error) {
	c.tokenMutex.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.tokenMutex.RUnlock()
		return token, nil
	}
	c.tokenMutex.RUnlock()

	c.tokenMutex.Lock()
	defer c.tokenMutex.Unlock()

	// Double-check after acquiring write lock
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	// Refresh token
	data := url.Values{}
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)
	data.Set("refresh_token", c.config.RefreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := c.httpClient.PostForm("https://accounts.zoho.com/oauth/v2/token", data)
	if err != nil {
		return "", fmt.Errorf("error refreshing token: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("error parsing token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	// Set expiry to 50 minutes (token expires in 60 minutes)
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-600) * time.Second)

	log.Printf("Site24x7 token refreshed, expires at: %v", c.tokenExpiry)

	return c.accessToken, nil
}

// getSite24x7LocationProfileID maps region code to Site24x7 location profile ID
func getSite24x7LocationProfileID(region string) string {
	switch region {
	case "CN", "China":
		return "567462000000029011"
	case "ID", "Indonesia":
		return "567462000000029013"
	case "IN", "India":
		return "567462000000029015"
	case "JP", "Japan":
		return "567462000000029017"
	case "KR", "Korea":
		return "567462000000029023"
	case "TH", "Thailand":
		return "567462000000029019"
	case "VN", "Vietnam":
		return "567462000000029021"
	default:
		return "567462000000029011" // Default to China
	}
}

// CreateMonitor creates a new monitor in Site24x7
func (c *Site24x7Client) CreateMonitor(fullURL string, name string, regions []string) (string, error) {
	token, err := c.getAccessToken()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	// Parse the URL to determine HTTP method
	httpMethod := "G" // GET
	if fullURL == "" {
		return "", fmt.Errorf("URL cannot be empty")
	}

	// Use the first region from the array (Site24x7 uses single region per monitor)
	region := "CN" // Default
	if len(regions) > 0 {
		region = regions[0]
	}

	// Get the appropriate location profile ID for the user's region
	locationProfileID := getSite24x7LocationProfileID(region)

	createReq := MonitorCreateRequest{
		DisplayName:           fmt.Sprintf("Monitor - %s", name),
		Type:                  "URL",
		Website:               fullURL,
		CheckFrequency:        "5", // Check every 5 minutes
		Timeout:               15,
		HTTPMethod:            httpMethod,
		LocationProfileID:     locationProfileID,              // Use region-specific location profile
		NotificationProfileID: "567462000000029001",           // Default notification profile
		ThresholdProfileID:    "567462000000029007",           // Default threshold profile
		UserGroupIDs:          []string{"567462000000025009"}, // Default user group
		UseIPv6:               false,
		MatchCase:             false,
		UserAgent:             "Mozilla Firefox",
		UseNameServer:         false,
	}

	jsonData, err := json.Marshal(createReq)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://www.site24x7.com/api/monitors", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json; version=2.1")
	req.Header.Set("Authorization", fmt.Sprintf("Zoho-oauthtoken %s", token))

	log.Printf("Creating Site24x7 monitor for %s in region %s (profile: %s)", fullURL, region, locationProfileID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	// Site24x7 returns 201 (Created) for successful monitor creation, not 200 (OK)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned non-success status: %d, body: %s", resp.StatusCode, string(body))
	}

	var createResp MonitorCreateResponse
	if err := json.Unmarshal(body, &createResp); err != nil {
		return "", fmt.Errorf("error parsing response: %w", err)
	}

	if createResp.Code != 0 {
		return "", fmt.Errorf("Site24x7 API error: %s", createResp.Message)
	}

	log.Printf("Created Site24x7 monitor %s for %s in region %s", createResp.Data.MonitorID, fullURL, region)

	return createResp.Data.MonitorID, nil
}

// UpdateMonitorStatus updates the status of a monitor
func (c *Site24x7Client) UpdateMonitorStatus(monitorID string, isActive bool) error {
	token, err := c.getAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Site24x7 uses PUT with JSON data to update monitor status
	endpoint := fmt.Sprintf("https://www.site24x7.com/api/monitors/%s", monitorID)

	// Create update request body
	updateRequest := map[string]interface{}{
		"monitor_id":    monitorID,
		"suspend_alert": !isActive, // Site24x7 uses suspend_alert: true to disable, false to enable
	}

	jsonData, err := json.Marshal(updateRequest)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json; version=2.1")
	req.Header.Set("Authorization", fmt.Sprintf("Zoho-oauthtoken %s", token))

	log.Printf("Updating Site24x7 monitor %s status to active=%v", monitorID, isActive)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	// Accept both 200 (OK) and 201 (Created) as success
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API returned non-success status: %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response to check for success
	var updateResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &updateResp); err != nil {
		return fmt.Errorf("error parsing response: %w", err)
	}

	if updateResp.Code != 0 {
		return fmt.Errorf("Site24x7 API error: %s", updateResp.Message)
	}

	log.Printf("Successfully updated Site24x7 monitor %s status to active=%v", monitorID, isActive)
	return nil
}

// DeleteMonitor deletes a monitor
func (c *Site24x7Client) DeleteMonitor(monitorID string) error {
	token, err := c.getAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	endpoint := fmt.Sprintf("https://www.site24x7.com/api/monitors/%s", monitorID)

	req, err := http.NewRequest("DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json; version=2.1")
	req.Header.Set("Authorization", fmt.Sprintf("Zoho-oauthtoken %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Accept 200 (OK), 204 (No Content), and 201 (Created) as success for deletion
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("API returned non-success status: %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("Successfully deleted Site24x7 monitor %s", monitorID)
	return nil
}

// GetLatestMonitorCheck gets the latest check result for a monitor
func (c *Site24x7Client) GetLatestMonitorCheck(monitorID, region string) (*model.DomainCheckResult, error) {
	token, err := c.getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Calculate time range (last 15 minutes)
	now := time.Now()
	startTime := now.Add(-15 * time.Minute)

	// Format times for Site24x7 API (UTC+8)
	location, _ := time.LoadLocation("Asia/Shanghai")
	startTimeStr := startTime.In(location).Format("2006-01-02T15:04:05-0700")
	endTimeStr := now.In(location).Format("2006-01-02T15:04:05-0700")

	// Build request URL
	requestURL := fmt.Sprintf("https://www.site24x7.com/api/reports/log_reports/%s?start_date=%s&end_date=%s",
		monitorID,
		url.QueryEscape(startTimeStr),
		url.QueryEscape(endTimeStr))

	log.Printf("Getting Site24x7 log reports for monitor %s: %s", monitorID, requestURL)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json; version=2.0")
	req.Header.Set("Authorization", fmt.Sprintf("Zoho-oauthtoken %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-success status: %d, body: %s", resp.StatusCode, string(body))
	}

	var logResp LogReportResponse
	if err := json.Unmarshal(body, &logResp); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	if logResp.Code != 0 {
		return nil, fmt.Errorf("Site24x7 API error: %s", logResp.Message)
	}

	// Check if we have any data
	if len(logResp.Data.Report) == 0 {
		return nil, fmt.Errorf("no log entries found for monitor %s", monitorID)
	}

	// Get the latest entry (first in the list as they should be sorted by time desc)
	latestEntry := logResp.Data.Report[0]

	// Parse response data
	statusCode := 0
	if latestEntry.ResponseCode != "" && latestEntry.ResponseCode != "-" {
		fmt.Sscanf(latestEntry.ResponseCode, "%d", &statusCode)
	}

	responseTime := 0
	if latestEntry.ResponseTime != "" && latestEntry.ResponseTime != "-" {
		fmt.Sscanf(latestEntry.ResponseTime, "%d", &responseTime)
	}

	// Parse availability (1 = available, 0 = unavailable)
	available := latestEntry.Availability == "1"

	// Parse timestamp
	checkedAt, err := time.Parse("2006-01-02T15:04:05-0700", latestEntry.CollectionTime)
	if err != nil {
		log.Printf("Could not parse timestamp '%s': %v. Using current time.", latestEntry.CollectionTime, err)
		checkedAt = time.Now()
	}

	result := &model.DomainCheckResult{
		Domain:           "", // Will be filled in by caller
		StatusCode:       statusCode,
		ResponseTime:     responseTime,
		Available:        available,
		CheckedAt:        checkedAt,
		ErrorCode:        0,
		TotalTime:        responseTime,
		ErrorDescription: latestEntry.Reason,
	}

	return result, nil
}

// Close cleans up resources used by the client
func (c *Site24x7Client) Close() {
	// No persistent connections to close for Site24x7
}
