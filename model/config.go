package model

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

func (m *Model) LoadConfig() error {
	rows, err := m.DB.Query("SELECT key, value FROM config")
	if err != nil {
		return err
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		config[key] = value
	}

	m.Config.DownloadDir = config["download_dir"]
	m.Config.MaxConnections, _ = strconv.Atoi(config["max_connections"])
	m.Config.SeedRatio, _ = strconv.ParseFloat(config["seed_ratio"], 64)
	m.Config.DownloadLimit, _ = strconv.ParseInt(config["download_limit"], 10, 64)
	m.Config.UploadLimit, _ = strconv.ParseInt(config["upload_limit"], 10, 64)

	return nil
}

func (m *Model) SaveConfig() error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	configs := map[string]string{
		"download_dir":    m.Config.DownloadDir,
		"max_connections": strconv.Itoa(m.Config.MaxConnections),
		"seed_ratio":      fmt.Sprintf("%.2f", m.Config.SeedRatio),
		"download_limit":  strconv.FormatInt(m.Config.DownloadLimit, 10),
		"upload_limit":    strconv.FormatInt(m.Config.UploadLimit, 10),
	}

	for key, value := range configs {
		_, err := tx.Exec(`
            UPDATE config 
            SET value = ?, updated_at = CURRENT_TIMESTAMP
            WHERE key = ?
        `, value, key)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (m *Model) ApplyConfig() {
	// Close the old client properly
	if m.Client != nil {
		m.Client.Close()
		time.Sleep(3 * time.Second) // Increase the delay if necessary
	}

	// Create new client configuration
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = m.Config.DownloadDir
	cfg.EstablishedConnsPerTorrent = m.Config.MaxConnections

	// Create a new client
	newClient, err := torrent.NewClient(cfg)
	if err != nil {
		m.Err = fmt.Errorf("failed to apply new configuration: %v", err)
		return
	}

	// Store existing torrents
	existingTorrents := make(map[string]*TorrentItem)
	for hash, item := range m.Torrents {
		existingTorrents[hash] = item
	}

	// Set new client
	m.Client = newClient

	// Re-add existing torrents to the new client with limited concurrency
	sem := make(chan struct{}, 5) // Limit to 5 concurrent goroutines
	var wg sync.WaitGroup

	for _, item := range existingTorrents {
		if item.MagnetURI != "" {
			wg.Add(1)
			go func(item *TorrentItem) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire semaphore
				defer func() { <-sem }() // Release semaphore
				m.AddTorrent(item.MagnetURI)
			}(item)
		}
	}

	wg.Wait()

	m.Err = fmt.Errorf("New configuration will be applied after restart: %v", err)
}
