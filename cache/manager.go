package cache

import (
	"database/sql"
	"grout/internal/fileutil"
	"grout/romm"
	"os"
	"path/filepath"
	"sync"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"go.uber.org/atomic"
	_ "modernc.org/sqlite"
)

// CacheManager is the unified cache interface for all caching operations
type CacheManager struct {
	db          *sql.DB
	dbPath      string
	mu          sync.RWMutex
	host        romm.Host
	config      Config
	initialized bool

	// Stats tracking
	stats *CacheStats
}

// CacheStats tracks cache performance metrics
type CacheStats struct {
	mu         sync.Mutex
	Hits       int64
	Misses     int64
	Errors     int64
	LastAccess time.Time
}

func (s *CacheStats) recordHit() {
	s.mu.Lock()
	s.Hits++
	s.LastAccess = time.Now()
	s.mu.Unlock()
}

func (s *CacheStats) recordMiss() {
	s.mu.Lock()
	s.Misses++
	s.LastAccess = time.Now()
	s.mu.Unlock()
}

func (s *CacheStats) recordError() {
	s.mu.Lock()
	s.Errors++
	s.mu.Unlock()
}

var (
	cacheManager     *CacheManager
	cacheManagerOnce sync.Once
	cacheManagerErr  error
)

// GetCacheManager returns the singleton cache manager instance
// Returns nil if not yet initialized
func GetCacheManager() *CacheManager {
	return cacheManager
}

// InitCacheManager initializes the singleton cache manager
// This should be called once at application startup
func InitCacheManager(host romm.Host, config Config) error {
	cacheManagerOnce.Do(func() {
		cacheManager, cacheManagerErr = newCacheManager(host, config)
	})
	return cacheManagerErr
}

// newCacheManager creates a new CacheManager instance
func newCacheManager(host romm.Host, config Config) (*CacheManager, error) {
	logger := gaba.GetLogger()

	dbPath := getCacheDBPath()

	// Ensure cache directory exists
	cacheDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, newCacheError("init", "", "", err)
	}

	// Clean up old JSON cache directories if they exist
	cleanupLegacyCache()

	// Open SQLite database with WAL mode for better concurrent read performance
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, newCacheError("init", "", "", err)
	}

	// SQLite is single-writer, limit connections
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Create tables
	if err := createTables(db); err != nil {
		db.Close()
		return nil, newCacheError("init", "", "", err)
	}

	cm := &CacheManager{
		db:          db,
		dbPath:      dbPath,
		host:        host,
		config:      config,
		initialized: true,
		stats:       &CacheStats{},
	}

	logger.Info("Cache manager initialized", "path", dbPath)
	return cm, nil
}

