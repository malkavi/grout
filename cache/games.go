package cache

import (
	"database/sql"
	"encoding/json"
	"grout/romm"
	"strconv"
	"strings"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

// Cache key types (kept for compatibility)
type Type string

const (
	Platform          Type = "platform"
	Collection        Type = "collection"
	SmartCollection   Type = "smart_collection"
	VirtualCollection Type = "virtual_collection"
)

// GetCacheKey generates a cache key (kept for compatibility with existing code)
func GetCacheKey(cacheType Type, id string) string {
	return string(cacheType) + "_" + id
}

// GetPlatformCacheKey generates a platform cache key
func GetPlatformCacheKey(platformID int) string {
	return GetCacheKey(Platform, strconv.Itoa(platformID))
}

// GetCollectionCacheKey generates a collection cache key
func GetCollectionCacheKey(collection romm.Collection) string {
	if collection.IsVirtual {
		return GetCacheKey(VirtualCollection, collection.VirtualID)
	}
	if collection.IsSmart {
		return GetCacheKey(SmartCollection, strconv.Itoa(collection.ID))
	}
	return GetCacheKey(Collection, strconv.Itoa(collection.ID))
}

// GetPlatformGames retrieves all games for a platform from cache
func (cm *CacheManager) GetPlatformGames(platformID int) ([]romm.Rom, error) {
	if cm == nil || !cm.initialized {
		return nil, ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	rows, err := cm.db.Query(`
		SELECT data_json FROM games WHERE platform_id = ? ORDER BY name
	`, platformID)
	if err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "games", GetPlatformCacheKey(platformID), err)
	}
	defer rows.Close()

	var games []romm.Rom
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "games", GetPlatformCacheKey(platformID), err)
		}

		var game romm.Rom
		if err := json.Unmarshal([]byte(dataJSON), &game); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "games", GetPlatformCacheKey(platformID), err)
		}
		games = append(games, game)
	}

	if err := rows.Err(); err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "games", GetPlatformCacheKey(platformID), err)
	}

	if len(games) > 0 {
		cm.stats.recordHit()
	} else {
		cm.stats.recordMiss()
	}

	return games, nil
}

// SavePlatformGames saves games for a platform to cache
func (cm *CacheManager) SavePlatformGames(platformID int, games []romm.Rom) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	logger := gaba.GetLogger()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	tx, err := cm.db.Begin()
	if err != nil {
		return newCacheError("save", "games", GetPlatformCacheKey(platformID), err)
	}
	defer tx.Rollback()

	// Delete existing games for this platform
	_, err = tx.Exec(`DELETE FROM games WHERE platform_id = ?`, platformID)
	if err != nil {
		return newCacheError("save", "games", GetPlatformCacheKey(platformID), err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO games (id, platform_id, platform_fs_slug, name, fs_name, data_json, updated_at, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return newCacheError("save", "games", GetPlatformCacheKey(platformID), err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, game := range games {
		dataJSON, err := json.Marshal(game)
		if err != nil {
			return newCacheError("save", "games", GetPlatformCacheKey(platformID), err)
		}

		_, err = stmt.Exec(
			game.ID,
			game.PlatformID,
			game.PlatformFSSlug,
			game.Name,
			game.FsName,
			string(dataJSON),
			game.UpdatedAt,
			now,
		)
		if err != nil {
			return newCacheError("save", "games", GetPlatformCacheKey(platformID), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return newCacheError("save", "games", GetPlatformCacheKey(platformID), err)
	}

	logger.Debug("Saved platform games to cache", "platformID", platformID, "count", len(games))
	return nil
}

// GetCollectionGames retrieves all games for a collection from cache
func (cm *CacheManager) GetCollectionGames(collection romm.Collection) ([]romm.Rom, error) {
	if cm == nil || !cm.initialized {
		return nil, ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Get the collection's internal ID
	collectionID, err := cm.getCollectionInternalID(collection)
	if err != nil {
		return nil, err
	}

	rows, err := cm.db.Query(`
		SELECT g.data_json FROM games g
		INNER JOIN game_collections gc ON g.id = gc.game_id
		WHERE gc.collection_id = ?
		ORDER BY g.name
	`, collectionID)
	if err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "games", GetCollectionCacheKey(collection), err)
	}
	defer rows.Close()

	var games []romm.Rom
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "games", GetCollectionCacheKey(collection), err)
		}

		var game romm.Rom
		if err := json.Unmarshal([]byte(dataJSON), &game); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "games", GetCollectionCacheKey(collection), err)
		}
		games = append(games, game)
	}

	if err := rows.Err(); err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "games", GetCollectionCacheKey(collection), err)
	}

	if len(games) > 0 {
		cm.stats.recordHit()
	} else {
		cm.stats.recordMiss()
	}

	return games, nil
}

