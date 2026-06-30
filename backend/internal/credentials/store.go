package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("provider credential not found")

type Status struct {
	Provider   string     `json:"provider"`
	Configured bool       `json:"configured"`
	LastFour   string     `json:"lastFour,omitempty"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

type Store struct {
	pool *pgxpool.Pool
	aead cipher.AEAD
}

func NewStore(pool *pgxpool.Pool, encryptionSecret string) (*Store, error) {
	if strings.TrimSpace(encryptionSecret) == "" {
		return nil, fmt.Errorf("credential encryption secret is required")
	}
	key := sha256.Sum256([]byte(encryptionSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, aead: aead}, nil
}

func (s *Store) Upsert(ctx context.Context, organizationID uuid.UUID, subject, provider, secret string) (Status, error) {
	subject = strings.TrimSpace(subject)
	provider = strings.ToLower(strings.TrimSpace(provider))
	secret = strings.TrimSpace(secret)
	if subject == "" || provider == "" || len(secret) < 20 || strings.ContainsAny(secret, "\r\n\t ") {
		return Status{}, fmt.Errorf("invalid provider credential")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Status{}, err
	}
	aad := []byte(organizationID.String() + ":" + subject + ":" + provider)
	ciphertext := s.aead.Seal(nil, nonce, []byte(secret), aad)
	lastFour := secret[len(secret)-4:]
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO provider_credentials (
			organization_id, subject, provider, encrypted_secret, nonce, last_four
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (organization_id, subject, provider) DO UPDATE SET
			encrypted_secret=EXCLUDED.encrypted_secret,
			nonce=EXCLUDED.nonce,
			last_four=EXCLUDED.last_four,
			updated_at=now()
		RETURNING updated_at`, organizationID, subject, provider, ciphertext, nonce, lastFour).Scan(&updatedAt)
	if err != nil {
		return Status{}, err
	}
	return Status{Provider: provider, Configured: true, LastFour: lastFour, UpdatedAt: &updatedAt}, nil
}

func (s *Store) GetStatus(ctx context.Context, organizationID uuid.UUID, subject, provider string) (Status, error) {
	status := Status{Provider: provider}
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT last_four, updated_at
		FROM provider_credentials
		WHERE organization_id=$1 AND subject=$2 AND provider=$3`,
		organizationID, subject, provider).Scan(&status.LastFour, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	status.Configured = true
	status.UpdatedAt = &updatedAt
	return status, nil
}

func (s *Store) Resolve(ctx context.Context, organizationID uuid.UUID, subject, provider string) (string, error) {
	var ciphertext, nonce []byte
	err := s.pool.QueryRow(ctx, `
		SELECT encrypted_secret, nonce
		FROM provider_credentials
		WHERE organization_id=$1 AND subject=$2 AND provider=$3`,
		organizationID, subject, provider).Scan(&ciphertext, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	aad := []byte(organizationID.String() + ":" + subject + ":" + provider)
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", fmt.Errorf("decrypt provider credential: %w", err)
	}
	return string(plaintext), nil
}

func (s *Store) Delete(ctx context.Context, organizationID uuid.UUID, subject, provider string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM provider_credentials
		WHERE organization_id=$1 AND subject=$2 AND provider=$3`,
		organizationID, subject, provider)
	return err
}
