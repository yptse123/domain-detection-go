package deepcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// DeepCheckClient handles deep check API calls
type DeepCheckClient struct {
	httpClient *http.Client
	baseURL    string
}

// DeepCheckRequest represents the request to the deep check API
type DeepCheckRequest struct {
	ITDOG_TEST_URL string `json:"ITDOG_TEST_URL"`
}

// DeepCheckResponse represents the response from the deep check API
type DeepCheckResponse struct {
	OrderID        string `json:"orderID"`
	ITDOG_TEST_URL string `json:"ITDOG_TEST_URL"`
}

// NewDeepCheckClient creates a new deep check client
func NewDeepCheckClient() *DeepCheckClient {
	// Get base URL from environment variable
	baseURL := os.Getenv("DEEP_CHECK_BASE_URL")
	if baseURL == "" {
		// Fallback to default URL if not configured
		baseURL = "https://itdog-hq-public.passgfw-global-mixed-uat-eks.y8schwifty.app"
		log.Printf("[DEEP-CHECK] WARNING: DEEP_CHECK_BASE_URL not configured, using default: %s", baseURL)
	} else {
		log.Printf("[DEEP-CHECK] Using configured base URL: %s", baseURL)
	}

	return &DeepCheckClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// RequestDeepCheck sends a deep check request for the given URL
func (c *DeepCheckClient) RequestDeepCheck(url string) (*DeepCheckResponse, error) {
	// Prepare request payload
	request := DeepCheckRequest{
		ITDOG_TEST_URL: url,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[DEEP-CHECK] Requesting deep check for URL: %s", url)

	// Create HTTP request
	apiURL := fmt.Sprintf("%s/v1/hq/order", c.baseURL)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	log.Printf("[DEEP-CHECK] Making request to: %s", apiURL)
	log.Printf("[DEEP-CHECK] Request payload: %s", string(jsonData))

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[DEEP-CHECK] ERROR: Failed to send request: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	if err != nil {
		log.Printf("[DEEP-CHECK] ERROR: Failed to read response body: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[DEEP-CHECK] Response status: %d", resp.StatusCode)
	log.Printf("[DEEP-CHECK] Response body: %s", responseBody.String())

	// Check response status
	if resp.StatusCode != http.StatusOK {
		log.Printf("[DEEP-CHECK] ERROR: API returned status %d: %s", resp.StatusCode, responseBody.String())
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, responseBody.String())
	}

	// Parse response
	var deepCheckResp DeepCheckResponse
	if err := json.Unmarshal(responseBody.Bytes(), &deepCheckResp); err != nil {
		log.Printf("[DEEP-CHECK] ERROR: Failed to parse response: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	log.Printf("[DEEP-CHECK] SUCCESS: Order created - OrderID: %s, URL: %s",
		deepCheckResp.OrderID, deepCheckResp.ITDOG_TEST_URL)

	return &deepCheckResp, nil
}

// Close cleans up resources
func (c *DeepCheckClient) Close() {
	// No resources to clean up for now
}