// SaveCollectionGames saves game-collection mappings to cache
// Games should already exist from platform fetching; this only creates the mappings
func (cm *CacheManager) SaveCollectionGames(collection romm.Collection, games []romm.Rom) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	if len(games) == 0 {
		return nil
	}

	logger := gaba.GetLogger()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Get the collection's internal ID (collection should already be saved)
	collectionID, err := cm.getCollectionInternalIDLocked(collection)
	if err != nil {
		return err
	}

	tx, err := cm.db.Begin()
	if err != nil {
		return newCacheError("save", "games", GetCollectionCacheKey(collection), err)
	}
	defer tx.Rollback()

	// Delete existing game-collection mappings for this collection
	_, err = tx.Exec(`DELETE FROM game_collections WHERE collection_id = ?`, collectionID)
	if err != nil {
		return newCacheError("save", "games", GetCollectionCacheKey(collection), err)
	}

	// Batch insert mappings for better performance
	// SQLite supports up to 999 variables, so batch in groups
	const batchSize = 400 // 2 params per row = 800 variables max
	for i := 0; i < len(games); i += batchSize {
		end := i + batchSize
		if end > len(games) {
			end = len(games)
		}
		batch := games[i:end]

		// Build batch insert query
		query := "INSERT OR IGNORE INTO game_collections (game_id, collection_id) VALUES "
		args := make([]interface{}, 0, len(batch)*2)
		for j, game := range batch {
			if j > 0 {
				query += ", "
			}
			query += "(?, ?)"
			args = append(args, game.ID, collectionID)
		}

		if _, err := tx.Exec(query, args...); err != nil {
			return newCacheError("save", "games", GetCollectionCacheKey(collection), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return newCacheError("save", "games", GetCollectionCacheKey(collection), err)
	}

	logger.Debug("Saved collection game mappings", "collection", collection.Name, "count", len(games))
	return nil
}

// getCollectionInternalIDLocked gets collection ID without acquiring lock (caller must hold lock)
func (cm *CacheManager) getCollectionInternalIDLocked(collection romm.Collection) (int64, error) {
	var id int64
	var err error

	if collection.IsVirtual {
		err = cm.db.QueryRow(`SELECT id FROM collections WHERE virtual_id = ?`, collection.VirtualID).Scan(&id)
	} else {
		collType := "regular"
		if collection.IsSmart {
			collType = "smart"
		}
		err = cm.db.QueryRow(`SELECT id FROM collections WHERE romm_id = ? AND type = ?`, collection.ID, collType).Scan(&id)
	}

	if err == sql.ErrNoRows {
		cm.stats.recordMiss()
		return 0, ErrCacheMiss
	}
	if err != nil {
		cm.stats.recordError()
		return 0, newCacheError("get", "collections", GetCollectionCacheKey(collection), err)
	}

	return id, nil
}

// SaveAllCollectionMappings saves all game-collection mappings in a single transaction
// Uses ROMIDs from the collection response - no additional API calls needed
func (cm *CacheManager) SaveAllCollectionMappings(collections []romm.Collection) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	if len(collections) == 0 {
		return nil
	}

	logger := gaba.GetLogger()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	tx, err := cm.db.Begin()
	if err != nil {
		return newCacheError("save", "collection_mappings", "", err)
	}
	defer tx.Rollback()

	// Clear all existing mappings
	if _, err := tx.Exec(`DELETE FROM game_collections`); err != nil {
		return newCacheError("save", "collection_mappings", "", err)
	}

	// Build a map of collection identifiers to internal IDs
	collectionIDs := make(map[string]int64)
	for _, coll := range collections {
		var id int64
		var err error

		if coll.IsVirtual {
			err = tx.QueryRow(`SELECT id FROM collections WHERE virtual_id = ?`, coll.VirtualID).Scan(&id)
		} else {
			collType := "regular"
			if coll.IsSmart {
				collType = "smart"
			}
			err = tx.QueryRow(`SELECT id FROM collections WHERE romm_id = ? AND type = ?`, coll.ID, collType).Scan(&id)
		}

		if err == nil {
			collectionIDs[GetCollectionCacheKey(coll)] = id
		}
	}

	// Batch insert all mappings
	const batchSize = 400
	var allMappings []struct {
		gameID       int
		collectionID int64
	}

	for _, coll := range collections {
		collID, ok := collectionIDs[GetCollectionCacheKey(coll)]
		if !ok {
			continue
		}
		for _, romID := range coll.ROMIDs {
			allMappings = append(allMappings, struct {
				gameID       int
				collectionID int64
			}{romID, collID})
		}
	}

	for i := 0; i < len(allMappings); i += batchSize {
		end := i + batchSize
		if end > len(allMappings) {
			end = len(allMappings)
		}
		batch := allMappings[i:end]

		query := "INSERT OR IGNORE INTO game_collections (game_id, collection_id) VALUES "
		args := make([]interface{}, 0, len(batch)*2)
		for j, m := range batch {
			if j > 0 {
				query += ", "
			}
			query += "(?, ?)"
			args = append(args, m.gameID, m.collectionID)
		}

		if _, err := tx.Exec(query, args...); err != nil {
			return newCacheError("save", "collection_mappings", "", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return newCacheError("save", "collection_mappings", "", err)
	}

	logger.Debug("Saved all collection mappings", "collections", len(collections), "mappings", len(allMappings))
	return nil
}

// GetGamesByIDs retrieves multiple games by their IDs efficiently
func (cm *CacheManager) GetGamesByIDs(gameIDs []int) ([]romm.Rom, error) {
	if cm == nil || !cm.initialized {
		return nil, ErrNotInitialized
	}

	if len(gameIDs) == 0 {
		return nil, nil
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Build query with placeholders
	placeholders := make([]string, len(gameIDs))
	args := make([]interface{}, len(gameIDs))
	for i, id := range gameIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := "SELECT data_json FROM games WHERE id IN (" + strings.Join(placeholders, ",") + ") ORDER BY name"

	rows, err := cm.db.Query(query, args...)
	if err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "games", "batch", err)
	}
	defer rows.Close()

	var games []romm.Rom
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "games", "batch", err)
		}

		var game romm.Rom
		if err := json.Unmarshal([]byte(dataJSON), &game); err != nil {
			cm.stats.recordError()
			return nil, newCacheError("get", "games", "batch", err)
		}
		games = append(games, game)
	}

	if err := rows.Err(); err != nil {
		cm.stats.recordError()
		return nil, newCacheError("get", "games", "batch", err)
	}

	if len(games) > 0 {
		cm.stats.recordHit()
	} else {
		cm.stats.recordMiss()
	}

	return games, nil
}

