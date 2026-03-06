package ui

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/basecamp/once/internal/docker"
)

var installKeys = struct {
	Back key.Binding
}{
	Back: WithHelp(NewKeyBinding("esc"), "esc", "back"),
}

type installState int

const (
	installStateAppList   installState = iota // Screen 1: choose app
	installStateImageForm                     // Screen 2: enter image ref
	installStateHostname                      // Screen 3: enter hostname
	installStateActivity                      // Installing
)

type InstallFormSubmitMsg struct {
	ImageRef string
	Hostname string
}

type Install struct {
	namespace     *docker.Namespace
	width, height int
	help          Help
	state         installState
	appList       InstallAppList
	imageForm     InstallImageForm
	hostnameForm  InstallHostnameForm
	activity      *InstallActivity
	starfield     *Starfield
	logo          *Logo
	err           error
	cliMode       bool
	customImage   bool
}

func NewInstall(ns *docker.Namespace, imageRef string) Install {
	h := NewHelp()
	h.SetBindings([]key.Binding{installKeys.Back})

	m := Install{
		namespace: ns,
		help:      h,
		cliMode:   imageRef != "",
	}

	if imageRef != "" {
		if expanded, ok := expandAlias(imageRef); ok {
			imageRef = expanded
		}
		m.state = installStateHostname
		m.hostnameForm = NewInstallHostnameForm(imageRef)
	} else {
		m.state = installStateAppList
		m.appList = NewInstallAppList()
	}

	if m.showLogo() {
		m.starfield = NewStarfield()
		m.logo = NewLogo()
	}
	return m
}

func (m Install) Init() tea.Cmd {
	cmds := []tea.Cmd{m.initCurrentScreen()}
	if m.showLogo() {
		cmds = append(cmds, m.starfield.Init(), m.logo.Init())
	}
	return tea.Batch(cmds...)
}

func (m Install) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		var cmds []tea.Cmd
		if m.starfield != nil {
			cmds = append(cmds, m.starfield.Update(tea.WindowSizeMsg{Width: m.width, Height: m.middleHeight()}))
		}
		if m.state == installStateActivity {
			cmds = append(cmds, m.activity.Update(msg))
		} else {
			cmds = append(cmds, m.updateCurrentScreen(msg))
		}
		return m, tea.Batch(cmds...)

	case starfieldTickMsg:
		if m.starfield != nil {
			return m, m.starfield.Update(msg)
		}
		return m, nil

	case logoShineStartMsg, logoShineStepMsg:
		if m.showLogo() && m.state != installStateActivity {
			return m, m.logo.Update(msg)
		}
		return m, nil

	case MouseEvent:
		if m.state != installStateActivity {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			if cmd != nil {
				return m, cmd
			}
		}

	case tea.KeyPressMsg:
		if m.state != installStateActivity {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, installKeys.Back) {
				return m.handleBack()
			}
		}

	case InstallAppSelectedMsg:
		m.hostnameForm = NewInstallHostnameForm(msg.ImageRef)
		m.customImage = false
		m.state = installStateHostname
		return m, m.initScreenWithSize()

	case InstallCustomSelectedMsg:
		m.imageForm = NewInstallImageForm()
		m.state = installStateImageForm
		return m, m.initScreenWithSize()

	case InstallImageSubmitMsg:
		m.hostnameForm = NewInstallHostnameForm(msg.ImageRef)
		m.customImage = true
		m.state = installStateHostname
		return m, m.initScreenWithSize()

	case InstallImageBackMsg:
		m.state = installStateAppList
		return m, nil

	case InstallHostnameBackMsg:
		if m.cliMode {
			return m, m.cancelFromScreen()
		}
		if m.customImage {
			m.state = installStateImageForm
			return m, m.imageForm.Init()
		}
		m.state = installStateAppList
		return m, nil

	case InstallFormSubmitMsg:
		if m.namespace != nil && m.namespace.HostInUse(msg.Hostname) {
			m.err = docker.ErrHostnameInUse
			return m, nil
		}
		m.state = installStateActivity
		m.activity = NewInstallActivity(m.namespace, msg.ImageRef, msg.Hostname)
		m.activity.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.activity.Init()

	case InstallActivityFailedMsg:
		m.activity = nil
		m.err = msg.Err
		if errors.Is(msg.Err, docker.ErrPullFailed) {
			m.state = m.imageErrorState()
		} else {
			m.state = installStateHostname
		}
		if m.showLogo() {
			return m, m.logo.Init()
		}
		return m, nil

	case InstallActivityDoneMsg:
		return m, func() tea.Msg { return NavigateToAppMsg(msg) }
	}

	if m.state == installStateActivity {
		return m, m.activity.Update(msg)
	}
	return m, m.updateCurrentScreen(msg)
}

