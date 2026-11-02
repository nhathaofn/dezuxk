package sqlite

import (
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/security"
)

// ProvidersRepository implements providers.Repository using SQLite and an injected Cipher.
type ProvidersRepository struct {
	db     *DB
	cipher security.Cipher
}

var _ providers.Repository = (*ProvidersRepository)(nil)

// NewProvidersRepository creates a new ProvidersRepository.
func NewProvidersRepository(db *DB, cipher security.Cipher) *ProvidersRepository {
	return &ProvidersRepository{
		db:     db,
		cipher: cipher,
	}
}
