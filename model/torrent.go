package model

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Modify addTorrent method
func (m *Model) AddTorrent(magnetURI string) {
	t, err := m.Client.AddMagnet(magnetURI)
	if err != nil {
		m.Err = fmt.Errorf("failed to add magnet: %v", err)
		return
	}

	infoHash := t.InfoHash().String()

	m.Mu.Lock()
	m.Torrents[infoHash] = &TorrentItem{
		Name:       "Fetching metadata...",
		Progress:   0,
		Speed:      0,
		State:      "fetching_metadata",
		Torrent:    t,
		MagnetURI:  magnetURI,
		LastUpdate: time.Now(),
		LastBytes:  0,
	}
	m.Mu.Unlock()

	if err := m.SaveTorrentState(infoHash, m.Torrents[infoHash]); err != nil {
		m.Err = err
		return
	}

	go func() {
		select {
		case <-t.GotInfo():
			m.Mu.Lock()
			if item, exists := m.Torrents[infoHash]; exists {
				item.Name = t.Name()
				item.State = "downloading"
				m.SaveTorrentState(infoHash, item)
				// Start downloading all files automatically
				t.DownloadAll()
			}
			m.Mu.Unlock()
		case <-time.After(30 * time.Second):
			m.Mu.Lock()
			delete(m.Torrents, infoHash)
			m.Err = fmt.Errorf("timeout waiting for torrent info")
			m.Mu.Unlock()
		}
	}()
}

// func (m *Model) AddTorrent(magnetURI string) {
// 	t, err := m.Client.AddMagnet(magnetURI)
// 	if err != nil {
// 		m.Err = fmt.Errorf("failed to add magnet: %v", err)
// 		return
// 	}

// 	infoHash := t.InfoHash().String()

// 	m.Mu.Lock()
// 	m.Torrents[infoHash] = &TorrentItem{
// 		Name:       "Fetching metadata...",
// 		Progress:   0,
// 		Speed:      0,
// 		State:      "fetching_metadata",
// 		Torrent:    t,
// 		MagnetURI:  magnetURI,
// 		LastUpdate: time.Now(),
// 		LastBytes:  0,
// 	}
// 	m.Mu.Unlock()

// 	if err := m.SaveTorrentState(infoHash, m.Torrents[infoHash]); err != nil {
// 		m.Err = err
// 		return
// 	}

// 	go func() {
// 		select {
// 		case <-t.GotInfo():
// 			m.Mu.Lock()
// 			if item, exists := m.Torrents[infoHash]; exists {
// 				item.Name = t.Name()
// 				item.State = "downloading"
// 				m.SaveTorrentState(infoHash, item)
// 				t.DownloadAll()
// 			}
// 			m.Mu.Unlock()
// 		case <-time.After(120 * time.Second): // Increase timeout to 120 seconds
// 			m.Mu.Lock()
// 			delete(m.Torrents, infoHash)
// 			m.Err = fmt.Errorf("timeout waiting for torrent info")
// 			m.Mu.Unlock()
// 		}
// 	}()
// }

func (m *Model) AddTorrentFromFile(torrentPath string) {
	t, err := m.Client.AddTorrentFromFile(torrentPath)
	if err != nil {
		m.Err = fmt.Errorf("failed to add torrent: %v", err)
		return
	}

	infoHash := t.InfoHash().String()

	mi := t.Metainfo()
	magnetURI, err := mi.MagnetV2()
	if err != nil {
		m.Err = fmt.Errorf("failed to generate magnet URI: %v", err)
		return
	}

	m.Mu.Lock()
	m.Torrents[infoHash] = &TorrentItem{
		Name:       filepath.Base(torrentPath),
		Progress:   0,
		Speed:      0,
		State:      "connecting",
		Torrent:    t,
		MagnetURI:  magnetURI.String(),
		LastUpdate: time.Now(),
		LastBytes:  0,
	}
	m.Mu.Unlock()

	if err := m.SaveTorrentState(infoHash, m.Torrents[infoHash]); err != nil {
		m.Err = err
		return
	}

	go func() {
		select {
		case <-t.GotInfo():
			m.Mu.Lock()
			if item, exists := m.Torrents[infoHash]; exists {
				item.Name = t.Name()
				item.State = "downloading"
				m.SaveTorrentState(infoHash, item)
				t.DownloadAll()
			}
			m.Mu.Unlock()
		case <-time.After(30 * time.Second):
			m.Mu.Lock()
			delete(m.Torrents, infoHash)
			m.Err = fmt.Errorf("timeout waiting for torrent info")
			m.Mu.Unlock()
		}
	}()
}

type tickMsg struct{}

func (m *Model) UpdateTorrents() tea.Msg {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	now := time.Now()
	needsUpdate := false

	for infoHash, item := range m.Torrents {
		if item.Torrent == nil {
			continue
		}

		stats := item.Torrent.Stats()
		bytesCompleted := item.Torrent.BytesCompleted()
		totalLength := item.Torrent.Length()

		item.TotalPeers = stats.TotalPeers
		item.ActivePeers = stats.ActivePeers
		item.Downloaded = bytesCompleted
		item.Uploaded = stats.BytesWritten.Int64()

		if totalLength > 0 {
			newProgress := float64(bytesCompleted) / float64(totalLength) * 100
			if newProgress != item.Progress {
				item.Progress = newProgress
				needsUpdate = true

				if err := m.SaveTorrentState(infoHash, item); err != nil {
					m.Err = err
				}
			}

			timeDiff := now.Sub(item.LastUpdate).Seconds()
			bytesDiff := bytesCompleted - item.LastBytes
			if timeDiff > 0 && bytesDiff > 1024 {
				item.Speed = float64(bytesDiff) / timeDiff / 1024 / 1024
				needsUpdate = true
			}
		}

		newState := item.State
		if item.Torrent.Complete().Bool() {
			newState = "completed"
		} else if stats.ActivePeers > 0 && bytesCompleted < totalLength {
			newState = "downloading"
		} else if stats.TotalPeers == 0 {
			newState = "searching"
		} else if stats.ActivePeers == 0 {
			newState = "connecting"
		}

		if newState != item.State {
			item.State = newState
			needsUpdate = true

			if err := m.SaveTorrentState(infoHash, item); err != nil {
				m.Err = err
			}
		}

		item.LastBytes = bytesCompleted
		item.LastUpdate = now

		uploadDiff := stats.BytesWritten.Int64() - item.LastBytes
		if timeDiff := now.Sub(item.LastUpdate).Seconds(); timeDiff > 0 {
			item.UploadSpeed = float64(uploadDiff) / timeDiff / 1024 / 1024
		}
	}

	if needsUpdate {
		return tickMsg{}
	}
	return nil
}
