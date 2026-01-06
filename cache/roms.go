package cache

import (
	"database/sql"
	"path/filepath"
	"strings"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

// stripExtension removes the file extension from a filename
func stripExtension(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

// GetRomIDByFilename looks up a ROM ID by platform and filename
// Returns (romID, romName, found)
func (cm *CacheManager) GetRomIDByFilename(fsSlug, filename string) (int, string, bool) {
	if cm == nil || !cm.initialized {
		return 0, "", false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := stripExtension(filename)

	var romID int
	var romName string
	err := cm.db.QueryRow(`
		SELECT rom_id, rom_name FROM rom_id_cache
		WHERE platform_fs_slug = ? AND filename_key = ?
	`, fsSlug, key).Scan(&romID, &romName)

	if err == sql.ErrNoRows {
		cm.stats.recordMiss()
		return 0, "", false
	}
	if err != nil {
		cm.stats.recordError()
		return 0, "", false
	}

	cm.stats.recordHit()
	return romID, romName, true
}

// StoreRomID stores a ROM ID mapping for a platform and filename
func (cm *CacheManager) StoreRomID(fsSlug, filename string, romID int, romName string) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	logger := gaba.GetLogger()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	key := stripExtension(filename)

	_, err := cm.db.Exec(`
		INSERT OR REPLACE INTO rom_id_cache
		(platform_fs_slug, filename_key, rom_id, rom_name, cached_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, fsSlug, key, romID, romName)

	if err != nil {
		logger.Debug("Failed to store ROM ID", "fsSlug", fsSlug, "filename", filename, "error", err)
		return newCacheError("save", "rom_id", key, err)
	}

	logger.Debug("Stored ROM ID mapping", "fsSlug", fsSlug, "filename", filename, "romID", romID)
	return nil
}

// ClearRomIDCache clears all ROM ID mappings
func (cm *CacheManager) ClearRomIDCache() error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	_, err := cm.db.Exec(`DELETE FROM rom_id_cache`)
	if err != nil {
		return newCacheError("clear", "rom_id", "", err)
	}

	return nil
}

// GetRomIDCacheCount returns the number of cached ROM ID mappings
func (cm *CacheManager) GetRomIDCacheCount() int {
	if cm == nil || !cm.initialized {
		return 0
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	cm.db.QueryRow(`SELECT COUNT(*) FROM rom_id_cache`).Scan(&count)
	return count
}

// Compatibility functions for existing code

// GetCachedRomIDByFilename looks up a ROM ID (compatibility wrapper)
func GetCachedRomIDByFilename(fsSlug, filename string) (int, string, bool) {
	cm := GetCacheManager()
	if cm == nil {
		return 0, "", false
	}
	return cm.GetRomIDByFilename(fsSlug, filename)
}

// StoreRomID stores a ROM ID mapping (compatibility wrapper)
func StoreRomID(fsSlug, filename string, romID int, romName string) {
	cm := GetCacheManager()
	if cm == nil {
		return
	}
	_ = cm.StoreRomID(fsSlug, filename, romID, romName)
}

// ClearRomCache clears the ROM ID cache (compatibility wrapper)
func ClearRomCache() error {
	cm := GetCacheManager()
	if cm == nil {
		return nil
	}
	return cm.ClearRomIDCache()
}

// HasRomCache checks if ROM cache has data (compatibility wrapper)
func HasRomCache() bool {
	cm := GetCacheManager()
	if cm == nil {
		return false
	}
	return cm.GetRomIDCacheCount() > 0
}

// GetRomCacheDir returns a placeholder path (compatibility wrapper)
// Note: ROM cache is now in SQLite, this is kept for compatibility
func GetRomCacheDir() string {
	return GetArtworkCacheDir()
}