func (m Install) View() string {
	var contentView string
	if m.state == installStateActivity {
		contentView = m.activity.View()
	} else {
		formView := m.viewCurrentScreen()
		if m.err != nil {
			errorLine := lipgloss.NewStyle().Foreground(Colors.Error).Render("Error: " + m.err.Error())
			formView = lipgloss.JoinVertical(lipgloss.Center, errorLine, "", formView)
		}
		contentView = formView
	}

	var helpLine string
	if m.state != installStateActivity {
		helpLine = Styles.CenteredLine(m.width, m.help.View())
	}

	middleH := m.middleHeight()

	if m.starfield != nil {
		if m.showLogo() && m.state != installStateActivity {
			return m.renderLogoWithStarfield(contentView, middleH) + helpLine
		}
		return m.renderMiddleWithStarfield(contentView, middleH) + helpLine
	}

	middle := m.renderMiddleCentered(contentView, middleH)
	titleLine := Styles.TitleRule(m.width, "install")
	return titleLine + "\n\n" + middle + helpLine
}

// Private

func (m Install) initCurrentScreen() tea.Cmd {
	switch m.state {
	case installStateAppList:
		return m.appList.Init()
	case installStateImageForm:
		return m.imageForm.Init()
	case installStateHostname:
		return m.hostnameForm.Init()
	}
	return nil
}

