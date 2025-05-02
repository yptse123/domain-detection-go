package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"domain-detection-go/pkg/model"
)

// UptrendsConfig holds configuration for Uptrends API
type UptrendsConfig struct {
	APIKey      string
	APIUsername string
	BaseURL     string
	MaxRetries  int
	RetryDelay  time.Duration
}

// UptrendsClient is a client for the Uptrends API
type UptrendsClient struct {
	config      UptrendsConfig
	httpClient  *http.Client
	rateLimiter *time.Ticker
	// mu          sync.Mutex
}

// NewUptrendsClient creates a new client for the Uptrends API
func NewUptrendsClient(config UptrendsConfig) *UptrendsClient {
	// Set default values if not provided
	if config.BaseURL == "" {
		config.BaseURL = "https://api.uptrends.com/v4"
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 2 * time.Second
	}

	// Rate limit to avoid hitting API limits (1 request per second)
	rateLimiter := time.NewTicker(1 * time.Second)

	client := &UptrendsClient{
		config:      config,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rateLimiter: rateLimiter,
	}

	// Fetch checkpoint IDs at startup
	// go func() {
	// 	checkpoints, err := client.GetCheckpoints()
	// 	if err != nil {
	// 		log.Printf("Error fetching checkpoints: %v", err)
	// 		return
	// 	}

	// 	log.Printf("Successfully loaded %d checkpoints", len(checkpoints))
	// 	// You can store these checkpoints for later use
	// }()

	return client
}

// Updated getCheckpointIdForRegion with the correct IDs
func getCheckpointIdForRegion(region string) string {
	// Use the verified checkpoint IDs
	switch region {
	case "CN", "China":
		// For China, we have multiple options, but let's use Beijing as primary
		return "77" // Beijing
	case "JP", "Japan":
		return "49" // Tokyo
	case "KR", "Korea":
		return "88" // Seoul
	case "TH", "Thailand":
		return "96" // Bangkok
	case "IN", "India":
		return "36" // Mumbai
	case "ID", "Indonesia":
		// No direct checkpoint for Indonesia, use a nearby one like Singapore
		return "19" // Singapore as fallback
	case "VN", "Vietnam":
		return "168" // Hanoi
	default:
		return "19" // Singapore as default
	}
}

// Updated GetCheckpoints function to parse the correct response format
func (c *UptrendsClient) GetCheckpoints() (map[string]string, error) {
	// Wait for rate limiter
	<-c.rateLimiter.C

	// Fetch checkpoints from API
	url := fmt.Sprintf("%s/Checkpoint", c.config.BaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add authentication
	req.SetBasicAuth(c.config.APIUsername, c.config.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching checkpoints: %w", err)
	}
	defer resp.Body.Close()

	// Read and log abbreviated response for debugging
	body, _ := ioutil.ReadAll(resp.Body)
	bodyPreview := string(body)
	if len(bodyPreview) > 1000 {
		bodyPreview = bodyPreview[:1000] + "... (truncated)"
	}
	log.Printf("Checkpoints API response (preview): %s", bodyPreview)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-200 status when getting checkpoints: %d", resp.StatusCode)
	}

	// Parse checkpoints with the correct structure
	var checkpointResponse struct {
		Data []struct {
			Id         int    `json:"Id"`
			Type       string `json:"Type"`
			Attributes struct {
				CheckpointName      string   `json:"CheckpointName"`
				Code                string   `json:"Code"`
				Ipv4Addresses       []string `json:"Ipv4Addresses"`
				IpV6Addresses       []string `json:"IpV6Addresses"`
				IsPrimaryCheckpoint bool     `json:"IsPrimaryCheckpoint"`
				SupportsIpv6        bool     `json:"SupportsIpv6"`
				HasHighAvailability bool     `json:"HasHighAvailability"`
			} `json:"Attributes"`
			Links struct {
				Self string `json:"Self"`
			} `json:"Links"`
		} `json:"Data"`
	}

	if err := json.Unmarshal(body, &checkpointResponse); err != nil {
		return nil, fmt.Errorf("error parsing checkpoints: %w", err)
	}

	// Map checkpoint names and codes to IDs
	checkpointMap := make(map[string]string)
	for _, cp := range checkpointResponse.Data {
		idStr := fmt.Sprintf("%d", cp.Id)
		name := cp.Attributes.CheckpointName
		code := cp.Attributes.Code

		log.Printf("Found checkpoint: %s (ID: %s, Code: %s)", name, idStr, code)

		// Store by name and code
		checkpointMap[name] = idStr
		checkpointMap[code] = idStr
	}

	return checkpointMap, nil
}

