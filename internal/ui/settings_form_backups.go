package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
)

const (
	backupsPathField = iota
	backupsAutoBackField
)

type SettingsFormBackups struct {
	app      *docker.Application
	settings docker.ApplicationSettings
	form     Form
}

func NewSettingsFormBackups(app *docker.Application) SettingsFormBackups {
	pathField := NewTextField("/path/to/backups")
	pathField.SetValue(app.Settings.Backup.Path)

	autoBackField := NewCheckboxField("Automatically create backups", app.Settings.Backup.AutoBack)

	form := NewForm("Done",
		FormItem{Label: "Backup location", Field: pathField},
		FormItem{Label: "Backups", Field: autoBackField},
	)
	form.SetActionButton("Run backup now", func() tea.Msg {
		return settingsRunActionMsg{
			action: func() error {
				return runBackup(app, pathField.Value())
			},
			successMessage: "Backup complete",
		}
	})

	return SettingsFormBackups{
		app:      app,
		settings: app.Settings,
		form:     form,
	}
}

func (m SettingsFormBackups) Title() string {
	return "Backups"
}

func (m SettingsFormBackups) Init() tea.Cmd {
	return nil
}

func (m SettingsFormBackups) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var (
		action FormAction
		cmd    tea.Cmd
	)
	m.form, action, cmd = m.form.Update(msg)

	switch action {
	case FormSubmitted:
		m.settings.Backup.Path = m.form.TextField(backupsPathField).Value()
		m.settings.Backup.AutoBack = m.form.CheckboxField(backupsAutoBackField).Checked()
		return m, func() tea.Msg { return SettingsSectionSubmitMsg{Settings: m.settings} }
	case FormCancelled:
		return m, func() tea.Msg { return SettingsSectionCancelMsg{} }
	}

	return m, cmd
}

func (m SettingsFormBackups) View() string {
	return m.form.View()
}

// Helpers

func runBackup(app *docker.Application, dir string) error {
	f, err := createBackupFile(dir, app.Settings.Name)
	if err != nil {
		return err
	}
	defer f.Close()

	return app.Backup(context.Background(), f)
}

func createBackupFile(dir, appName string) (*os.File, error) {
	if dir == "" {
		return nil, fmt.Errorf("backup location is required")
	}

	if !filepath.IsAbs(dir) {
		return nil, docker.ErrBackupPathRelative
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	filename := backupFileName(appName, time.Now())
	return os.Create(filepath.Join(dir, filename))
}

func backupFileName(appName string, t time.Time) string {
	return fmt.Sprintf("%s-%s.tar.gz", appName, t.Format("20060102-150405"))
}
