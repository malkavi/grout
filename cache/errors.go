package cache

import (
	"errors"
	"fmt"
)

// Sentinel errors for cache operations
var (
	ErrNotInitialized  = errors.New("cache manager not initialized")
	ErrCacheMiss       = errors.New("cache miss")
	ErrDBClosed        = errors.New("database connection closed")
	ErrInvalidCacheKey = errors.New("invalid cache key")
)

// CacheError provides detailed error information for cache operations
type CacheError struct {
	Op        string // Operation name: "get", "save", "delete", etc.
	Key       string // Cache key if applicable
	CacheType string // "platform", "collection", "rom_id", "artwork"
	Err       error  // Underlying error
}

func (e *CacheError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("cache %s [%s:%s]: %v", e.Op, e.CacheType, e.Key, e.Err)
	}
	if e.CacheType != "" {
		return fmt.Sprintf("cache %s [%s]: %v", e.Op, e.CacheType, e.Err)
	}
	return fmt.Sprintf("cache %s: %v", e.Op, e.Err)
}

func (e *CacheError) Unwrap() error {
	return e.Err
}

// newCacheError creates a new CacheError
func newCacheError(op, cacheType, key string, err error) *CacheError {
	return &CacheError{
		Op:        op,
		Key:       key,
		CacheType: cacheType,
		Err:       err,
	}
}