// CreateMonitor creates a new monitor in Uptrends
func (c *UptrendsClient) CreateMonitor(domain string, name string, region string) (string, error) {
	// Wait for rate limiter
	<-c.rateLimiter.C

	// Map region code to Uptrends region ID
	regionID := getUptrendsRegionID(region)

	// Create request body
	requestBody := map[string]interface{}{
		"MonitorType": "Https",
		"Url":         fmt.Sprintf("https://%s", domain),
		"SelectedCheckpoints": map[string]interface{}{
			"Checkpoints":      []interface{}{},
			"Regions":          []int{regionID},
			"ExcludeLocations": []interface{}{},
		},
		"UsePrimaryCheckpointsOnly": false,
		"Name":                      name,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshalling request: %w", err)
	}

	// Log the request for debugging
	log.Printf("Creating monitor with request: %s", string(jsonData))

	// Build request
	url := fmt.Sprintf("%s/Monitor", c.config.BaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Add headers
	req.SetBasicAuth(c.config.APIUsername, c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	// Log full response for debugging
	log.Printf("Uptrends API response: %s", string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("API returned non-success status: %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		MonitorGuid string `json:"MonitorGuid"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("error parsing response: %w", err)
	}

	return response.MonitorGuid, nil
}

// UpdateMonitorStatus updates the IsActive status of a monitor in Uptrends
func (c *UptrendsClient) UpdateMonitorStatus(monitorGuid string, isActive bool) error {
	// Wait for rate limiter
	<-c.rateLimiter.C

	// Create request body
	requestBody := map[string]interface{}{
		"IsActive": isActive,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("error marshalling request: %w", err)
	}

	// Log the request for debugging
	log.Printf("Updating monitor %s active status to %v", monitorGuid, isActive)

	// Build request using PATCH method as specified in the API
	url := fmt.Sprintf("%s/Monitor/%s", c.config.BaseURL, monitorGuid)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Add headers
	req.SetBasicAuth(c.config.APIUsername, c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	// Log response for debugging
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		log.Printf("API error response: %s", string(body))
		return fmt.Errorf("API returned non-success status: %d", resp.StatusCode)
	}

	return nil
}

// DeleteMonitor deletes a monitor in Uptrends
func (c *UptrendsClient) DeleteMonitor(monitorGuid string) error {
	// Wait for rate limiter
	<-c.rateLimiter.C

	// Build request for DELETE method
	url := fmt.Sprintf("%s/Monitor/%s", c.config.BaseURL, monitorGuid)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("error creating delete request: %w", err)
	}

	// Add headers
	req.SetBasicAuth(c.config.APIUsername, c.config.APIKey)
	req.Header.Set("Accept", "application/json")

	// Log the request for debugging
	log.Printf("Deleting monitor with GUID: %s", monitorGuid)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error deleting monitor: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Printf("Error deleting monitor - status: %d, response: %s", resp.StatusCode, string(body))
		return fmt.Errorf("API returned non-success status: %d", resp.StatusCode)
	}

	log.Printf("Successfully deleted monitor with GUID: %s", monitorGuid)
	return nil
}

// GetLatestMonitorCheck gets the latest check result for a monitor
func (c *UptrendsClient) GetLatestMonitorCheck(monitorGuid string) (*model.DomainCheckResult, error) {
	// Wait for rate limiter
	<-c.rateLimiter.C

	// Build request URL with query parameters
	baseUrl := fmt.Sprintf("%s/MonitorCheck/Monitor/%s", c.config.BaseURL, monitorGuid)
	query := url.Values{}
	query.Add("Sorting", "Descending") // Get the most recent check
	query.Add("Take", "1")             // Only get the latest check
	query.Add("PresetPeriod", "Last2Hours")

	requestUrl := fmt.Sprintf("%s?%s", baseUrl, query.Encode())

	// Log the request for debugging
	log.Printf("Getting latest check for monitor %s: %s", monitorGuid, requestUrl)

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add authentication
	req.SetBasicAuth(c.config.APIUsername, c.config.APIKey)
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Log response for debugging
	log.Printf("Uptrends API check response: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-success status: %d", resp.StatusCode)
	}

	// Parse response with correct types for numeric fields
	var checkResponse struct {
		Data []struct {
			Type       string `json:"Type"`
			Id         int64  `json:"Id"`
			Attributes struct {
				MonitorGuid       string  `json:"MonitorGuid"`
				Timestamp         string  `json:"Timestamp"`
				ErrorCode         int     `json:"ErrorCode"`
				TotalTime         float64 `json:"TotalTime"`      // Changed to float64
				ResolveTime       float64 `json:"ResolveTime"`    // Changed to float64
				ConnectionTime    float64 `json:"ConnectionTime"` // Changed to float64
				DownloadTime      float64 `json:"DownloadTime"`   // Changed to float64
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
		} `json:"Data"`
	}

	if err := json.Unmarshal(body, &checkResponse); err != nil {
		log.Printf("Error parsing check response: %v", err)
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	// Check if we have any data
	if len(checkResponse.Data) == 0 {
		return nil, fmt.Errorf("no check results found for monitor %s", monitorGuid)
	}

	// Get the first (latest) result
	check := checkResponse.Data[0].Attributes

	// Determine if the check was successful based on ErrorLevel
	isAvailable := check.ErrorLevel == "NoError" || check.ErrorLevel == "Warning"

	// Parse the timestamp manually
	var checkedAt time.Time

	// Try different formats since Uptrends seems inconsistent
	formats := []string{
		"2006-01-02T15:04:05",       // Format without timezone
		"2006-01-02T15:04:05Z",      // Format with Z
		"2006-01-02T15:04:05Z07:00", // Full RFC3339
	}

	for _, format := range formats {
		checkedAt, err = time.Parse(format, check.Timestamp)
		if err == nil {
			break
		}
	}

	if err != nil {
		// If we couldn't parse the time, use current time as fallback
		log.Printf("Could not parse timestamp '%s': %v. Using current time.", check.Timestamp, err)
		checkedAt = time.Now()
	}

	// Create domain check result
	result := &model.DomainCheckResult{
		StatusCode:       check.HttpStatusCode,
		ResponseTime:     int(check.TotalTime), // Convert float to int
		Available:        isAvailable,
		CheckedAt:        checkedAt,
		ErrorCode:        check.ErrorCode,
		TotalTime:        int(check.TotalTime), // Convert float to int
		ErrorDescription: check.ErrorDescription,
	}

	return result, nil
}

// Map region code to Uptrends region ID
func getUptrendsRegionID(region string) int {
	switch region {
	case "CN", "China":
		return 45
	case "IN", "India":
		return 101
	case "JP", "Japan":
		return 109
	case "KR", "Korea":
		return 117
	case "TH", "Thailand":
		return 248
	case "ID", "Indonesia":
		return 251
	case "VN", "Vietnam":
		return 255
	default:
		return 45 // Default to China
	}
}

// Close cleans up resources used by the client
func (c *UptrendsClient) Close() {
	c.rateLimiter.Stop()
}
