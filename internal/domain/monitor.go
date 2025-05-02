package domain

// MonitorClient defines the interface for domain monitoring operations
type MonitorClient interface {
	// CreateMonitor creates a monitor for a domain and returns the monitor GUID
	CreateMonitor(domain string, name string, region string) (string, error)

	// UpdateMonitorStatus updates the active status of a monitor
	UpdateMonitorStatus(monitorGuid string, isActive bool) error

	// DeleteMonitor deletes a monitor in the monitoring service
	DeleteMonitor(monitorGuid string) error
}
