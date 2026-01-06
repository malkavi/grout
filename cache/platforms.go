package cache

import (
	"database/sql"
	"encoding/json"
	"grout/romm"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

// GetPlatforms retrieves all cached platforms
func (cm *CacheManager) GetPlatforms() ([]romm.Platform, error) {
	if cm == nil || !cm.initialized {
		return nil, ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	rows, err := cm.db.Query(`
		SELECT data_json FROM platforms ORDER BY name
	`)
	if err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "platforms", "", err)
	}
	defer rows.Close()

	var platforms []romm.Platform
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "platforms", "", err)
		}

		var platform romm.Platform
		if err := json.Unmarshal([]byte(dataJSON), &platform); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "platforms", "", err)
		}
		platforms = append(platforms, platform)
	}

	if err := rows.Err(); err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "platforms", "", err)
	}

	if len(platforms) > 0 {
		cm.stats.recordHit()
	} else {
		cm.stats.recordMiss()
	}

	return platforms, nil
}

// SavePlatforms saves platform data to the cache
func (cm *CacheManager) SavePlatforms(platforms []romm.Platform) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	logger := gaba.GetLogger()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	tx, err := cm.db.Begin()
	if err != nil {
		return newCacheError("save", "platforms", "", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO platforms
		(id, slug, fs_slug, name, custom_name, rom_count, has_bios, data_json, updated_at, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return newCacheError("save", "platforms", "", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, p := range platforms {
		dataJSON, err := json.Marshal(p)
		if err != nil {
			return newCacheError("save", "platforms", p.Slug, err)
		}

		hasBIOS := 0
		if p.HasBIOS {
			hasBIOS = 1
		}

		_, err = stmt.Exec(
			p.ID,
			p.Slug,
			p.FSSlug,
			p.Name,
			p.CustomName,
			p.ROMCount,
			hasBIOS,
			string(dataJSON),
			p.UpdatedAt,
			now,
		)
		if err != nil {
			return newCacheError("save", "platforms", p.Slug, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return newCacheError("save", "platforms", "", err)
	}

	logger.Debug("Saved platforms to cache", "count", len(platforms))
	return nil
}

// GetPlatformByID retrieves a single platform by ID
func (cm *CacheManager) GetPlatformByID(platformID int) (romm.Platform, error) {
	if cm == nil || !cm.initialized {
		return romm.Platform{}, ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var dataJSON string
	err := cm.db.QueryRow(`
		SELECT data_json FROM platforms WHERE id = ?
	`, platformID).Scan(&dataJSON)

	if err == sql.ErrNoRows {
		cm.stats.recordMiss()
		return romm.Platform{}, ErrCacheMiss
	}
	if err != nil {
		cm.stats.recordError()
		return romm.Platform{}, newCacheError("get", "platforms", "", err)
	}

	var platform romm.Platform
	if err := json.Unmarshal([]byte(dataJSON), &platform); err != nil {
		cm.stats.recordError()
		return romm.Platform{}, newCacheError("get", "platforms", "", err)
	}

	cm.stats.recordHit()
	return platform, nil
}

// HasBIOS returns whether a platform has BIOS files available
func (cm *CacheManager) HasBIOS(platformID int) (bool, bool) {
	if cm == nil || !cm.initialized {
		return false, false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var hasBIOS int
	err := cm.db.QueryRow(`
		SELECT has_bios FROM bios_availability WHERE platform_id = ?
	`, platformID).Scan(&hasBIOS)

	if err == sql.ErrNoRows {
		return false, false
	}
	if err != nil {
		return false, false
	}

	return hasBIOS == 1, true
}

// SetBIOSAvailability sets whether a platform has BIOS files
func (cm *CacheManager) SetBIOSAvailability(platformID int, hasBIOS bool) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	biosInt := 0
	if hasBIOS {
		biosInt = 1
	}

	_, err := cm.db.Exec(`
		INSERT OR REPLACE INTO bios_availability (platform_id, has_bios, checked_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, platformID, biosInt)

	if err != nil {
		return newCacheError("save", "bios", "", err)
	}

	return nil
}
