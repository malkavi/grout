package cache

import (
	"database/sql"
	"grout/internal/stringutil"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

func (cm *Manager) GetRomIDByFilename(fsSlug, filename string) (int, string, bool) {
	if cm == nil || !cm.initialized {
		return 0, "", false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := stringutil.StripExtension(filename)

	var romID int
	var romName string
	err := cm.db.QueryRow(`
		SELECT id, name FROM games
		WHERE platform_fs_slug = ? AND fs_name_no_ext = ?
	`, fsSlug, key).Scan(&romID, &romName)

	if err == sql.ErrNoRows {
		cm.stats.recordMiss()
		return 0, "", false
	}
	if err != nil {
		cm.stats.recordError()
		gaba.GetLogger().Debug("ROM lookup error", "fsSlug", fsSlug, "filename", filename, "error", err)
		return 0, "", false
	}

	cm.stats.recordHit()
	return romID, romName, true
}

func (cm *Manager) GetRomByHash(md5, sha1, crc string) (int, string, bool) {
	if cm == nil || !cm.initialized {
		return 0, "", false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var romID int
	var romName string

	if md5 != "" {
		err := cm.db.QueryRow(`SELECT id, name FROM games WHERE md5_hash = ?`, md5).Scan(&romID, &romName)
		if err == nil {
			cm.stats.recordHit()
			return romID, romName, true
		}
	}

	if sha1 != "" {
		err := cm.db.QueryRow(`SELECT id, name FROM games WHERE sha1_hash = ?`, sha1).Scan(&romID, &romName)
		if err == nil {
			cm.stats.recordHit()
			return romID, romName, true
		}
	}

	if crc != "" {
		err := cm.db.QueryRow(`SELECT id, name FROM games WHERE crc_hash = ?`, crc).Scan(&romID, &romName)
		if err == nil {
			cm.stats.recordHit()
			return romID, romName, true
		}
	}

	cm.stats.recordMiss()
	return 0, "", false
}

func GetCachedRomIDByFilename(fsSlug, filename string) (int, string, bool) {
	cm := GetCacheManager()
	if cm == nil {
		return 0, "", false
	}
	return cm.GetRomIDByFilename(fsSlug, filename)
}
