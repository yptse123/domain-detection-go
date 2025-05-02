package handler

import (
	"domain-detection-go/internal/monitor"
)

// MonitorHandler handles domain monitoring requests
type MonitorHandler struct {
	monitorService *monitor.MonitorService
}

// NewMonitorHandler creates a new monitor handler
func NewMonitorHandler(monitorService *monitor.MonitorService) *MonitorHandler {
	return &MonitorHandler{
		monitorService: monitorService,
	}
}