// GetGameByID retrieves a single game by ID
func (cm *CacheManager) GetGameByID(gameID int) (romm.Rom, error) {
	if cm == nil || !cm.initialized {
		return romm.Rom{}, ErrNotInitialized
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var dataJSON string
	err := cm.db.QueryRow(`
		SELECT data_json FROM games WHERE id = ?
	`, gameID).Scan(&dataJSON)

	if err == sql.ErrNoRows {
		cm.stats.recordMiss()
		return romm.Rom{}, ErrCacheMiss
	}
	if err != nil {
		cm.stats.recordError()
		return romm.Rom{}, newCacheError("get", "games", strconv.Itoa(gameID), err)
	}

	var game romm.Rom
	if err := json.Unmarshal([]byte(dataJSON), &game); err != nil {
		cm.stats.recordError()
		return romm.Rom{}, newCacheError("get", "games", strconv.Itoa(gameID), err)
	}

	cm.stats.recordHit()
	return game, nil
}

// HasPlatformGames checks if games are cached for a platform
func (cm *CacheManager) HasPlatformGames(platformID int) bool {
	if cm == nil || !cm.initialized {
		return false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int
	err := cm.db.QueryRow(`SELECT COUNT(*) FROM games WHERE platform_id = ?`, platformID).Scan(&count)
	return err == nil && count > 0
}

// GetCachedGameIDs returns a map of all game IDs in the cache for fast lookup
func (cm *CacheManager) GetCachedGameIDs() map[int]bool {
	if cm == nil || !cm.initialized {
		return nil
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	rows, err := cm.db.Query(`SELECT id FROM games`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	gameIDs := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			gameIDs[id] = true
		}
	}

	return gameIDs
}

// Helper function to get collection internal ID
func (cm *CacheManager) getCollectionInternalID(collection romm.Collection) (int64, error) {
	var id int64
	var err error

	if collection.IsVirtual {
		err = cm.db.QueryRow(`SELECT id FROM collections WHERE virtual_id = ?`, collection.VirtualID).Scan(&id)
	} else {
		collType := "regular"
		if collection.IsSmart {
			collType = "smart"
		}
		err = cm.db.QueryRow(`SELECT id FROM collections WHERE romm_id = ? AND type = ?`, collection.ID, collType).Scan(&id)
	}

	if err == sql.ErrNoRows {
		cm.stats.recordMiss()
		return 0, ErrCacheMiss
	}
	if err != nil {
		cm.stats.recordError()
		return 0, newCacheError("get", "collections", GetCollectionCacheKey(collection), err)
	}

	return id, nil
}

// ClearGamesCache clears the games cache (compatibility wrapper)
func ClearGamesCache() error {
	cm := GetCacheManager()
	if cm == nil {
		return nil
	}
	return cm.ClearGames()
}

// HasGamesCache checks if games cache has data (compatibility wrapper)
func HasGamesCache() bool {
	cm := GetCacheManager()
	if cm == nil {
		return false
	}
	return cm.HasCache()
}

// GetGamesCacheDir returns the cache directory (kept for compatibility)
// Note: This no longer reflects actual storage location but is kept for compatibility
func GetGamesCacheDir() string {
	return GetArtworkCacheDir() // Just return something valid
}
