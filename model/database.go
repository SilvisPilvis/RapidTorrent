package model

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	homeDir, _ = os.UserHomeDir()
	dbPath     = "./rapidtorrent.db"
)

func initDatabase(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS torrents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		info_hash TEXT UNIQUE,
		magnet_uri TEXT NOT NULL,
		name TEXT,
		progress REAL DEFAULT 0,
		state TEXT DEFAULT 'pending',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS torrent_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		torrent_id INTEGER,
		status TEXT,
		progress REAL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(torrent_id) REFERENCES torrents(id)
	);

	CREATE TABLE IF NOT EXISTS config (
        key TEXT PRIMARY KEY,
        value TEXT,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

	CREATE INDEX IF NOT EXISTS idx_torrents_info_hash ON torrents(info_hash);
	CREATE INDEX IF NOT EXISTS idx_history_torrent_id ON torrent_history(torrent_id);
	`

	_, err := db.Exec(schema)

	defaultConfig := map[string]string{
		"download_dir":    filepath.Join(homeDir, "Downloads"),
		"max_connections": "50",
		"seed_ratio":      "1.5",
		"download_limit":  "0",
		"upload_limit":    "0",
	}

	for key, value := range defaultConfig {
		_, err := db.Exec(`
            INSERT OR IGNORE INTO config (key, value)
            VALUES (?, ?)
        `, key, value)
		if err != nil {
			return err
		}
	}

	return err
}

func GetConfigValue(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	return value, err
}

func CheckConfigExists(db *sql.DB) (bool, error) {
	rows, err := db.Query("SELECT COUNT(*) FROM config")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return false, err
		}
	}

	return count > 0, nil
}

var dbMutex sync.Mutex

func (m *Model) SaveTorrentState(infoHash string, item *TorrentItem) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := m.ExecuteSaveTorrent(infoHash, item)
		if err == nil {
			return nil
		}

		if strings.Contains(err.Error(), "database is locked") {
			time.Sleep(time.Millisecond * 250 * time.Duration(1<<attempt)) // Increase delay
			continue
		}

		return err
	}
	return fmt.Errorf("database is locked after %d retries", maxRetries)
}

func (m *Model) ExecuteSaveTorrent(infoHash string, item *TorrentItem) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	query := `
        INSERT INTO torrents (info_hash, magnet_uri, name, progress, state, updated_at)
        VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(info_hash) DO UPDATE SET
            progress = ?,
            state = ?,
            updated_at = CURRENT_TIMESTAMP
    `

	_, err = tx.Exec(query,
		infoHash,
		item.MagnetURI,
		item.Name,
		item.Progress,
		item.State,
		item.Progress,
		item.State,
	)

	if err != nil {
		return fmt.Errorf("failed to save torrent state: %v", err)
	}

	historyQuery := `
        INSERT INTO torrent_history (torrent_id, status, progress)
        SELECT id, ?, ? FROM torrents WHERE info_hash = ?
    `
	_, err = tx.Exec(historyQuery, item.State, item.Progress, infoHash)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (m *Model) RestoreActiveTorrents() error {
	query := `
		SELECT info_hash, magnet_uri, name, progress, state
		FROM torrents
		WHERE state != 'completed'
	`

	rows, err := m.DB.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var infoHash, magnetURI, name, state string
		var progress float64
		if err := rows.Scan(&infoHash, &magnetURI, &name, &progress, &state); err != nil {
			return err
		}

		if state != "completed" {
			go m.AddTorrent(magnetURI)
		}
	}

	return rows.Err()
}
