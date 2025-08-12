package deepcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// DeepCheckCallbackRequest represents the callback from the deep check service
type DeepCheckCallbackRequest struct {
	OrderID string            `json:"orderID"`
	Records []DeepCheckRecord `json:"records"`
	Count   int               `json:"count"`
}

// DeepCheckRecord represents a single test record from different regions
type DeepCheckRecord struct {
	Type         string `json:"type"`          // "success" or "error"
	IP           string `json:"ip"`            // Resolved IP
	HTTPCode     int    `json:"http_code"`     // HTTP status code
	AllTime      string `json:"all_time"`      // Total response time
	DNSTime      string `json:"dns_time"`      // DNS resolution time
	ConnectTime  string `json:"connect_time"`  // Connection time
	DownloadTime string `json:"download_time"` // Download time
	Redirect     int    `json:"redirect"`      // Number of redirects
	RedirectTime string `json:"redirect_time"` // Redirect time
	Head         string `json:"head"`          // Response headers
	NodeID       int    `json:"node_id"`       // Test node ID
	Line         int    `json:"line"`          // Network line type
	Name         string `json:"name"`          // Node name (e.g., "山西太原电信")
	Region       int    `json:"region"`        // Region ID
	Province     int    `json:"province"`      // Province ID
	Address      string `json:"address"`       // IP address location
	ISP          string `json:"isp"`           // ISP name (电信/联通/移动)
	City         string `json:"city"`          // City name
	RegionName   string `json:"regionName"`    // Region name (华北/华东/etc)
}

// DeepCheckSummary represents analysis summary of the deep check results
type DeepCheckSummary struct {
	TotalNodes   int
	SuccessNodes int
	ErrorNodes   int
	SuccessRate  float64
	Status       string // "全部正常", "部分異常", "全部異常"
	StatusEmoji  string
	TargetDomain string
	CheckTime    time.Time
}

// GetResponseTimeMs converts the time string to milliseconds for display
func (r *DeepCheckRecord) GetResponseTimeMs() int {
	timeFloat, err := strconv.ParseFloat(r.AllTime, 64)
	if err != nil {
		return 0
	}
	return int(timeFloat * 1000) // Convert seconds to milliseconds
}

// IsHealthy returns true if the record indicates a healthy response
func (r *DeepCheckRecord) IsHealthy() bool {
	return r.Type == "success" && (r.HTTPCode == 0 || (r.HTTPCode >= 200 && r.HTTPCode < 400))
}

// GetStatusDescription returns a description of the status
func (r *DeepCheckRecord) GetStatusDescription() string {
	if r.HTTPCode == 0 && r.Type == "success" {
		return "連線正常"
	}

	switch r.HTTPCode {
	case 200:
		return "正常"
	case 301, 302, 303, 307, 308:
		return "重新導向"
	case 404:
		return "頁面不存在"
	case 500:
		return "伺服器錯誤"
	case 502:
		return "壞閘道"
	case 503:
		return "服務不可用"
	case 504:
		return "閘道逾時"
	case 0:
		return "無回應"
	default:
		if r.HTTPCode >= 400 && r.HTTPCode < 500 {
			return "客戶端錯誤"
		} else if r.HTTPCode >= 500 {
			return "伺服器錯誤"
		}
		return "未知狀態"
	}
}

// AnalyzeResults analyzes the deep check results and returns a summary
func (req *DeepCheckCallbackRequest) AnalyzeResults(targetDomain string) *DeepCheckSummary {
	summary := &DeepCheckSummary{
		TotalNodes:   req.Count,
		TargetDomain: targetDomain,
		CheckTime:    time.Now(),
	}

	successCount := 0
	for _, record := range req.Records {
		if record.IsHealthy() {
			successCount++
		}
	}

	summary.SuccessNodes = successCount
	summary.ErrorNodes = summary.TotalNodes - successCount
	summary.SuccessRate = float64(successCount) / float64(summary.TotalNodes) * 100

	// Determine status
	if successCount == summary.TotalNodes {
		summary.Status = "全部正常"
		summary.StatusEmoji = "✅"
	} else if successCount == 0 {
		summary.Status = "全部異常"
		summary.StatusEmoji = "🔴"
	} else {
		summary.Status = "部分異常"
		summary.StatusEmoji = "🟡"
	}

	return summary
}

