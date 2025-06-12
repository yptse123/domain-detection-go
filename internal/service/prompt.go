package service

import (
	"database/sql/driver"
	"domain-detection-go/pkg/model"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type TelegramPromptService struct {
	db *sqlx.DB
}

func NewTelegramPromptService(db *sqlx.DB) *TelegramPromptService {
	return &TelegramPromptService{db: db}
}

// MessagesMap is a custom type for handling JSONB
type MessagesMap map[string]string

// Scan implements the sql.Scanner interface
func (m *MessagesMap) Scan(value interface{}) error {
	if value == nil {
		*m = make(map[string]string)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into MessagesMap", value)
	}

	return json.Unmarshal(bytes, m)
}

// Value implements the driver.Valuer interface
func (m MessagesMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	return json.Marshal(m)
}

// GetPrompts with pagination
func (s *TelegramPromptService) GetPrompts(page, perPage int, search, sortBy, sortOrder string) (*model.TelegramPromptResponse, error) {
	offset := (page - 1) * perPage

	// Build where clause
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if search != "" {
		whereClause += fmt.Sprintf(" AND (prompt_key ILIKE $%d OR description ILIKE $%d OR messages::text ILIKE $%d)", argIndex, argIndex, argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}

	// Validate sort fields
	validSorts := map[string]bool{"prompt_key": true, "description": true, "updated_at": true, "created_at": true}
	if !validSorts[sortBy] {
		sortBy = "updated_at"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM telegram_prompts %s", whereClause)
	var total int
	err := s.db.Get(&total, countQuery, args...)
	if err != nil {
		return nil, err
	}

	// Get prompts
	query := fmt.Sprintf(`
        SELECT id, prompt_key, description, messages, created_at, updated_at
        FROM telegram_prompts %s
        ORDER BY %s %s
        LIMIT $%d OFFSET $%d
    `, whereClause, sortBy, sortOrder, argIndex, argIndex+1)

	args = append(args, perPage, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prompts []model.TelegramPrompt
	for rows.Next() {
		var prompt model.TelegramPrompt
		var messages MessagesMap

		err := rows.Scan(&prompt.ID, &prompt.PromptKey, &prompt.Description, &messages, &prompt.CreatedAt, &prompt.UpdatedAt)
		if err != nil {
			return nil, err
		}

		prompt.Messages = map[string]string(messages)
		prompts = append(prompts, prompt)
	}

	totalPages := (total + perPage - 1) / perPage

	return &model.TelegramPromptResponse{
		Prompts:    prompts,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

// GetTranslation gets a specific message for a language
func (s *TelegramPromptService) GetTranslation(key, language string) (string, error) {
	var messages MessagesMap
	err := s.db.Get(&messages, `
        SELECT messages FROM telegram_prompts 
        WHERE prompt_key = $1
    `, key)
	if err != nil {
		return "", err
	}

	// Try to get the specific language
	if msg, exists := messages[language]; exists && msg != "" {
		return msg, nil
	}

	// Fallback to English
	if msg, exists := messages["en"]; exists && msg != "" {
		return msg, nil
	}

	// Return empty if nothing found
	return "", fmt.Errorf("no translation found for key %s", key)
}

// GetAllPromptsByLanguage gets all prompts with messages for a specific language
func (s *TelegramPromptService) GetAllPromptsByLanguage(language string) ([]model.TelegramPrompt, error) {
	rows, err := s.db.Query(`
        SELECT id, prompt_key, description, messages, created_at, updated_at
        FROM telegram_prompts
        ORDER BY prompt_key
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prompts []model.TelegramPrompt
	for rows.Next() {
		var prompt model.TelegramPrompt
		var messages MessagesMap

		err := rows.Scan(&prompt.ID, &prompt.PromptKey, &prompt.Description, &messages, &prompt.CreatedAt, &prompt.UpdatedAt)
		if err != nil {
			return nil, err
		}

		// Create a simplified prompt with only the requested language message
		prompt.Messages = make(map[string]string)
		if msg, exists := messages[language]; exists && msg != "" {
			prompt.Messages[language] = msg
		} else if msg, exists := messages["en"]; exists && msg != "" {
			// Fallback to English
			prompt.Messages[language] = msg
		}

		prompts = append(prompts, prompt)
	}

	return prompts, nil
}

// CreatePrompt creates a new prompt
func (s *TelegramPromptService) CreatePrompt(req model.TelegramPromptRequest) (*model.TelegramPrompt, error) {
	messages := req.ToMessages()
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	var prompt model.TelegramPrompt
	err = s.db.QueryRow(`
        INSERT INTO telegram_prompts (prompt_key, description, messages, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
        RETURNING id, prompt_key, description, messages, created_at, updated_at
    `, req.PromptKey, req.Description, messagesJSON).Scan(
		&prompt.ID, &prompt.PromptKey, &prompt.Description, (*MessagesMap)(&prompt.Messages), &prompt.CreatedAt, &prompt.UpdatedAt)

	return &prompt, err
}

// UpdatePrompt updates an existing prompt
func (s *TelegramPromptService) UpdatePrompt(id int, req model.TelegramPromptRequest) (*model.TelegramPrompt, error) {
	messages := req.ToMessages()
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	var prompt model.TelegramPrompt
	err = s.db.QueryRow(`
        UPDATE telegram_prompts 
        SET description = $1, messages = $2, updated_at = NOW()
        WHERE id = $3
        RETURNING id, prompt_key, description, messages, created_at, updated_at
    `, req.Description, messagesJSON, id).Scan(
		&prompt.ID, &prompt.PromptKey, &prompt.Description, (*MessagesMap)(&prompt.Messages), &prompt.CreatedAt, &prompt.UpdatedAt)

	return &prompt, err
}

// DeletePrompt deletes a prompt
func (s *TelegramPromptService) DeletePrompt(id int) error {
	_, err := s.db.Exec("DELETE FROM telegram_prompts WHERE id = $1", id)
	return err
}

// GetPromptByID gets a prompt by ID
func (s *TelegramPromptService) GetPromptByID(id int) (*model.TelegramPrompt, error) {
	var prompt model.TelegramPrompt
	var messages MessagesMap

	err := s.db.QueryRow(`
        SELECT id, prompt_key, description, messages, created_at, updated_at
        FROM telegram_prompts WHERE id = $1
    `, id).Scan(&prompt.ID, &prompt.PromptKey, &prompt.Description, &messages, &prompt.CreatedAt, &prompt.UpdatedAt)

	if err != nil {
		return nil, err
	}

	prompt.Messages = map[string]string(messages)
	return &prompt, nil
}
