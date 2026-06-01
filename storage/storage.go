package storage

import "context"

type Provider interface {
	Name() string
	Authenticate(ctx context.Context) error
	Upload(ctx context.Context, key string, data []byte) error
	Download(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	// List returns all keys under the given prefix.
	// Implementations should return only the key basename, not the full path.
	List(ctx context.Context, prefix string) ([]string, error)
}