// FormatTelegramMessage formats the callback results for Telegram
func (req *DeepCheckCallbackRequest) FormatTelegramMessage(targetDomain string) string {
	summary := req.AnalyzeResults(targetDomain)

	var message strings.Builder

	// Header
	message.WriteString("🌐 **深度網絡檢測報告**\n\n")

	// Summary
	message.WriteString(fmt.Sprintf("%s **%s**：%d/%d 節點正常 (%.1f%%)\n",
		summary.StatusEmoji, summary.Status, summary.SuccessNodes, summary.TotalNodes, summary.SuccessRate))

	message.WriteString(fmt.Sprintf("📍 **目標域名**：%s\n", targetDomain))
	message.WriteString(fmt.Sprintf("🕓 **檢查時間**：%s\n", summary.CheckTime.Format("2006-01-02 15:04:05 (UTC+8)")))
	message.WriteString(fmt.Sprintf("🔍 **訂單編號**：%s\n\n", req.OrderID))

	switch summary.Status {
	case "全部正常":
		message.WriteString("🟢 **全部節點連線正常**\n\n")
		message.WriteString("**詳細結果**：\n")
		message.WriteString("```\n")
		message.WriteString("省份      | 城市     | 電訊商 | 響應時間 | 狀態碼\n")
		message.WriteString("---------|---------|-------|---------|-------\n")

		for _, record := range req.Records {
			city := strings.Split(record.City, record.Name)[0]
			if city == "" {
				city = "–"
			}
			message.WriteString(fmt.Sprintf("%-8s | %-7s | %-4s | %4dms | %d\n",
				record.RegionName, city, record.ISP, record.GetResponseTimeMs(), record.HTTPCode))
		}
		message.WriteString("```")

	case "部分異常":
		message.WriteString("🟡 **部分異常**：部份地區訪問緩慢或跳轉多\n\n")

		// Error regions
		message.WriteString("**異常地區列表**：\n")
		message.WriteString("```\n")
		message.WriteString("省份      | 城市     | 電訊商 | 響應時間 | 狀態碼 | 描述\n")
		message.WriteString("---------|---------|-------|---------|-------|--------\n")

		for _, record := range req.Records {
			if !record.IsHealthy() {
				city := strings.Split(record.City, record.Name)[0]
				if city == "" {
					city = "–"
				}
				responseTime := fmt.Sprintf("%dms", record.GetResponseTimeMs())
				if record.HTTPCode == 0 {
					responseTime = "–"
				}
				message.WriteString(fmt.Sprintf("%-8s | %-7s | %-4s | %-7s | %-5d | %s\n",
					record.RegionName, city, record.ISP, responseTime, record.HTTPCode, record.GetStatusDescription()))
			}
		}
		message.WriteString("```\n\n")

		// Normal regions
		message.WriteString("**正常地區**：\n")
		message.WriteString("```\n")
		message.WriteString("省份      | 城市     | 電訊商 | 響應時間 | 狀態碼\n")
		message.WriteString("---------|---------|-------|---------|-------\n")

		for _, record := range req.Records {
			if record.IsHealthy() {
				city := strings.Split(record.City, record.Name)[0]
				if city == "" {
					city = "–"
				}
				message.WriteString(fmt.Sprintf("%-8s | %-7s | %-4s | %4dms | %d\n",
					record.RegionName, city, record.ISP, record.GetResponseTimeMs(), record.HTTPCode))
			}
		}
		message.WriteString("```")

	default: // 全部異常
		message.WriteString("🔴 **所有地區無法訪問域名**\n\n")
		message.WriteString("🚨 **全部異常**\n\n")

		message.WriteString("**詳細錯誤資訊**：\n")
		message.WriteString("```\n")
		message.WriteString("省份      | 城市     | 電訊商 | 響應時間 | 狀態碼 | 問題描述\n")
		message.WriteString("---------|---------|-------|---------|-------|----------\n")

		for _, record := range req.Records {
			city := strings.Split(record.City, record.Name)[0]
			if city == "" {
				city = "–"
			}
			responseTime := fmt.Sprintf("%dms", record.GetResponseTimeMs())
			if record.HTTPCode == 0 {
				responseTime = "–"
			}
			message.WriteString(fmt.Sprintf("%-8s | %-7s | %-4s | %-7s | %-5d | %s\n",
				record.RegionName, city, record.ISP, responseTime, record.HTTPCode, record.GetStatusDescription()))
		}
		message.WriteString("```")
	}

	formattedMessage := message.String()

	// LOG THE RAW TELEGRAM MESSAGE FOR PREVIEW
	log.Printf("[DEEP-CHECK] RAW TELEGRAM MESSAGE PREVIEW:\n%s", formattedMessage)
	log.Printf("[DEEP-CHECK] TELEGRAM MESSAGE LENGTH: %d characters", len(formattedMessage))

	return formattedMessage
}

