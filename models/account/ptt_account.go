package account

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/jackc/pgx/v5"
)

var (
	ErrPTTAccountNotFound = errors.New("ptt account not found")
	ErrPTTAccountExists   = errors.New("ptt account already exists")
)

// PTTAccount represents a user's PTT account binding
type PTTAccount struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	PTTUsername string    `json:"ptt_username"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PTTAccountPostgres is the PostgreSQL repository for PTT accounts
type PTTAccountPostgres struct{}

// getEncryptionKey returns the AES encryption key from environment
func getEncryptionKey() []byte {
	key := os.Getenv("PTT_ENCRYPT_KEY")
	if key == "" {
		key = os.Getenv("JWT_SECRET") // fallback to JWT secret
	}
	// Ensure key is 32 bytes for AES-256
	keyBytes := []byte(key)
	if len(keyBytes) < 32 {
		padded := make([]byte, 32)
		copy(padded, keyBytes)
		return padded
	}
	return keyBytes[:32]
}

// encrypt encrypts plaintext using AES-GCM
func encrypt(plaintext string) (string, error) {
	key := getEncryptionKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts ciphertext using AES-GCM
func decrypt(ciphertext string) (string, error) {
	key := getEncryptionKey()
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// Create creates a new PTT account binding
func (p *PTTAccountPostgres) Create(userID int, pttUsername, pttPassword string) (*PTTAccount, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	// Encrypt password
	encryptedPassword, err := encrypt(pttPassword)
	if err != nil {
		return nil, err
	}

	var acc PTTAccount
	err = pool.QueryRow(ctx, `
		INSERT INTO ptt_accounts (user_id, ptt_username, ptt_password_encrypted)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, ptt_username, created_at, updated_at
	`, userID, pttUsername, encryptedPassword).Scan(
		&acc.ID,
		&acc.UserID,
		&acc.PTTUsername,
		&acc.CreatedAt,
		&acc.UpdatedAt,
	)

	if err != nil {
		if err.Error() == "ERROR: duplicate key value violates unique constraint \"ptt_accounts_user_id_key\" (SQLSTATE 23505)" {
			return nil, ErrPTTAccountExists
		}
		return nil, err
	}

	return &acc, nil
}

// FindByUserID finds a PTT account by user ID
func (p *PTTAccountPostgres) FindByUserID(userID int) (*PTTAccount, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var acc PTTAccount
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, ptt_username, created_at, updated_at
		FROM ptt_accounts
		WHERE user_id = $1
	`, userID).Scan(
		&acc.ID,
		&acc.UserID,
		&acc.PTTUsername,
		&acc.CreatedAt,
		&acc.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPTTAccountNotFound
		}
		return nil, err
	}

	return &acc, nil
}

// GetCredentials retrieves the PTT username and decrypted password
func (p *PTTAccountPostgres) GetCredentials(userID int) (username, password string, err error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var encryptedPassword string
	err = pool.QueryRow(ctx, `
		SELECT ptt_username, ptt_password_encrypted
		FROM ptt_accounts
		WHERE user_id = $1
	`, userID).Scan(&username, &encryptedPassword)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrPTTAccountNotFound
		}
		return "", "", err
	}

	password, err = decrypt(encryptedPassword)
	if err != nil {
		return "", "", err
	}

	return username, password, nil
}

// Update updates a PTT account binding
func (p *PTTAccountPostgres) Update(userID int, pttUsername, pttPassword string) (*PTTAccount, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	// Encrypt password
	encryptedPassword, err := encrypt(pttPassword)
	if err != nil {
		return nil, err
	}

	var acc PTTAccount
	err = pool.QueryRow(ctx, `
		UPDATE ptt_accounts
		SET ptt_username = $2, ptt_password_encrypted = $3
		WHERE user_id = $1
		RETURNING id, user_id, ptt_username, created_at, updated_at
	`, userID, pttUsername, encryptedPassword).Scan(
		&acc.ID,
		&acc.UserID,
		&acc.PTTUsername,
		&acc.CreatedAt,
		&acc.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPTTAccountNotFound
		}
		return nil, err
	}

	return &acc, nil
}

// Delete deletes a PTT account binding
func (p *PTTAccountPostgres) Delete(userID int) error {
	ctx := context.Background()
	pool := connections.Postgres()

	result, err := pool.Exec(ctx, `
		DELETE FROM ptt_accounts WHERE user_id = $1
	`, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrPTTAccountNotFound
	}

	return nil
}

// Exists checks if a PTT account binding exists for a user
func (p *PTTAccountPostgres) Exists(userID int) (bool, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM ptt_accounts WHERE user_id = $1)
	`, userID).Scan(&exists)

	return exists, err
}
