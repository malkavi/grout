package artwork

import (
	"grout/cache"
	"grout/internal/fileutil"
	"os"
	"path/filepath"
	"strconv"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

func GetCacheDir() string {
	return cache.GetArtworkCacheDir()
}

func ClearCache() error {
	// Clear SQLite metadata
	if cm := cache.GetCacheManager(); cm != nil {
		if err := cm.ClearArtwork(); err != nil {
			gaba.GetLogger().Debug("Failed to clear artwork metadata", "error", err)
		}
	}

	// Clear files from disk
	cacheDir := GetCacheDir()
	if !fileutil.FileExists(cacheDir) {
		return nil
	}
	return os.RemoveAll(cacheDir)
}

func HasCache() bool {
	// Check SQLite first for speed
	if cm := cache.GetCacheManager(); cm != nil {
		if cm.GetArtworkCount() > 0 {
			return true
		}
	}

	// Fall back to filesystem check
	cacheDir := GetCacheDir()
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func GetCachePath(platformFSSlug string, romID int) string {
	return filepath.Join(GetCacheDir(), platformFSSlug, strconv.Itoa(romID)+".png")
}

func Exists(platformFSSlug string, romID int) bool {
	// Check SQLite metadata first (fast)
	if cm := cache.GetCacheManager(); cm != nil {
		if cm.IsArtworkCached(platformFSSlug, romID) {
			return true
		}
	}

	// Fall back to filesystem check
	return fileutil.FileExists(GetCachePath(platformFSSlug, romID))
}

func EnsureCacheDir(platformFSSlug string) error {
	dir := filepath.Join(GetCacheDir(), platformFSSlug)
	return os.MkdirAll(dir, 0755)
}

// MarkCached records that artwork has been cached for a ROM
func MarkCached(platformFSSlug string, romID int) {
	if cm := cache.GetCacheManager(); cm != nil {
		path := GetCachePath(platformFSSlug, romID)
		if err := cm.MarkArtworkCached(platformFSSlug, romID, path); err != nil {
			gaba.GetLogger().Debug("Failed to mark artwork cached", "error", err)
		}
	}
}

func ValidateCache() {
	if cm := cache.GetCacheManager(); cm != nil {
		go func() {
			removed, err := cm.ValidateArtworkCache()
			if err != nil {
				gaba.GetLogger().Debug("Failed to validate artwork cache", "error", err)
				return
			}
			if removed > 0 {
				gaba.GetLogger().Debug("Removed invalid artwork entries", "count", removed)
			}
		}()
	}
}
