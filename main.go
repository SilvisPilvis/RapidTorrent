package main

import (
	"flag"
	"fmt"
	"time"

	"main/model"

	tea "github.com/charmbracelet/bubbletea"
)

func printUsage() {
	fmt.Printf(`
RapidTorrent - A simple terminal torrent client

Usage:
    rapidtorrent [options]

Options:
    -h, --help      Show this help message
    -magnet URL     Download torrent from magnet URL
    -file PATH      Download torrent from .torrent file

Examples:
    rapidtorrent
    rapidtorrent -magnet "magnet:?xt=urn:btih:..."
    rapidtorrent -file "path/to/file.torrent"

Keys:
    enter   Add new magnet link
		esc     Back
    q       Quit application
    ctrl+c  Force quit
`)
}

func main() {
	var magnetURL string
	var torrentFile string
	var help bool

	flag.BoolVar(&help, "h", false, "Show help message")
	flag.StringVar(&magnetURL, "magnet", "", "Magnet URL to start downloading")
	flag.StringVar(&torrentFile, "file", "", "Path to .torrent file to start downloading")
	flag.Parse()

	// Show help if -h flag is provided
	if help {
		printUsage()
		return
	}

	m, err := model.InitialModel()
	if err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		return
	}

	defer func() {
		_, err := m.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		if err != nil {
			fmt.Printf("Error checkpointing WAL: %v\n", err)
		}
		m.DB.Close()
	}()

	// Handle command line arguments
	if magnetURL != "" {
		go m.AddTorrent(magnetURL)
	}
	if torrentFile != "" {
		go m.AddTorrentFromFile(torrentFile)
	}

	go func() {
		ticker := time.NewTicker(time.Second / 2)
		for range ticker.C {
			m.UpdateTorrents()
		}
	}()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
	}
}