// Close closes the database connection
func (cm *CacheManager) Close() error {
	if cm == nil || cm.db == nil {
		return nil
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.initialized = false
	return cm.db.Close()
}

// IsFirstRun returns true if the cache has no games (needs population)
func (cm *CacheManager) IsFirstRun() bool {
	if cm == nil || !cm.initialized {
		return true
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	err := cm.db.QueryRow("SELECT COUNT(*) FROM games").Scan(&count)
	if err != nil {
		return true
	}

	return count == 0
}

// HasCache returns true if the cache database exists and has data
func (cm *CacheManager) HasCache() bool {
	if cm == nil || !cm.initialized {
		return false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	err := cm.db.QueryRow("SELECT COUNT(*) FROM games").Scan(&count)
	return err == nil && count > 0
}

// Clear removes all cached data but keeps the database structure
func (cm *CacheManager) Clear() error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	logger := gaba.GetLogger()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	tables := []string{"games", "game_collections", "collections", "platforms", "rom_id_cache", "artwork_metadata", "bios_availability"}

	tx, err := cm.db.Begin()
	if err != nil {
		return newCacheError("clear", "", "", err)
	}
	defer tx.Rollback()

	for _, table := range tables {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return newCacheError("clear", table, "", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return newCacheError("clear", "", "", err)
	}

	// Also clear artwork files from disk
	artworkDir := GetArtworkCacheDir()
	if fileutil.FileExists(artworkDir) {
		os.RemoveAll(artworkDir)
	}

	logger.Info("Cache cleared")
	return nil
}

// ClearGames removes only the games cache
func (cm *CacheManager) ClearGames() error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	tx, err := cm.db.Begin()
	if err != nil {
		return newCacheError("clear_games", "", "", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM game_collections"); err != nil {
		return newCacheError("clear_games", "game_collections", "", err)
	}

	if _, err := tx.Exec("DELETE FROM games"); err != nil {
		return newCacheError("clear_games", "games", "", err)
	}

	return tx.Commit()
}

// ClearArtwork removes artwork metadata and files
func (cm *CacheManager) ClearArtwork() error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, err := cm.db.Exec("DELETE FROM artwork_metadata"); err != nil {
		return newCacheError("clear_artwork", "", "", err)
	}

	// Also clear artwork files from disk
	artworkDir := GetArtworkCacheDir()
	if fileutil.FileExists(artworkDir) {
		os.RemoveAll(artworkDir)
	}

	return nil
}

// ClearCollections removes collections and their game mappings
func (cm *CacheManager) ClearCollections() error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	tx, err := cm.db.Begin()
	if err != nil {
		return newCacheError("clear_collections", "", "", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM game_collections"); err != nil {
		return newCacheError("clear_collections", "game_collections", "", err)
	}

	if _, err := tx.Exec("DELETE FROM collections"); err != nil {
		return newCacheError("clear_collections", "collections", "", err)
	}

	return tx.Commit()
}

// HasCollections returns true if collections are cached
func (cm *CacheManager) HasCollections() bool {
	if cm == nil || !cm.initialized {
		return false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	err := cm.db.QueryRow("SELECT COUNT(*) FROM collections").Scan(&count)
	return err == nil && count > 0
}

// Cache refresh timestamp keys
const (
	MetaKeyGamesRefreshedAt       = "games_refreshed_at"
	MetaKeyCollectionsRefreshedAt = "collections_refreshed_at"
	MetaKeyArtworkRefreshedAt     = "artwork_refreshed_at"
)

// SetMetadata stores a key-value pair in the cache metadata table
func (cm *CacheManager) SetMetadata(key, value string) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	_, err := cm.db.Exec(`
		INSERT OR REPLACE INTO cache_metadata (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, key, value)
	if err != nil {
		return newCacheError("set_metadata", key, "", err)
	}

	return nil
}

// GetMetadata retrieves a value from the cache metadata table
func (cm *CacheManager) GetMetadata(key string) (string, error) {
	if cm == nil || !cm.initialized {
		return "", ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var value string
	err := cm.db.QueryRow(`SELECT value FROM cache_metadata WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", newCacheError("get_metadata", key, "", err)
	}

	return value, nil
}

// GetLastRefreshTime returns the last refresh time for a cache type
func (cm *CacheManager) GetLastRefreshTime(key string) (time.Time, error) {
	value, err := cm.GetMetadata(key)
	if err != nil {
		return time.Time{}, err
	}

	return time.Parse(time.RFC3339, value)
}

// RecordRefreshTime records the current time as the last refresh for a cache type
func (cm *CacheManager) RecordRefreshTime(key string) error {
	return cm.SetMetadata(key, time.Now().Format(time.RFC3339))
}

// GetAllRefreshTimes returns the last refresh times for all cache types
func (cm *CacheManager) GetAllRefreshTimes() map[string]time.Time {
	result := make(map[string]time.Time)

	keys := []string{MetaKeyGamesRefreshedAt, MetaKeyCollectionsRefreshedAt, MetaKeyArtworkRefreshedAt}
	for _, key := range keys {
		if t, err := cm.GetLastRefreshTime(key); err == nil {
			result[key] = t
		}
	}

	return result
}

// GetStats returns current cache statistics
func (cm *CacheManager) GetStats() CacheStats {
	if cm == nil || cm.stats == nil {
		return CacheStats{}
	}

	cm.stats.mu.Lock()
	defer cm.stats.mu.Unlock()

	return CacheStats{
		Hits:       cm.stats.Hits,
		Misses:     cm.stats.Misses,
		Errors:     cm.stats.Errors,
		LastAccess: cm.stats.LastAccess,
	}
}

// SetHost updates the host configuration (used after re-login)
func (cm *CacheManager) SetHost(host romm.Host) {
	if cm == nil {
		return
	}

	cm.mu.Lock()
	cm.host = host
	cm.mu.Unlock()
}

// PopulateFullCacheWithProgress populates the entire cache with progress reporting
func (cm *CacheManager) PopulateFullCacheWithProgress(platforms []romm.Platform, progress *atomic.Float64) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	return cm.populateCache(platforms, progress)
}

// Helper functions

// getCacheDBPath returns the path to the SQLite database file
func getCacheDBPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Join(os.TempDir(), ".cache", "grout.db")
	}
	return filepath.Join(wd, ".cache", "grout.db")
}

// GetArtworkCacheDir returns the path to the artwork cache directory
func GetArtworkCacheDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Join(os.TempDir(), ".cache", "artwork")
	}
	return filepath.Join(wd, ".cache", "artwork")
}

// cleanupLegacyCache removes old JSON cache directories
func cleanupLegacyCache() {
	logger := gaba.GetLogger()

	wd, err := os.Getwd()
	if err != nil {
		return
	}

	// Remove old games JSON cache
	gamesDir := filepath.Join(wd, ".cache", "games")
	if fileutil.FileExists(gamesDir) {
		if err := os.RemoveAll(gamesDir); err != nil {
			logger.Debug("Failed to remove legacy games cache", "error", err)
		} else {
			logger.Debug("Removed legacy games cache directory")
		}
	}

	// Remove old ROM ID JSON cache
	romsDir := filepath.Join(wd, ".cache", "roms")
	if fileutil.FileExists(romsDir) {
		if err := os.RemoveAll(romsDir); err != nil {
			logger.Debug("Failed to remove legacy roms cache", "error", err)
		} else {
			logger.Debug("Removed legacy roms cache directory")
		}
	}
}
