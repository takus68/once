package ui

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/basecamp/once/internal/docker"
)

type SettingsSection interface {
	Init() tea.Cmd
	Update(tea.Msg) (SettingsSection, tea.Cmd)
	View() string
	Title() string
}

type SettingsSectionSubmitMsg struct {
	Settings docker.ApplicationSettings
}

type SettingsSectionCancelMsg struct{}

type settingsKeyMap struct {
	Back key.Binding
}

func (k settingsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Back}
}

func (k settingsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Back}}
}

var settingsKeys = settingsKeyMap{
	Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

type settingsState int

const (
	settingsStateForm settingsState = iota
	settingsStateDeploying
	settingsStateRunningAction
	settingsStateActionComplete
)

type Settings struct {
	namespace            *docker.Namespace
	app                  *docker.Application
	width, height        int
	help                 Help
	state                settingsState
	section              SettingsSection
	sectionType          SettingsSectionType
	progress             ProgressBusy
	err                  error
	actionSuccessMessage string
}

type settingsDeployFinishedMsg struct {
	err error
}

type settingsActionFinishedMsg struct {
	err error
}

type settingsRunActionMsg struct {
	action         func() error
	successMessage string
}

func NewSettings(ns *docker.Namespace, app *docker.Application, sectionType SettingsSectionType) Settings {
	var section SettingsSection
	switch sectionType {
	case SettingsSectionApplication:
		section = NewSettingsFormApplication(app.Settings)
	case SettingsSectionEmail:
		section = NewSettingsFormEmail(app.Settings)
	case SettingsSectionEnvironment:
		section = NewSettingsFormEnvironment(app.Settings)
	case SettingsSectionResources:
		section = NewSettingsFormResources(app.Settings)
	case SettingsSectionUpdates:
		section = NewSettingsFormUpdates(app)
	case SettingsSectionBackups:
		section = NewSettingsFormBackups(app)
	}

	return Settings{
		namespace:   ns,
		app:         app,
		help:        NewHelp(),
		state:       settingsStateForm,
		section:     section,
		sectionType: sectionType,
	}
}

func (m Settings) Init() tea.Cmd {
	return m.section.Init()
}

func (m Settings) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.progress = NewProgressBusy(m.width, Colors.Border)
		if m.state == settingsStateForm {
			m.section, _ = m.section.Update(msg)
		}
		if m.state == settingsStateDeploying || m.state == settingsStateRunningAction {
			cmds = append(cmds, m.progress.Init())
		}

	case tea.MouseClickMsg:
		if m.state == settingsStateForm {
			if cmd := m.help.Update(msg, settingsKeys); cmd != nil {
				return m, cmd
			}
		}
		if m.state == settingsStateActionComplete {
			if msg.Button == tea.MouseLeft {
				if zi := zone.Get(m.doneButtonZoneID()); zi != nil && zi.InBounds(msg) {
					return m, func() tea.Msg { return navigateToDashboardMsg{} }
				}
			}
		}

	case tea.KeyMsg:
		if m.state == settingsStateActionComplete {
			if key.Matches(msg, key.NewBinding(key.WithKeys("enter"))) {
				return m, func() tea.Msg { return navigateToDashboardMsg{} }
			}
			return m, nil
		}
		if m.state == settingsStateForm {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, settingsKeys.Back) {
				return m, func() tea.Msg { return navigateToDashboardMsg{} }
			}
		}

	case SettingsSectionCancelMsg:
		return m, func() tea.Msg { return navigateToDashboardMsg{} }

	case SettingsSectionSubmitMsg:
		if msg.Settings.Equal(m.app.Settings) {
			return m, func() tea.Msg { return navigateToDashboardMsg{} }
		}
		m.state = settingsStateDeploying
		m.app.Settings = msg.Settings
		m.progress = NewProgressBusy(m.width, Colors.Border)
		return m, tea.Batch(m.progress.Init(), m.runDeploy())

	case settingsRunActionMsg:
		m.state = settingsStateRunningAction
		m.actionSuccessMessage = msg.successMessage
		m.progress = NewProgressBusy(m.width, Colors.Border)
		return m, tea.Batch(m.progress.Init(), func() tea.Msg {
			return settingsActionFinishedMsg{err: msg.action()}
		})

	case settingsDeployFinishedMsg:
		return m, func() tea.Msg { return navigateToAppMsg{app: m.app} }

	case settingsActionFinishedMsg:
		if msg.err != nil {
			m.state = settingsStateForm
			m.err = msg.err
			return m, nil
		}
		if m.actionSuccessMessage != "" {
			m.state = settingsStateActionComplete
			return m, nil
		}
		return m, func() tea.Msg { return navigateToAppMsg{app: m.app} }

	case progressBusyTickMsg:
		if m.state == settingsStateDeploying || m.state == settingsStateRunningAction {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	var cmd tea.Cmd
	if m.state == settingsStateForm {
		m.section, cmd = m.section.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Settings) View() string {
	subtitle := Styles.SubTitle.Width(m.width).Align(lipgloss.Center).Render(m.section.Title() + " Settings")
	titleBox := Styles.TitleBox(m.width, m.app.Settings.URL(), subtitle)

	var contentView string
	switch m.state {
	case settingsStateForm:
		var errorLine string
		if m.err != nil {
			errorLine = lipgloss.NewStyle().Foreground(Colors.Error).Render("Error: " + m.err.Error())
		}
		contentView = lipgloss.JoinVertical(lipgloss.Center, errorLine, "", m.section.View())
	case settingsStateActionComplete:
		contentView = m.renderActionComplete()
	default:
		contentView = m.progress.View()
	}

	var helpLine string
	if m.state == settingsStateForm {
		helpView := m.help.View(settingsKeys)
		helpLine = Styles.HelpLine(m.width, helpView)
	}

	titleBoxHeight := lipgloss.Height(titleBox)
	helpHeight := lipgloss.Height(helpLine)
	middleHeight := m.height - titleBoxHeight - helpHeight

	centeredContent := lipgloss.Place(
		m.width,
		middleHeight,
		lipgloss.Center,
		lipgloss.Center,
		contentView,
	)

	return titleBox + centeredContent + helpLine
}

// Private

func (m Settings) renderActionComplete() string {
	statusLine := Styles.CenteredLine(m.width, m.actionSuccessMessage)

	buttonStyle := Styles.Button.BorderForeground(Colors.Focused)
	buttonView := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		MarginTop(1).
		Render(zone.Mark(m.doneButtonZoneID(), buttonStyle.Render("Done")))

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, buttonView)
}

func (m Settings) doneButtonZoneID() string { return "settings_done_button" }

func (m Settings) runDeploy() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Deploy(context.Background(), nil)
		return settingsDeployFinishedMsg{err: err}
	}
}
