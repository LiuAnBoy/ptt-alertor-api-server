package account

import (
	"errors"
	"time"
)

var (
	ErrEmailExists     = errors.New("email already exists")
	ErrInvalidPassword = errors.New("invalid password")
	ErrAccountNotFound = errors.New("account not found")
	ErrAccountDisabled = errors.New("account is disabled")
)

// Account represents a user account
type Account struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Role      string    `json:"role"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
