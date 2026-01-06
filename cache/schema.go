package cache

import (
	"database/sql"
)

const schemaVersion = 1

// createTables creates all required database tables
func createTables(db *sql.DB) error {
	// Create tables in a transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Cache metadata table - stores schema version and global state
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS cache_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Platforms table - cached platform data
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS platforms (
			id INTEGER PRIMARY KEY,
			slug TEXT NOT NULL,
			fs_slug TEXT NOT NULL,
			name TEXT NOT NULL,
			custom_name TEXT DEFAULT '',
			rom_count INTEGER DEFAULT 0,
			has_bios INTEGER DEFAULT 0,
			data_json TEXT NOT NULL,
			updated_at DATETIME,
			cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_platforms_fs_slug ON platforms(fs_slug)`)
	if err != nil {
		return err
	}

	// Collections table - cached collection data
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS collections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			romm_id INTEGER,
			virtual_id TEXT,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			rom_count INTEGER DEFAULT 0,
			data_json TEXT NOT NULL,
			updated_at DATETIME,
			cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(romm_id, type),
			UNIQUE(virtual_id)
		)
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_collections_type ON collections(type)`)
	if err != nil {
		return err
	}

	// Games table - cached ROM/game data
	// Stores full JSON to preserve all fields from romm.Rom
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS games (
			id INTEGER PRIMARY KEY,
			platform_id INTEGER NOT NULL,
			platform_fs_slug TEXT NOT NULL,
			name TEXT NOT NULL,
			fs_name TEXT DEFAULT '',
			data_json TEXT NOT NULL,
			updated_at DATETIME,
			cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_games_platform_id ON games(platform_id)`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_games_platform_fs_slug ON games(platform_fs_slug)`)
	if err != nil {
		return err
	}

	// Game-Collection many-to-many relationship
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS game_collections (
			game_id INTEGER NOT NULL,
			collection_id INTEGER NOT NULL,
			PRIMARY KEY (game_id, collection_id)
		)
	`)
	if err != nil {
		return err
	}

	// ROM ID cache - maps filenames to ROM IDs for save sync
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS rom_id_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform_fs_slug TEXT NOT NULL,
			filename_key TEXT NOT NULL,
			rom_id INTEGER NOT NULL,
			rom_name TEXT NOT NULL,
			cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(platform_fs_slug, filename_key)
		)
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_rom_id_cache_lookup ON rom_id_cache(platform_fs_slug, filename_key)`)
	if err != nil {
		return err
	}

	// Artwork metadata - tracks artwork files on disk
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS artwork_metadata (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform_fs_slug TEXT NOT NULL,
			rom_id INTEGER NOT NULL,
			file_path TEXT NOT NULL,
			file_size_bytes INTEGER DEFAULT 0,
			cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			validated_at DATETIME,
			UNIQUE(platform_fs_slug, rom_id)
		)
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_artwork_platform_rom ON artwork_metadata(platform_fs_slug, rom_id)`)
	if err != nil {
		return err
	}

	// BIOS availability per platform
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS bios_availability (
			platform_id INTEGER PRIMARY KEY,
			has_bios INTEGER NOT NULL DEFAULT 0,
			checked_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Store schema version
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO cache_metadata (key, value, updated_at)
		VALUES ('schema_version', ?, CURRENT_TIMESTAMP)
	`, schemaVersion)
	if err != nil {
		return err
	}

	return tx.Commit()
}
