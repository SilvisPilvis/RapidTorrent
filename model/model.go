package model

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"
)

type Model struct {
	TextInput    textinput.Model
	Progress     progress.Model
	Viewport     viewport.Model
	Torrents     map[string]*TorrentItem
	Client       *torrent.Client
	DB           *sql.DB
	Err          error
	Width        int
	Height       int
	Mu           sync.RWMutex
	LastRender   time.Time
	Config       Config
	ShowConfig   bool
	ConfigInputs []textinput.Model
}

type TorrentItem struct {
	ID          int64
	Name        string
	Progress    float64
	Speed       float64
	State       string
	Torrent     *torrent.Torrent
	MagnetURI   string
	LastUpdate  time.Time
	LastBytes   int64
	TotalPeers  int
	ActivePeers int
	UploadSpeed float64
	Downloaded  int64
	Uploaded    int64
}

type Config struct {
	DownloadDir    string
	MaxConnections int
	SeedRatio      float64
	DownloadLimit  int64
	UploadLimit    int64
}

func InitialModel() (*Model, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	if err := initDatabase(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %v", err)
	}

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = filepath.Join(homeDir, "Downloads")
	cfg.EstablishedConnsPerTorrent = 50
	cfg.MaxUnverifiedBytes = 1 << 30
	cfg.DisableIPv6 = false
	cfg.DisableTCP = false
	cfg.DisableUTP = false
	cfg.NoDHT = false
	cfg.NoUpload = false
	cfg.Seed = true

	client, err := torrent.NewClient(cfg)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create torrent client: %v", err)
	}

	ti := textinput.New()
	ti.Placeholder = "Enter magnet link..."
	ti.Focus()
	ti.CharLimit = 1024
	ti.Width = 80

	prog := progress.New(progress.WithDefaultGradient())

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		PaddingRight(2)

	var m = &Model{}

	exists, err := CheckConfigExists(db)
	if err != nil {
		return nil, err
	}

	m = &Model{
		TextInput:  ti,
		Progress:   prog,
		Viewport:   vp,
		Torrents:   make(map[string]*TorrentItem),
		Client:     client,
		DB:         db,
		LastRender: time.Now(),
	}

	if exists {
		if err := m.LoadConfig(); err != nil {
			return nil, err
		}
	}

	// m := &Model{
	// 	TextInput:  ti,
	// 	Progress:   prog,
	// 	Viewport:   vp,
	// 	Torrents:   make(map[string]*TorrentItem),
	// 	Client:     client,
	// 	DB:         db,
	// 	LastRender: time.Now(),
	// }

	if err := m.RestoreActiveTorrents(); err != nil {
		fmt.Printf("Warning: couldn't restore torrents: %v\n", err)
	}

	return m, nil
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.UpdateTorrents)
}

func newConfigInput(label, placeholder, value string) textinput.Model {
	i := textinput.New()
	i.Placeholder = placeholder
	i.SetValue(value)
	i.Width = 40
	i.Prompt = label + ": "
	return i
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "c":
			if !m.ShowConfig {
				m.ShowConfig = true

				if m.Config.DownloadDir == "" {
					m.Config.DownloadDir = filepath.Join(homeDir, "Downloads")
				}

				m.ConfigInputs = []textinput.Model{
					newConfigInput("Download Directory", m.Config.DownloadDir, ""),
					newConfigInput("Max Connections", "Enter max connections", strconv.Itoa(m.Config.MaxConnections)),
					newConfigInput("Seed Ratio", "Enter seed ratio", fmt.Sprintf("%.2f", m.Config.SeedRatio)),
					newConfigInput("Download Limit (KB/s, 0 for unlimited)", "Enter download limit", strconv.FormatInt(m.Config.DownloadLimit, 10)),
					newConfigInput("Upload Limit (KB/s, 0 for unlimited)", "Enter upload limit", strconv.FormatInt(m.Config.UploadLimit, 10)),
				}
				m.ConfigInputs[0].Focus()
				return m, nil
			}
		case "esc":
			if m.ShowConfig {
				m.ShowConfig = false
				return m, nil
			}
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.ShowConfig {
				m.Config.DownloadDir = m.ConfigInputs[0].Value()
				m.Config.MaxConnections, _ = strconv.Atoi(m.ConfigInputs[1].Value())
				m.Config.SeedRatio, _ = strconv.ParseFloat(m.ConfigInputs[2].Value(), 64)
				m.Config.DownloadLimit, _ = strconv.ParseInt(m.ConfigInputs[3].Value(), 10, 64)
				m.Config.UploadLimit, _ = strconv.ParseInt(m.ConfigInputs[4].Value(), 10, 64)

				if err := m.SaveConfig(); err != nil {
					m.Err = err
				} else {
					m.ShowConfig = false
					m.ApplyConfig()
				}
				return m, nil
			} else {
				magnetLink := strings.TrimSpace(m.TextInput.Value())
				if magnetLink != "" {
					go m.AddTorrent(magnetLink)
					m.TextInput.Reset()
				}
			}
		case "tab":
			if m.ShowConfig {
				for i := range m.ConfigInputs {
					if m.ConfigInputs[i].Focused() {
						m.ConfigInputs[i].Blur()
						nextIndex := (i + 1) % len(m.ConfigInputs)
						m.ConfigInputs[nextIndex].Focus()
						break
					}
				}
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Viewport.Width = msg.Width
		m.Viewport.Height = msg.Height - 8

	case tickMsg:
		if time.Since(m.LastRender) < time.Second/30 {
			return m, nil
		}
		m.LastRender = time.Now()

		return m, tea.Batch(
			tea.Every(time.Second, func(t time.Time) tea.Msg {
				return tickMsg{}
			}),
			m.UpdateTorrents,
		)
	}

	if m.ShowConfig {
		for i := range m.ConfigInputs {
			var cmd tea.Cmd
			m.ConfigInputs[i], cmd = m.ConfigInputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}
	} else {
		if cmd := m.handleUpdates(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleUpdates(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.TextInput, cmd = m.TextInput.Update(msg)
	if cmd != nil {
		return cmd
	}

	m.Viewport, cmd = m.Viewport.Update(msg)
	return cmd
}
