package ui

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/basecamp/once/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_KnownAppFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, installStateAppList, m.state)

	// Select Campfire (first item, already selected)
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateHostname, m.state)

	// Enter hostname and submit
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_CustomImageFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Select "Other image..."
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	assert.Equal(t, installStateImageForm, m.state)

	// Submit image
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "nginx:latest"})
	assert.Equal(t, installStateHostname, m.state)

	// Submit hostname
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "nginx:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_CLIModeSkipsToHostname(t *testing.T) {
	m := NewInstall(nil, "campfire")
	assert.Equal(t, installStateHostname, m.state)
}

func TestInstall_CLIModeExpandsAlias(t *testing.T) {
	m := NewInstall(nil, "campfire")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	view := ansi.Strip(m.View())
	assert.Contains(t, view, "once-campfire.example.com")
}

func TestInstall_SubmitTriggersActivity(t *testing.T) {
	m := newTestInstall()
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
	navMsg, ok := msg.(NavigateToAppMsg)
	require.True(t, ok, "expected NavigateToAppMsg, got %T", msg)
	assert.Equal(t, app, navMsg.App)
}

func TestInstall_FailureReturnsToHostname(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Go through known app flow to hostname
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	// Simulate failure
	installErr := errors.New("connection refused")
	m, cmd := updateInstall(m, InstallActivityFailedMsg{Err: installErr})

	assert.NotNil(t, cmd, "expected logo Init cmd on failure return")
	assert.Equal(t, installStateHostname, m.state)
	assert.Equal(t, installErr, m.err)
	assert.Contains(t, m.View(), "Error: connection refused")
}

func TestInstall_ErrorClearsOnKeypress(t *testing.T) {
	m := newTestInstall()
	m.state = installStateHostname
	m.hostnameForm = NewInstallHostnameForm("nginx:latest")
	m.err = errors.New("some error")

	m, _ = updateInstall(m, keyPressMsg("a"))
	assert.Nil(t, m.err)
}

func TestInstall_BackNavigation_AppListEscNavigatesToDashboard(t *testing.T) {
	m := newTestInstall()

	_, cmd := updateInstall(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
}

func TestInstall_BackNavigation_ImageFormEscGoesToAppList(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	assert.Equal(t, installStateImageForm, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_HostnameEscGoesToAppList(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_HostnameEscGoesToImageForm(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "nginx:latest"})
	assert.Equal(t, installStateHostname, m.state)
	assert.True(t, m.customImage)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateImageForm, m.state)
}

func TestInstall_BackNavigation_HostnameBackMsgKnownApp(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})

	m, _ = updateInstall(m, InstallHostnameBackMsg{})
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_HostnameBackMsgCustomImage(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "nginx:latest"})

	m, _ = updateInstall(m, InstallHostnameBackMsg{})
	assert.Equal(t, installStateImageForm, m.state)
}

func TestInstall_BackNavigation_ImageFormBackMsg(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})

	m, _ = updateInstall(m, InstallImageBackMsg{})
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_EscQuitsInCLIMode(t *testing.T) {
	m := NewInstall(nil, "nginx:latest")

	_, cmd := updateInstall(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(QuitMsg)
	assert.True(t, ok, "expected QuitMsg, got %T", msg)
}

func TestInstall_HostnameBackQuitsInCLIMode(t *testing.T) {
	m := NewInstall(nil, "nginx:latest")

	_, cmd := updateInstall(m, InstallHostnameBackMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(QuitMsg)
	assert.True(t, ok, "expected QuitMsg, got %T", msg)
}

func TestInstall_ShowsLogoAndHidesTitleWhenNoApps(t *testing.T) {
	m := NewInstall(nil, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})

	view := m.View()
	assert.Contains(t, view, "██████╗")
	assert.NotContains(t, view, "ONCE · install")
}

func TestInstall_ShowsTitleAndHidesLogoWhenAppsExist(t *testing.T) {
	ns := newTestNamespace("myapp")
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})

	view := m.View()
	assert.NotContains(t, view, "██████╗")
	assert.Contains(t, view, "ONCE · install")
}

func TestInstall_PullFailureReturnsToImageForm(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "bad:image"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "bad:image", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	pullErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, docker.ErrPullFailed)
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: pullErr})
	assert.Equal(t, installStateImageForm, m.state)
	assert.Equal(t, pullErr, m.err)
}

func TestInstall_PullFailureReturnsToAppList(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	pullErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, docker.ErrPullFailed)
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: pullErr})
	assert.Equal(t, installStateAppList, m.state)
	assert.Equal(t, pullErr, m.err)
}

func TestInstall_NonPullDeployFailureReturnsToHostname(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})

	deployErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, errors.New("container crashed"))
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: deployErr})
	assert.Equal(t, installStateHostname, m.state)
}

func TestInstall_HostnameInUseBlocksInstall(t *testing.T) {
	ns := newTestNamespace()
	ns.AddApplication(docker.ApplicationSettings{Name: "myapp", Host: "taken.example.com"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "taken.example.com"})
	assert.Equal(t, installStateHostname, m.state)
	assert.ErrorIs(t, m.err, docker.ErrHostnameInUse)
}

func TestInstall_UniqueHostnameAllowsInstall(t *testing.T) {
	ns := newTestNamespace()
	ns.AddApplication(docker.ApplicationSettings{Name: "myapp", Host: "taken.example.com"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "unique.example.com"})
	assert.Equal(t, installStateActivity, m.state)
	assert.Nil(t, m.err)
}

func TestInstall_FailureRestartsLogoOnlyWhenNoApps(t *testing.T) {
	noApps := NewInstall(nil, "")
	noApps, _ = updateInstall(noApps, tea.WindowSizeMsg{Width: 80, Height: 40})
	noApps, _ = updateInstall(noApps, InstallFormSubmitMsg{ImageRef: "nginx:latest", Hostname: "app.example.com"})
	_, cmd := updateInstall(noApps, InstallActivityFailedMsg{Err: errors.New("fail")})
	assert.NotNil(t, cmd)

	withApps := NewInstall(newTestNamespace("myapp"), "")
	withApps, _ = updateInstall(withApps, tea.WindowSizeMsg{Width: 80, Height: 40})
	withApps, _ = updateInstall(withApps, InstallFormSubmitMsg{ImageRef: "nginx:latest", Hostname: "app.example.com"})
	_, cmd = updateInstall(withApps, InstallActivityFailedMsg{Err: errors.New("fail")})
	assert.Nil(t, cmd)
}

// Helpers

func newTestInstall() Install {
	return NewInstall(nil, "")
}

func newTestNamespace(appNames ...string) *docker.Namespace {
	ns, err := docker.NewNamespace("test")
	if err != nil {
		panic(err)
	}
	for _, name := range appNames {
		ns.AddApplication(docker.ApplicationSettings{Name: name})
	}
	return ns
}

func updateInstall(m Install, msg tea.Msg) (Install, tea.Cmd) {
	comp, cmd := m.Update(msg)
	return comp.(Install), cmd
}
