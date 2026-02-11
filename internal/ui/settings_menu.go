package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type settingsMenuCloseKeyMap struct {
	Close key.Binding
}

func (k settingsMenuCloseKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Close}
}

func (k settingsMenuCloseKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Close}}
}

var settingsMenuCloseKeys = settingsMenuCloseKeyMap{
	Close: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
}

type SettingsMenuCloseMsg struct{}

type SettingsMenuSelectMsg struct {
	app     *docker.Application
	section SettingsSectionType
}

type SettingsMenu struct {
	app  *docker.Application
	menu Menu
	help Help
}

func NewSettingsMenu(app *docker.Application) SettingsMenu {
	return SettingsMenu{
		app: app,
		menu: NewMenu("settings",
			MenuItem{Label: "Application", Key: int(SettingsSectionApplication), Shortcut: key.NewBinding(key.WithKeys("a"))},
			MenuItem{Label: "Email", Key: int(SettingsSectionEmail), Shortcut: key.NewBinding(key.WithKeys("e"))},
			MenuItem{Label: "Environment", Key: int(SettingsSectionEnvironment), Shortcut: key.NewBinding(key.WithKeys("v"))},
			MenuItem{Label: "Resources", Key: int(SettingsSectionResources), Shortcut: key.NewBinding(key.WithKeys("r"))},
			MenuItem{Label: "Updates", Key: int(SettingsSectionUpdates), Shortcut: key.NewBinding(key.WithKeys("u"))},
			MenuItem{Label: "Backups", Key: int(SettingsSectionBackups), Shortcut: key.NewBinding(key.WithKeys("b"))},
		),
		help: NewHelp(),
	}
}

func (m SettingsMenu) Init() tea.Cmd {
	return nil
}

func (m SettingsMenu) Update(msg tea.Msg) (SettingsMenu, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if cmd := m.help.Update(msg, settingsMenuCloseKeys); cmd != nil {
			return m, cmd
		}

	case tea.KeyMsg:
		if key.Matches(msg, settingsMenuCloseKeys.Close) {
			return m, func() tea.Msg { return SettingsMenuCloseMsg{} }
		}

	case MenuSelectMsg:
		return m, m.selectSection(SettingsSectionType(msg.Key))
	}

	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

func (m SettingsMenu) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Colors.Primary).
		MarginBottom(1)

	title := titleStyle.Render("Settings")

	menuView := m.menu.View()

	helpView := m.help.View(settingsMenuCloseKeys)
	menuWidth := lipgloss.Width(menuView)
	helpLine := lipgloss.NewStyle().MarginTop(1).Width(menuWidth).Align(lipgloss.Center).Render(helpView)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		menuView,
		helpLine,
	)

	return boxStyle.Render(content)
}

// Private

func (m SettingsMenu) selectSection(section SettingsSectionType) tea.Cmd {
	return func() tea.Msg {
		return SettingsMenuSelectMsg{app: m.app, section: section}
	}
}
