package binding

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

const (
	ServiceTelegram = "telegram"
	ServiceLine     = "line"
	ServiceDiscord  = "discord"
)

var (
	ErrBindingNotFound   = errors.New("binding not found")
	ErrBindingExists     = errors.New("binding already exists")
	ErrInvalidBindCode   = errors.New("invalid or expired bind code")
	ErrServiceIDExists   = errors.New("service id already bound to another account")
)

// NotificationBinding represents a notification service binding
type NotificationBinding struct {
	ID                int        `json:"id"`
	UserID            int        `json:"user_id"`
	Service           string     `json:"service"`
	ServiceID         string     `json:"service_id"`
	BindCode          *string    `json:"-"`
	BindCodeExpiresAt *time.Time `json:"-"`
	Enabled           bool       `json:"enabled"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// GenerateBindCode generates a random bind code
func GenerateBindCode() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// IsBindCodeValid checks if bind code is valid and not expired
func (b *NotificationBinding) IsBindCodeValid(code string) bool {
	if b.BindCode == nil || b.BindCodeExpiresAt == nil {
		return false
	}
	if *b.BindCode != code {
		return false
	}
	if time.Now().After(*b.BindCodeExpiresAt) {
		return false
	}
	return true
}

// Repository interface for notification bindings
type Repository interface {
	// Create creates a new binding
	Create(userID int, service, serviceID string) (*NotificationBinding, error)

	// FindByUserAndService finds binding by user ID and service type
	FindByUserAndService(userID int, service string) (*NotificationBinding, error)

	// FindByServiceID finds binding by service type and service ID
	FindByServiceID(service, serviceID string) (*NotificationBinding, error)

	// FindAllByUser finds all bindings for a user
	FindAllByUser(userID int) ([]*NotificationBinding, error)

	// UpdateBindCode updates the bind code for pending binding
	UpdateBindCode(userID int, service, code string, expiresAt time.Time) error

	// ConfirmBinding confirms binding with service ID
	ConfirmBinding(userID int, service, serviceID string) error

	// Delete deletes a binding
	Delete(userID int, service string) error

	// SetEnabled enables or disables a binding
	SetEnabled(userID int, service string, enabled bool) error

	// FindByBindCode finds binding by bind code
	FindByBindCode(service, code string) (*NotificationBinding, error)
}