// FormatEmailMessage formats the callback results for Email (HTML format)
func (req *DeepCheckCallbackRequest) FormatEmailMessage(targetDomain string) (string, string) {
	summary := req.AnalyzeResults(targetDomain)

	subject := fmt.Sprintf("深度網絡檢測報告 - %s [%s]", targetDomain, summary.Status)

	var body strings.Builder
	body.WriteString(`
	<!DOCTYPE html>
	<html>
	<head>
		<meta charset="UTF-8">
		<style>
			body { font-family: Arial, sans-serif; line-height: 1.6; }
			.header { background-color: #f4f4f4; padding: 20px; text-align: center; }
			.content { padding: 20px; }
			.summary { background-color: #e7f3ff; padding: 15px; margin: 10px 0; border-radius: 5px; }
			table { width: 100%; border-collapse: collapse; margin: 15px 0; }
			th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
			th { background-color: #f2f2f2; }
			.success { color: #28a745; }
			.warning { color: #ffc107; }
			.danger { color: #dc3545; }
		</style>
	</head>
	<body>`)

	body.WriteString(fmt.Sprintf(`
    <div class="header">
        <h2>🌐 深度網絡檢測報告</h2>
        <p>%s %s：%d/%d 節點正常 (%.1f%%)</p>
    </div>
    <div class="content">
        <div class="summary">
            <p><strong>📍 目標域名：</strong>%s</p>
            <p><strong>🕓 檢查時間：</strong>%s</p>
            <p><strong>🔍 訂單編號：</strong>%s</p>
        </div>`,
		summary.StatusEmoji, summary.Status, summary.SuccessNodes, summary.TotalNodes, summary.SuccessRate,
		targetDomain, summary.CheckTime.Format("2006-01-02 15:04:05 (UTC+8)"), req.OrderID))

	if summary.Status == "部分異常" {
		// Error table
		body.WriteString(`<h3 class="warning">異常地區列表：</h3>`)
		body.WriteString(`<table><tr><th>省份</th><th>城市</th><th>電訊商</th><th>響應時間</th><th>狀態碼</th><th>描述</th></tr>`)
		for _, record := range req.Records {
			if !record.IsHealthy() {
				city := strings.Split(record.City, record.Name)[0]
				if city == "" {
					city = "–"
				}
				responseTime := fmt.Sprintf("%dms", record.GetResponseTimeMs())
				if record.HTTPCode == 0 {
					responseTime = "–"
				}
				body.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>`,
					record.RegionName, city, record.ISP, responseTime, record.HTTPCode, record.GetStatusDescription()))
			}
		}
		body.WriteString(`</table>`)

		// Success table
		body.WriteString(`<h3 class="success">正常地區：</h3>`)
	} else {
		body.WriteString(`<h3>詳細結果：</h3>`)
	}

	// Main results table
	body.WriteString(`<table><tr><th>省份</th><th>城市</th><th>電訊商</th><th>響應時間</th><th>狀態碼</th></tr>`)
	for _, record := range req.Records {
		if summary.Status == "部分異常" && !record.IsHealthy() {
			continue // Skip error records for partial failure (already shown above)
		}

		city := strings.Split(record.City, record.Name)[0]
		if city == "" {
			city = "–"
		}

		statusClass := "success"
		if !record.IsHealthy() {
			statusClass = "danger"
		}

		body.WriteString(fmt.Sprintf(`<tr class="%s"><td>%s</td><td>%s</td><td>%s</td><td>%dms</td><td>%d</td></tr>`,
			statusClass, record.RegionName, city, record.ISP, record.GetResponseTimeMs(), record.HTTPCode))
	}
	body.WriteString(`</table>`)

	body.WriteString(`
		</div>
	</body>
	</html>`)

	htmlBody := body.String()

	// LOG THE RAW EMAIL MESSAGE FOR PREVIEW
	log.Printf("[DEEP-CHECK] RAW EMAIL SUBJECT PREVIEW: %s", subject)
	log.Printf("[DEEP-CHECK] RAW EMAIL HTML BODY PREVIEW:\n%s", htmlBody)
	log.Printf("[DEEP-CHECK] EMAIL HTML BODY LENGTH: %d characters", len(htmlBody))

	return subject, htmlBody
}
