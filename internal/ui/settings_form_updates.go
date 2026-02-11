package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
)

const updatesAutoUpdateField = 0

type SettingsFormUpdates struct {
	app      *docker.Application
	settings docker.ApplicationSettings
	form     Form
}

func NewSettingsFormUpdates(app *docker.Application) SettingsFormUpdates {
	autoUpdateField := NewCheckboxField("Automatically apply updates", app.Settings.AutoUpdate)

	form := NewForm("Done",
		FormItem{Label: "Updates", Field: autoUpdateField},
	)
	form.SetActionButton("Check for updates", func() tea.Msg {
		return settingsRunActionMsg{action: func() error {
			return app.Update(context.Background(), nil)
		}}
	})

	return SettingsFormUpdates{
		app:      app,
		settings: app.Settings,
		form:     form,
	}
}

func (m SettingsFormUpdates) Title() string {
	return "Updates"
}

func (m SettingsFormUpdates) Init() tea.Cmd {
	return nil
}

func (m SettingsFormUpdates) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var (
		action FormAction
		cmd    tea.Cmd
	)
	m.form, action, cmd = m.form.Update(msg)

	switch action {
	case FormSubmitted:
		m.settings.AutoUpdate = m.form.CheckboxField(updatesAutoUpdateField).Checked()
		return m, func() tea.Msg { return SettingsSectionSubmitMsg{Settings: m.settings} }
	case FormCancelled:
		return m, func() tea.Msg { return SettingsSectionCancelMsg{} }
	}

	return m, cmd
}

func (m SettingsFormUpdates) View() string {
	return m.form.View()
}
