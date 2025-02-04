package model

import (
	"fmt"
	"strings"

	"main/utils"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF75B7")).
			MarginLeft(2)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9F72FF")).
			MarginLeft(2)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			MarginLeft(2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#FF75B7")).
			Bold(true)
)

func (m *Model) View() string {
	m.Mu.RLock()
	defer m.Mu.RUnlock()

	var s strings.Builder
	s.WriteString(titleStyle.Render("🧲 RapidTorrent"))
	s.WriteString("\n\n")

	if m.ShowConfig {
		s.WriteString(titleStyle.Render("Configuration"))
		s.WriteString("\n\n")
		for _, input := range m.ConfigInputs {
			s.WriteString(input.View())
			s.WriteString("\n")
		}
		s.WriteString("\nPress Enter to save, Esc to cancel")
	} else {
		var content strings.Builder
		for _, item := range m.Torrents {
			content.WriteString(fmt.Sprintf("Name: %s\n", item.Name))

			progressWidth := m.Width - 8
			if progressWidth < 20 {
				progressWidth = 20
			}

			prog := progress.New(
				progress.WithDefaultGradient(),
				progress.WithWidth(progressWidth),
				progress.WithoutPercentage(),
			)

			progStr := prog.ViewAs(item.Progress / 100)
			content.WriteString(fmt.Sprintf("%s %.1f%%\n", progStr, item.Progress))

			content.WriteString(fmt.Sprintf("↓ %.2f MB/s • ↑ %.2f MB/s • Peers: %d/%d\n",
				item.Speed, item.UploadSpeed, item.ActivePeers, item.TotalPeers))
			content.WriteString(fmt.Sprintf("Downloaded: %s • Uploaded: %s • Ratio: %.2f\n",
				utils.FormatBytes(item.Downloaded), utils.FormatBytes(item.Uploaded),
				float64(item.Uploaded)/float64(item.Downloaded+1)))
			content.WriteString(fmt.Sprintf("State: %s\n", item.State))

			separatorWidth := m.Width - 4
			if separatorWidth < 1 {
				separatorWidth = 1
			}
			content.WriteString(strings.Repeat("─", separatorWidth))
			content.WriteString("\n")
		}

		m.Viewport.SetContent(content.String())
		s.WriteString(m.Viewport.View())
		s.WriteString("\n\n")
		s.WriteString(m.TextInput.View())
	}

	if m.Err != nil {
		s.WriteString("\n")
		s.WriteString(errorStyle.Render(m.Err.Error()))
	}

	statusBar := fmt.Sprintf(" %d torrents • Press 'c' for config • Press 'Tab' to switch between options • 'q' to quit", len(m.Torrents))
	s.WriteString("\n")
	s.WriteString(statusBarStyle.Render(statusBar))

	return s.String()
}
