package service

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"domain-detection-go/internal/deepcheck"
	"domain-detection-go/pkg/model"

	"github.com/jmoiron/sqlx"
)

// DeepCheckService handles deep check order management
type DeepCheckService struct {
	db *sqlx.DB
}

// NewDeepCheckService creates a new deep check service
func NewDeepCheckService(db *sqlx.DB) *DeepCheckService {
	return &DeepCheckService{
		db: db,
	}
}

// CreateDeepCheckOrder creates a new deep check order record
func (s *DeepCheckService) CreateDeepCheckOrder(orderID string, userID, domainID int, domainName string) error {
	_, err := s.db.Exec(`
        INSERT INTO deep_check_orders (order_id, user_id, domain_id, domain_name, status, created_at)
        VALUES ($1, $2, $3, $4, 'pending', NOW())
    `, orderID, userID, domainID, domainName)

	if err != nil {
		log.Printf("Failed to create deep check order record: %v", err)
		return fmt.Errorf("failed to create deep check order: %w", err)
	}

	log.Printf("Created deep check order record: OrderID=%s, UserID=%d, DomainID=%d, Domain=%s",
		orderID, userID, domainID, domainName)

	return nil
}

// GetDeepCheckOrderByOrderID retrieves a deep check order by order ID
func (s *DeepCheckService) GetDeepCheckOrderByOrderID(orderID string) (*model.DeepCheckOrder, error) {
	var order model.DeepCheckOrder

	err := s.db.Get(&order, `
        SELECT id, order_id, user_id, domain_id, domain_name, status, 
               created_at, completed_at, callback_received, callback_data
        FROM deep_check_orders 
        WHERE order_id = $1
    `, orderID)

	if err != nil {
		return nil, fmt.Errorf("failed to get deep check order: %w", err)
	}

	return &order, nil
}

// UpdateDeepCheckOrderCallback updates the order with callback data
func (s *DeepCheckService) UpdateDeepCheckOrderCallback(orderID string, callback *deepcheck.DeepCheckCallbackRequest) error {
	// Convert callback to JSON for storage
	callbackJSON, err := json.Marshal(callback)
	if err != nil {
		return fmt.Errorf("failed to marshal callback data: %w", err)
	}

	var callbackData model.CallbackData
	if err := json.Unmarshal(callbackJSON, &callbackData); err != nil {
		return fmt.Errorf("failed to convert callback data: %w", err)
	}

	_, err = s.db.Exec(`
        UPDATE deep_check_orders 
        SET status = 'completed', 
            completed_at = NOW(), 
            callback_received = true, 
            callback_data = $1
        WHERE order_id = $2
    `, callbackData, orderID)

	if err != nil {
		log.Printf("Failed to update deep check order callback: %v", err)
		return fmt.Errorf("failed to update deep check order: %w", err)
	}

	log.Printf("Updated deep check order with callback: OrderID=%s", orderID)
	return nil
}

// GetPendingDeepCheckOrders gets all pending orders (for cleanup/monitoring)
func (s *DeepCheckService) GetPendingDeepCheckOrders(olderThanMinutes int) ([]model.DeepCheckOrder, error) {
	var orders []model.DeepCheckOrder

	cutoffTime := time.Now().Add(-time.Duration(olderThanMinutes) * time.Minute)

	err := s.db.Select(&orders, `
        SELECT id, order_id, user_id, domain_id, domain_name, status, 
               created_at, completed_at, callback_received, callback_data
        FROM deep_check_orders 
        WHERE status = 'pending' AND created_at < $1
        ORDER BY created_at ASC
    `, cutoffTime)

	return orders, err
}

// MarkDeepCheckOrderFailed marks an order as failed
func (s *DeepCheckService) MarkDeepCheckOrderFailed(orderID string, reason string) error {
	_, err := s.db.Exec(`
        UPDATE deep_check_orders 
        SET status = 'failed', 
            completed_at = NOW(),
            callback_data = jsonb_build_object('error', $1)
        WHERE order_id = $2
    `, reason, orderID)

	return err
}
