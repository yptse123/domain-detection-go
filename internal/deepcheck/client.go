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
	// If type is not success, it's definitely not healthy
	if r.Type != "success" {
		return false
	}

	// Get response time in milliseconds
	responseTimeMs := r.GetResponseTimeMs()

	// If HTTP code is 0, check response time
	if r.HTTPCode == 0 {
		// Response time > 10 seconds (10000ms) indicates timeout/failure
		return responseTimeMs > 0 && responseTimeMs <= 10000
	}

	// For non-zero HTTP codes, check if it's in success range
	return r.HTTPCode >= 200 && r.HTTPCode < 400
}

// GetStatusDescription returns a description of the status
func (r *DeepCheckRecord) GetStatusDescription() string {
	responseTimeMs := r.GetResponseTimeMs()

	if r.HTTPCode == 0 {
		if r.Type == "success" {
			if responseTimeMs > 10000 {
				return "連線超時"
			}
			return "連線正常"
		}
		return "無回應"
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
	switch successCount {
	case summary.TotalNodes:
		summary.Status = "全部正常"
		summary.StatusEmoji = "✅"
	case 0:
		summary.Status = "全部異常"
		summary.StatusEmoji = "🔴"
	default:
		summary.Status = "部分異常"
		summary.StatusEmoji = "🟡"
	}

	return summary
}

// FormatTelegramMessage formats the callback results for Telegram (split into multiple messages if needed)
func (req *DeepCheckCallbackRequest) FormatTelegramMessage(targetDomain string) []string {
	summary := req.AnalyzeResults(targetDomain)

	var messages []string
	const maxMessageLength = 4000 // Leave some buffer for safety

	// Message 1: Header and Summary
	var headerMessage strings.Builder
	headerMessage.WriteString("🌐 **深度網絡檢測報告**\n\n")
	headerMessage.WriteString(fmt.Sprintf("%s **%s**：%d/%d 節點正常 (%.1f%%)\n",
		summary.StatusEmoji, summary.Status, summary.SuccessNodes, summary.TotalNodes, summary.SuccessRate))
	headerMessage.WriteString(fmt.Sprintf("📍 **目標域名**：%s\n", targetDomain))
	headerMessage.WriteString(fmt.Sprintf("🕓 **檢查時間**：%s\n", summary.CheckTime.Format("2006-01-02 15:04:05 (UTC+8)")))
	headerMessage.WriteString(fmt.Sprintf("🔍 **訂單編號**：%s\n", req.OrderID))

	messages = append(messages, headerMessage.String())

	// Message 2+: Detailed results based on status
	switch summary.Status {
	case "全部正常":
		detailMessages := req.formatAllNormalMessages(maxMessageLength)
		messages = append(messages, detailMessages...)

	case "部分異常":
		detailMessages := req.formatPartialFailureMessages(maxMessageLength)
		messages = append(messages, detailMessages...)

	default: // 全部異常
		detailMessages := req.formatAllFailureMessages(maxMessageLength)
		messages = append(messages, detailMessages...)
	}

	// Log all messages for preview
	log.Printf("[DEEP-CHECK] TELEGRAM MESSAGES COUNT: %d", len(messages))
	for i, msg := range messages {
		log.Printf("[DEEP-CHECK] TELEGRAM MESSAGE %d PREVIEW (%d chars):\n%s", i+1, len(msg), msg)
	}

	return messages
}

// formatAllNormalMessages formats messages for all normal status
func (req *DeepCheckCallbackRequest) formatAllNormalMessages(maxLength int) []string {
	tableData := req.createTableDataForNormalResults()

	if len(tableData.Rows) == 0 {
		return []string{"🟢 **全部節點連線正常**\n\n沒有正常節點記錄。"}
	}

	prefix := "🟢 **全部節點連線正常**\n\n**詳細結果**：\n"
	messages := req.splitTableByMessageLength(tableData, maxLength, prefix)

	// Update continuation messages
	for i := 1; i < len(messages); i++ {
		messages[i] = strings.Replace(messages[i], prefix, "**詳細結果 (續)**：\n", 1)
	}

	return messages
}

// formatPartialFailureMessages formats messages for partial failure status
func (req *DeepCheckCallbackRequest) formatPartialFailureMessages(maxLength int) []string {
	var messages []string

	// Error regions table
	errorTableData := req.createTableDataForErrorResults()
	if len(errorTableData.Rows) > 0 {
		errorPrefix := "🟡 **部分異常**：部份地區訪問緩慢或跳轉多\n\n**異常地區列表**：\n"
		errorMessages := req.splitTableByMessageLength(errorTableData, maxLength, errorPrefix)

		// Update continuation messages
		for i := 1; i < len(errorMessages); i++ {
			errorMessages[i] = strings.Replace(errorMessages[i], errorPrefix, "**異常地區列表 (續)**：\n", 1)
		}

		messages = append(messages, errorMessages...)
	}

	// Normal regions table
	normalTableData := req.createTableDataForNormalResults()
	if len(normalTableData.Rows) > 0 {
		normalPrefix := "**正常地區**：\n"
		normalMessages := req.splitTableByMessageLength(normalTableData, maxLength, normalPrefix)

		// Update continuation messages
		for i := 1; i < len(normalMessages); i++ {
			normalMessages[i] = strings.Replace(normalMessages[i], normalPrefix, "**正常地區 (續)**：\n", 1)
		}

		messages = append(messages, normalMessages...)
	}

	return messages
}

// formatAllFailureMessages formats messages for all failure status
func (req *DeepCheckCallbackRequest) formatAllFailureMessages(maxLength int) []string {
	tableData := req.createTableDataForAllResults()

	if len(tableData.Rows) == 0 {
		return []string{"🔴 **所有地區無法訪問域名**\n\n🚨 **全部異常**\n\n沒有檢測記錄。"}
	}

	prefix := "🔴 **所有地區無法訪問域名**\n\n🚨 **全部異常**\n\n**詳細錯誤資訊**：\n"
	messages := req.splitTableByMessageLength(tableData, maxLength, prefix)

	// Update continuation messages
	for i := 1; i < len(messages); i++ {
		messages[i] = strings.Replace(messages[i], prefix, "**詳細錯誤資訊 (續)**：\n", 1)
	}

	return messages
}

// extractCityName extracts city name from record
func (req *DeepCheckCallbackRequest) extractCityName(record DeepCheckRecord) string {
	city := strings.Split(record.City, record.Name)[0]
	if city == "" {
		city = "–"
	}
	return city
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

// TableData represents table structure for formatting
type TableData struct {
	Headers []string
	Rows    [][]string
}

// createBoxDrawingTable creates a box drawing table from table data
func (req *DeepCheckCallbackRequest) createBoxDrawingTable(data *TableData) string {
	if len(data.Headers) == 0 || len(data.Rows) == 0 {
		return ""
	}

	// Calculate column widths
	columnWidths := make([]int, len(data.Headers))
	for i, header := range data.Headers {
		columnWidths[i] = len(header)
	}

	for _, row := range data.Rows {
		for i, cell := range row {
			if i < len(columnWidths) && len(cell) > columnWidths[i] {
				columnWidths[i] = len(cell)
			}
		}
	}

	var result strings.Builder

	// Top line
	result.WriteString("┌")
	for i, width := range columnWidths {
		result.WriteString(strings.Repeat("─", width+2))
		if i < len(columnWidths)-1 {
			result.WriteString("┬")
		}
	}
	result.WriteString("┐\n")

	// Header row
	result.WriteString("│")
	for i, header := range data.Headers {
		result.WriteString(fmt.Sprintf(" %-*s │", columnWidths[i], header))
	}
	result.WriteString("\n")

	// Separator line
	result.WriteString("├")
	for i, width := range columnWidths {
		result.WriteString(strings.Repeat("─", width+2))
		if i < len(columnWidths)-1 {
			result.WriteString("┼")
		}
	}
	result.WriteString("┤\n")

	// Data rows
	for _, row := range data.Rows {
		result.WriteString("│")
		for i, cell := range row {
			if i < len(columnWidths) {
				result.WriteString(fmt.Sprintf(" %-*s │", columnWidths[i], cell))
			}
		}
		result.WriteString("\n")
	}

	// Bottom line
	result.WriteString("└")
	for i, width := range columnWidths {
		result.WriteString(strings.Repeat("─", width+2))
		if i < len(columnWidths)-1 {
			result.WriteString("┴")
		}
	}
	result.WriteString("┘")

	return result.String()
}

// createTableDataForNormalResults creates table data for normal results
func (req *DeepCheckCallbackRequest) createTableDataForNormalResults() *TableData {
	headers := []string{"省份", "城市", "電訊商", "響應時間", "狀態碼"}
	var rows [][]string

	for _, record := range req.Records {
		if record.IsHealthy() {
			city := req.extractCityName(record)
			responseTime := fmt.Sprintf("%dms", record.GetResponseTimeMs())
			if record.HTTPCode == 0 && record.GetResponseTimeMs() > 10000 {
				responseTime = "超時"
			}

			rows = append(rows, []string{
				record.RegionName,
				city,
				record.ISP,
				responseTime,
				fmt.Sprintf("%d", record.HTTPCode),
			})
		}
	}

	return &TableData{Headers: headers, Rows: rows}
}

// createTableDataForErrorResults creates table data for error results
func (req *DeepCheckCallbackRequest) createTableDataForErrorResults() *TableData {
	headers := []string{"省份", "城市", "電訊商", "響應時間", "狀態碼", "描述"}
	var rows [][]string

	for _, record := range req.Records {
		if !record.IsHealthy() {
			city := req.extractCityName(record)
			responseTime := fmt.Sprintf("%dms", record.GetResponseTimeMs())
			if record.HTTPCode == 0 || record.GetResponseTimeMs() > 10000 {
				responseTime = "超時"
			}

			rows = append(rows, []string{
				record.RegionName,
				city,
				record.ISP,
				responseTime,
				fmt.Sprintf("%d", record.HTTPCode),
				record.GetStatusDescription(),
			})
		}
	}

	return &TableData{Headers: headers, Rows: rows}
}

// createTableDataForAllResults creates table data for all results
func (req *DeepCheckCallbackRequest) createTableDataForAllResults() *TableData {
	headers := []string{"省份", "城市", "電訊商", "響應時間", "狀態碼", "問題描述"}
	var rows [][]string

	for _, record := range req.Records {
		city := req.extractCityName(record)
		responseTime := fmt.Sprintf("%dms", record.GetResponseTimeMs())
		if record.HTTPCode == 0 || record.GetResponseTimeMs() > 10000 {
			responseTime = "超時"
		}

		rows = append(rows, []string{
			record.RegionName,
			city,
			record.ISP,
			responseTime,
			fmt.Sprintf("%d", record.HTTPCode),
			record.GetStatusDescription(),
		})
	}

	return &TableData{Headers: headers, Rows: rows}
}

// splitTableByMessageLength splits table data into chunks that fit within message length
func (req *DeepCheckCallbackRequest) splitTableByMessageLength(data *TableData, maxLength int, prefix string) []string {
	if len(data.Rows) == 0 {
		return []string{}
	}

	var messages []string
	currentRows := [][]string{}

	for _, row := range data.Rows {
		// Test with current rows + new row
		testData := &TableData{
			Headers: data.Headers,
			Rows:    append(currentRows, row),
		}

		testTable := req.createBoxDrawingTable(testData)
		testMessage := prefix + "```\n" + testTable + "\n```"

		if len(testMessage) > maxLength && len(currentRows) > 0 {
			// Current message is full, create it and start new one
			currentData := &TableData{
				Headers: data.Headers,
				Rows:    currentRows,
			}
			currentTable := req.createBoxDrawingTable(currentData)
			messages = append(messages, prefix+"```\n"+currentTable+"\n```")

			// Start new message with current row
			currentRows = [][]string{row}
		} else {
			// Add row to current message
			currentRows = append(currentRows, row)
		}
	}

	// Add remaining rows if any
	if len(currentRows) > 0 {
		currentData := &TableData{
			Headers: data.Headers,
			Rows:    currentRows,
		}
		currentTable := req.createBoxDrawingTable(currentData)
		messages = append(messages, prefix+"```\n"+currentTable+"\n```")
	}

	return messages
}