func (m *Install) initScreenWithSize() tea.Cmd {
	initCmd := m.initCurrentScreen()
	if m.width > 0 {
		m.updateCurrentScreen(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
	return initCmd
}

func (m *Install) updateCurrentScreen(msg tea.Msg) tea.Cmd {
	switch m.state {
	case installStateAppList:
		var cmd tea.Cmd
		m.appList, cmd = m.appList.Update(msg)
		return cmd
	case installStateImageForm:
		var cmd tea.Cmd
		m.imageForm, cmd = m.imageForm.Update(msg)
		return cmd
	case installStateHostname:
		var cmd tea.Cmd
		m.hostnameForm, cmd = m.hostnameForm.Update(msg)
		return cmd
	}
	return nil
}

func (m Install) viewCurrentScreen() string {
	switch m.state {
	case installStateAppList:
		return m.appList.View()
	case installStateImageForm:
		return m.imageForm.View()
	case installStateHostname:
		return m.hostnameForm.View()
	}
	return ""
}

func (m Install) handleBack() (Install, tea.Cmd) {
	switch m.state {
	case installStateAppList:
		return m, m.cancelFromScreen()
	case installStateImageForm:
		m.state = installStateAppList
		return m, nil
	case installStateHostname:
		if m.cliMode {
			return m, m.cancelFromScreen()
		}
		if m.customImage {
			m.state = installStateImageForm
			return m, m.imageForm.Init()
		}
		m.state = installStateAppList
		return m, nil
	}
	return m, nil
}

func (m Install) imageErrorState() installState {
	if m.customImage {
		return installStateImageForm
	}
	return installStateAppList
}

func (m Install) showLogo() bool {
	return m.namespace == nil || len(m.namespace.Applications()) == 0
}

func (m Install) middleHeight() int {
	helpHeight := 1
	if m.starfield != nil {
		return max(m.height-helpHeight, 0)
	}
	titleHeight := 2
	return max(m.height-titleHeight-helpHeight, 0)
}

func (m Install) cancelFromScreen() tea.Cmd {
	if m.activity != nil {
		m.activity.Cancel()
	}
	if m.cliMode {
		return func() tea.Msg { return QuitMsg{} }
	}
	return func() tea.Msg { return NavigateToDashboardMsg{} }
}

func (m Install) renderMiddleCentered(contentView string, middleHeight int) string {
	centered := lipgloss.NewStyle().
		Width(m.width).
		Height(middleHeight).
		Align(lipgloss.Center, lipgloss.Center).
		Render(contentView)
	return centered
}

// renderLogoWithStarfield composites the logo (pinned at top) and form (centered)
// as independent layers over the starfield.
func (m Install) renderLogoWithStarfield(formView string, middleHeight int) string {
	m.starfield.ComputeGrid()

	logoView := m.logo.View()
	logoLines := strings.Split(logoView, "\n")
	logoWidth := 0
	for _, line := range logoLines {
		if w := ansi.StringWidth(line); w > logoWidth {
			logoWidth = w
		}
	}
	logoTop := 1
	logoLeft := (m.width - logoWidth) / 2

	formLines := strings.Split(formView, "\n")
	formWidth := 0
	for _, line := range formLines {
		if w := ansi.StringWidth(line); w > formWidth {
			formWidth = w
		}
	}
	formTop := (middleHeight - len(formLines)) / 2
	formLeft := (m.width - formWidth) / 2

	var sb strings.Builder
	for row := range middleHeight {
		logoRow := row - logoTop
		formRow := row - formTop

		inLogo := logoRow >= 0 && logoRow < len(logoLines)
		inForm := formRow >= 0 && formRow < len(formLines)

		switch {
		case inForm:
			sb.WriteString(m.starfield.RenderRow(row, 0, formLeft))
			fgLine := formLines[formRow]
			if w := ansi.StringWidth(fgLine); w < formWidth {
				fgLine += strings.Repeat(" ", formWidth-w)
			}
			sb.WriteString(fgLine)
			sb.WriteString(m.starfield.RenderRow(row, formLeft+formWidth, m.width))

		case inLogo:
			sb.WriteString(m.starfield.RenderRow(row, 0, logoLeft))
			fgLine := logoLines[logoRow]
			if w := ansi.StringWidth(fgLine); w < logoWidth {
				fgLine += strings.Repeat(" ", logoWidth-w)
			}
			sb.WriteString(fgLine)
			sb.WriteString(m.starfield.RenderRow(row, logoLeft+logoWidth, m.width))

		default:
			sb.WriteString(m.starfield.RenderFullRow(row))
		}

		if row < middleHeight-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// renderMiddleWithStarfield composites the content view over the starfield background.
func (m Install) renderMiddleWithStarfield(contentView string, middleHeight int) string {
	m.starfield.ComputeGrid()

	fgLines := strings.Split(contentView, "\n")
	fgHeight := len(fgLines)
	fgWidth := 0
	for _, line := range fgLines {
		if w := ansi.StringWidth(line); w > fgWidth {
			fgWidth = w
		}
	}

	topOffset := (middleHeight - fgHeight) / 2
	leftOffset := (m.width - fgWidth) / 2

	var sb strings.Builder
	for row := range middleHeight {
		fgRow := row - topOffset
		if fgRow >= 0 && fgRow < fgHeight {
			sb.WriteString(m.starfield.RenderRow(row, 0, leftOffset))

			fgLine := fgLines[fgRow]
			if w := ansi.StringWidth(fgLine); w < fgWidth {
				fgLine += strings.Repeat(" ", fgWidth-w)
			}
			sb.WriteString(fgLine)

			sb.WriteString(m.starfield.RenderRow(row, leftOffset+fgWidth, m.width))
		} else {
			sb.WriteString(m.starfield.RenderFullRow(row))
		}
		if row < middleHeight-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
