package ui

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/amar/internal/docker"
	"github.com/basecamp/amar/internal/metrics"
)

type dashboardKeyMap struct {
	Upgrade key.Binding
	PrevApp key.Binding
	NextApp key.Binding
	Quit    key.Binding
}

func (k dashboardKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.PrevApp, k.NextApp, k.Upgrade, k.Quit}
}

func (k dashboardKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.PrevApp, k.NextApp, k.Upgrade, k.Quit}}
}

var dashboardKeys = dashboardKeyMap{
	Upgrade: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "upgrade")),
	PrevApp: key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev app")),
	NextApp: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next app")),
	Quit:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")),
}

type Dashboard struct {
	app           *docker.Application
	scraper       *metrics.MetricsScraper
	width, height int
	upgrading     bool
	progress      ProgressBusy
	help          help.Model
	allReqChart   RequestRateChart
	errorChart    RequestRateChart
}

type RequestRateChart struct {
	scraper    *metrics.MetricsScraper
	service    string
	title      string
	onlyErrors bool
	width      int
	height     int
	data       []float64
}

func NewRequestRateChart(scraper *metrics.MetricsScraper, service, title string, onlyErrors bool) RequestRateChart {
	return RequestRateChart{
		scraper:    scraper,
		service:    service,
		title:      title,
		onlyErrors: onlyErrors,
	}
}

func (c *RequestRateChart) SetSize(width, height int) {
	c.width = width
	c.height = height
}

func (c *RequestRateChart) Update() {
	samples := c.scraper.FetchAverage(c.service, 200, 12)

	c.data = make([]float64, len(samples))
	for i, s := range samples {
		if c.onlyErrors {
			c.data[i] = float64(s.ServerErrors)
		} else {
			c.data[i] = float64(s.Success + s.ClientErrors + s.ServerErrors)
		}
	}

	// Reverse so most recent is on the right
	slices.Reverse(c.data)
}

func (c RequestRateChart) View() string {
	// Ensure data fills the chart width (each chart char = 2 data points)
	dataPoints := c.width * 2
	data := c.data
	if len(data) < dataPoints {
		padded := make([]float64, dataPoints)
		copy(padded[dataPoints-len(data):], data)
		data = padded
	} else if len(data) > dataPoints {
		data = data[len(data)-dataPoints:]
	}

	chart := Chart{
		Width:  c.width,
		Height: c.height,
		Data:   data,
		Title:  c.title,
	}

	if c.onlyErrors {
		chart.Color = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	} else {
		chart.Color = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
	}

	return chart.View()
}

type dashboardTickMsg struct{}

type upgradeFinishedMsg struct {
	err error
}

func NewDashboard(app *docker.Application, scraper *metrics.MetricsScraper) Dashboard {
	service := app.Settings.Name
	return Dashboard{
		app:         app,
		scraper:     scraper,
		help:        help.New(),
		allReqChart: NewRequestRateChart(scraper, service, "Requests/min", false),
		errorChart:  NewRequestRateChart(scraper, service, "Errors/min", true),
	}
}

func (m Dashboard) Init() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return dashboardTickMsg{} })
}

func (m Dashboard) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress = NewProgressBusy(m.width, lipgloss.Color("#6272a4"))
		m.help.SetWidth(m.width)

		chartWidth := m.width / 2
		chartHeight := 20
		m.allReqChart.SetSize(chartWidth, chartHeight)
		m.errorChart.SetSize(chartWidth, chartHeight)

		if m.upgrading {
			cmds = append(cmds, m.progress.Init())
		}
	case tea.KeyMsg:
		if key.Matches(msg, dashboardKeys.Upgrade) && !m.upgrading {
			m.upgrading = true
			m.progress = NewProgressBusy(m.width, lipgloss.Color("#6272a4"))
			return m, tea.Batch(m.progress.Init(), m.runUpgrade())
		}
	case upgradeFinishedMsg:
		m.upgrading = false
	case dashboardTickMsg:
		m.allReqChart.Update()
		m.errorChart.Update()
		cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg { return dashboardTickMsg{} }))
	case progressBusyTickMsg:
		if m.upgrading {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Dashboard) View() string {
	// Build info box content
	var status string
	var statusColor color.Color
	if m.upgrading {
		status = "upgrading..."
		statusColor = lipgloss.Color("#f1fa8c")
	} else if m.app.Running {
		status = "running"
		statusColor = lipgloss.Color("#50fa7b")
	} else {
		status = "stopped"
		statusColor = lipgloss.Color("#ff5555")
	}

	stateStyle := lipgloss.NewStyle().Foreground(statusColor)
	stateDisplay := fmt.Sprintf("State: %s", stateStyle.Render(status))

	if m.app.Running && !m.app.RunningSince.IsZero() && !m.upgrading {
		stateDisplay += fmt.Sprintf(" (up %s)", formatDuration(time.Since(m.app.RunningSince)))
	}

	// Inner width accounts for border (2) and padding (2)
	innerWidth := m.width - 4

	var infoLines []string
	titleLine := Styles.Title.Width(innerWidth).Align(lipgloss.Center).Render(m.app.Settings.Name)
	infoLines = append(infoLines, titleLine)
	infoLines = append(infoLines, stateDisplay)
	if url := m.app.Settings.URL(); url != "" {
		infoLines = append(infoLines, fmt.Sprintf("URL: %s", url))
	}

	infoContent := strings.Join(infoLines, "\n")
	infoBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6272a4")).
		Padding(0, 1).
		Width(m.width).
		Render(infoContent)

	// Charts side by side
	charts := lipgloss.JoinHorizontal(lipgloss.Top, m.allReqChart.View(), m.errorChart.View())

	// Help string (last line, centered)
	helpView := m.help.View(dashboardKeys)
	helpLine := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(helpView)

	// Progress bar (second-to-last line, only during upgrade)
	var bottomContent string
	if m.upgrading {
		bottomContent = m.progress.View() + "\n" + helpLine
	} else {
		bottomContent = helpLine
	}

	// Calculate available height for main content
	topContent := infoBox + "\n" + charts

	topHeight := lipgloss.Height(topContent)
	bottomHeight := lipgloss.Height(bottomContent)
	middleHeight := max(m.height-topHeight-bottomHeight, 0)

	middle := strings.Repeat("\n", middleHeight)

	return topContent + middle + bottomContent
}

// Private

func (m Dashboard) runUpgrade() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Update(context.Background(), nil)
		return upgradeFinishedMsg{err: err}
	}
}

// Helpers

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}
