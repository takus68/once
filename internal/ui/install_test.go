package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/basecamp/once/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_SubmitTriggersActivity(t *testing.T) {
	m := newTestInstall()
	assert.Equal(t, installStateForm, m.state)

	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "nginx:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_SuccessNavigatesToApp(t *testing.T) {
	m := newTestInstall()
	app := &docker.Application{}

	_, cmd := updateInstall(m, InstallActivityDoneMsg{App: app})
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(navigateToAppMsg)
	require.True(t, ok, "expected navigateToAppMsg, got %T", msg)
	assert.Equal(t, app, navMsg.app)
}

func TestInstall_FailureReturnsToFormWithError(t *testing.T) {
	m := newTestInstall()

	// Fill the form fields before submitting
	m.form = fillInstallForm(m.form, "nginx:latest", "app.example.com")

	// Submit to enter activity state
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "nginx:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	// Simulate failure
	installErr := errors.New("connection refused")
	m, cmd := updateInstall(m, InstallActivityFailedMsg{Err: installErr})

	assert.Nil(t, cmd)
	assert.Equal(t, installStateForm, m.state)
	assert.Equal(t, installErr, m.err)
	assert.Contains(t, m.View(), "Error: connection refused")

	// Form field values are preserved
	assert.Equal(t, "nginx:latest", m.form.ImageRef())
	assert.Equal(t, "app.example.com", m.form.Hostname())
}

func TestInstall_ErrorClearsOnKeypress(t *testing.T) {
	m := newTestInstall()
	m.err = errors.New("some error")

	m, _ = updateInstall(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.Nil(t, m.err)
}

func TestInstall_EscNavigatesToDashboard(t *testing.T) {
	m := newTestInstall()

	_, cmd := updateInstall(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(navigateToDashboardMsg)
	assert.True(t, ok, "expected navigateToDashboardMsg, got %T", msg)
}

func TestInstall_CancelNavigatesToDashboard(t *testing.T) {
	m := newTestInstall()

	_, cmd := updateInstall(m, InstallFormCancelMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(navigateToDashboardMsg)
	assert.True(t, ok, "expected navigateToDashboardMsg, got %T", msg)
}

// Helpers

func newTestInstall() Install {
	return NewInstall(nil)
}

func fillInstallForm(form InstallForm, imageRef, hostname string) InstallForm {
	form = installTypeText(form, imageRef)
	form = installPressEnter(form)
	form = installTypeText(form, hostname)
	return form
}

func updateInstall(m Install, msg tea.Msg) (Install, tea.Cmd) {
	component, cmd := m.Update(msg)
	return component.(Install), cmd
}
