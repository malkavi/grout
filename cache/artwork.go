package cache

import (
	"database/sql"
	"grout/internal/fileutil"
	"image/png"
	"os"
	"path/filepath"
	"strconv"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

func GetArtworkCachePath(fsSlug string, romID int) string {
	return filepath.Join(GetArtworkCacheDir(), fsSlug, strconv.Itoa(romID)+".png")
}

func (cm *CacheManager) IsArtworkCached(fsSlug string, romID int) bool {
	if cm == nil || !cm.initialized {
		// Fallback to file check
		return fileutil.FileExists(GetArtworkCachePath(fsSlug, romID))
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	err := cm.db.QueryRow(`
		SELECT COUNT(*) FROM artwork_metadata
		WHERE platform_fs_slug = ? AND rom_id = ?
	`, fsSlug, romID).Scan(&count)

	if err != nil || count == 0 {
		// No metadata, but check if file exists (for legacy compatibility)
		return fileutil.FileExists(GetArtworkCachePath(fsSlug, romID))
	}

	return fileutil.FileExists(GetArtworkCachePath(fsSlug, romID))
}

func (cm *CacheManager) GetArtworkPath(fsSlug string, romID int) string {
	return GetArtworkCachePath(fsSlug, romID)
}

func (cm *CacheManager) MarkArtworkCached(fsSlug string, romID int, filePath string) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	logger := gaba.GetLogger()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	var size int64
	if info, err := os.Stat(filePath); err == nil {
		size = info.Size()
	}

	_, err := cm.db.Exec(`
		INSERT OR REPLACE INTO artwork_metadata
		(platform_fs_slug, rom_id, file_path, file_size_bytes, cached_at, validated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, fsSlug, romID, filePath, size)

	if err != nil {
		logger.Debug("Failed to mark artwork cached", "fsSlug", fsSlug, "romID", romID, "error", err)
		return newCacheError("save", "artwork", strconv.Itoa(romID), err)
	}

	return nil
}

func (cm *CacheManager) RemoveArtworkMetadata(fsSlug string, romID int) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	_, err := cm.db.Exec(`
		DELETE FROM artwork_metadata
		WHERE platform_fs_slug = ? AND rom_id = ?
	`, fsSlug, romID)

	if err != nil {
		return newCacheError("delete", "artwork", strconv.Itoa(romID), err)
	}

	return nil
}

func (cm *CacheManager) ValidateArtworkCache() (int, error) {
	if cm == nil || !cm.initialized {
		return 0, ErrNotInitialized
	}

	logger := gaba.GetLogger()

	cm.mu.RLock()
	rows, err := cm.db.Query(`
		SELECT platform_fs_slug, rom_id, file_path FROM artwork_metadata
	`)
	cm.mu.RUnlock()

	if err != nil {
		return 0, newCacheError("validate", "artwork", "", err)
	}
	defer rows.Close()

	type toRemove struct {
		fsSlug string
		romID  int
		path   string
	}

	var removeList []toRemove

	for rows.Next() {
		var fsSlug string
		var romID int
		var filePath string

		if err := rows.Scan(&fsSlug, &romID, &filePath); err != nil {
			continue
		}

		if !fileutil.FileExists(filePath) {
			removeList = append(removeList, toRemove{fsSlug, romID, filePath})
			continue
		}

		if !isValidPNG(filePath) {
			removeList = append(removeList, toRemove{fsSlug, romID, filePath})
		}
	}

	cm.mu.Lock()
	for _, item := range removeList {
		cm.db.Exec(`
			DELETE FROM artwork_metadata
			WHERE platform_fs_slug = ? AND rom_id = ?
		`, item.fsSlug, item.romID)

		os.Remove(item.path)
	}
	cm.mu.Unlock()

	if len(removeList) > 0 {
		logger.Debug("Removed invalid artwork entries", "count", len(removeList))
	}

	return len(removeList), nil
}

func (cm *CacheManager) GetArtworkCount() int {
	if cm == nil || !cm.initialized {
		return 0
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	cm.db.QueryRow(`SELECT COUNT(*) FROM artwork_metadata`).Scan(&count)
	return count
}

func EnsureArtworkCacheDir(fsSlug string) error {
	dir := filepath.Join(GetArtworkCacheDir(), fsSlug)
	return os.MkdirAll(dir, 0755)
}

func isValidPNG(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	_, err = png.DecodeConfig(f)
	return err == nil
}

func (cm *CacheManager) ScanAndIndexArtwork() (int, error) {
	if cm == nil || !cm.initialized {
		return 0, ErrNotInitialized
	}

	logger := gaba.GetLogger()
	cacheDir := GetArtworkCacheDir()

	platformDirs, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, newCacheError("scan", "artwork", "", err)
	}

	indexed := 0
	for _, platformDir := range platformDirs {
		if !platformDir.IsDir() {
			continue
		}

		fsSlug := platformDir.Name()
		platformPath := filepath.Join(cacheDir, fsSlug)

		files, err := os.ReadDir(platformPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".png" {
				continue
			}

			name := file.Name()
			romIDStr := name[:len(name)-4] // Remove ".png"
			romID, err := strconv.Atoi(romIDStr)
			if err != nil {
				continue
			}

			filePath := filepath.Join(platformPath, file.Name())

			cm.mu.RLock()
			var count int
			cm.db.QueryRow(`
				SELECT COUNT(*) FROM artwork_metadata
				WHERE platform_fs_slug = ? AND rom_id = ?
			`, fsSlug, romID).Scan(&count)
			cm.mu.RUnlock()

			if count == 0 {
				cm.MarkArtworkCached(fsSlug, romID, filePath)
				indexed++
			}
		}
	}

	if indexed > 0 {
		logger.Debug("Indexed existing artwork files", "count", indexed)
	}

	return indexed, nil
}

func (cm *CacheManager) GetAllArtworkMetadata() ([]struct {
	FSSlug string
	RomID  int
	Path   string
}, error) {
	if cm == nil || !cm.initialized {
		return nil, ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	rows, err := cm.db.Query(`
		SELECT platform_fs_slug, rom_id, file_path FROM artwork_metadata
	`)
	if err != nil {
		return nil, newCacheError("get", "artwork", "", err)
	}
	defer rows.Close()

	var results []struct {
		FSSlug string
		RomID  int
		Path   string
	}

	for rows.Next() {
		var fsSlug string
		var romID int
		var path string

		if err := rows.Scan(&fsSlug, &romID, &path); err != nil {
			continue
		}

		results = append(results, struct {
			FSSlug string
			RomID  int
			Path   string
		}{fsSlug, romID, path})
	}

	return results, nil
}

func (cm *CacheManager) HasArtworkByFilename(fsSlug string) bool {
	if cm == nil || !cm.initialized {
		return false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	err := cm.db.QueryRow(`
		SELECT COUNT(*) FROM artwork_metadata WHERE platform_fs_slug = ?
	`, fsSlug).Scan(&count)

	if err != nil {
		// Fallback to checking directory
		dir := filepath.Join(GetArtworkCacheDir(), fsSlug)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		return len(entries) > 0
	}

	return count > 0
}

func GetGameByIDForArtwork(gameID int) (struct {
	PlatformFSSlug string
}, error) {
	cm := GetCacheManager()
	if cm == nil {
		return struct{ PlatformFSSlug string }{}, ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var platformFSSlug string
	err := cm.db.QueryRow(`
		SELECT platform_fs_slug FROM games WHERE id = ?
	`, gameID).Scan(&platformFSSlug)

	if err == sql.ErrNoRows {
		return struct{ PlatformFSSlug string }{}, ErrCacheMiss
	}
	if err != nil {
		return struct{ PlatformFSSlug string }{}, err
	}

	return struct{ PlatformFSSlug string }{platformFSSlug}, nil
}
