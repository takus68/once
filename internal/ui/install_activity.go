package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type installStage int

const (
	stagePreparing installStage = iota
	stageDownloading
	stageStarting
	stageVerifying
)

type installProgressMsg struct {
	stage      installStage
	percentage int
}

type installDoneMsg struct {
	app *docker.Application
	err error
}

type InstallActivityDoneMsg struct {
	App *docker.Application
}

type InstallActivityFailedMsg struct {
	Err error
}

type InstallActivity struct {
	namespace     *docker.Namespace
	imageRef      string
	hostname      string
	width, height int
	stage         installStage
	percentage    int
	progressBar   ProgressBar
	progressBusy  ProgressBusy
	progressChan  chan installProgressMsg
	doneChan      chan installDoneMsg
}

func NewInstallActivity(ns *docker.Namespace, imageRef, hostname string) InstallActivity {
	return InstallActivity{
		namespace:    ns,
		imageRef:     imageRef,
		hostname:     hostname,
		stage:        stagePreparing,
		progressChan: make(chan installProgressMsg, 10),
		doneChan:     make(chan installDoneMsg, 1),
	}
}

func (m InstallActivity) Init() tea.Cmd {
	return tea.Batch(m.progressBusy.Init(), m.startInstall(), m.waitForProgress())
}

func (m InstallActivity) Update(msg tea.Msg) (InstallActivity, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		progressWidth := min(m.width-4, 60)
		m.progressBar = NewProgressBar(progressWidth, Colors.Primary)
		m.progressBar.Total = 100
		m.progressBusy = NewProgressBusy(progressWidth, Colors.Primary)

	case installProgressMsg:
		m.stage = msg.stage
		m.percentage = msg.percentage
		m.progressBar.Current = float64(msg.percentage)
		if msg.stage == stageStarting || msg.stage == stageVerifying {
			return m, tea.Batch(m.progressBusy.Init(), m.waitForProgress())
		}
		return m, m.waitForProgress()

	case installDoneMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return InstallActivityFailedMsg{Err: msg.err} }
		}
		return m, func() tea.Msg { return InstallActivityDoneMsg{App: msg.app} }

	case progressBusyTickMsg:
		var cmd tea.Cmd
		m.progressBusy, cmd = m.progressBusy.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m InstallActivity) View() string {
	var status string
	switch m.stage {
	case stagePreparing:
		status = "Preparing..."
	case stageDownloading:
		status = "Downloading..."
	case stageStarting:
		status = "Starting..."
	case stageVerifying:
		status = "Verifying..."
	}

	statusLine := Styles.CenteredLine(m.width, status)

	var progressView string
	switch m.stage {
	case stagePreparing, stageStarting, stageVerifying:
		progressView = Styles.CenteredLine(m.width, m.progressBusy.View())
	case stageDownloading:
		progressView = Styles.CenteredLine(m.width, m.progressBar.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, progressView)
}

// Private

func (m InstallActivity) startInstall() tea.Cmd {
	return func() tea.Msg {
		go m.runInstall()
		return nil
	}
}

func (m InstallActivity) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		select {
		case progress, ok := <-m.progressChan:
			if ok {
				return progress
			}
		case done := <-m.doneChan:
			return done
		}
		return nil
	}
}

func (m InstallActivity) runInstall() {
	ctx := context.Background()

	m.progressChan <- installProgressMsg{stage: stagePreparing}

	if err := m.namespace.Setup(ctx); err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: 0}

	appName := docker.NameFromImageRef(m.imageRef)
	hostname := m.hostname
	if hostname == "" {
		hostname = appName + ".localhost"
	}

	app := m.namespace.AddApplication(docker.ApplicationSettings{
		Name:  appName,
		Image: m.imageRef,
		Host:  hostname,
	})

	progress := func(p docker.DeployProgress) {
		switch p.Stage {
		case docker.DeployStageDownloading:
			m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: p.Percentage}
		case docker.DeployStageStarting:
			m.progressChan <- installProgressMsg{stage: stageStarting, percentage: 100}
		}
	}

	if err := app.Deploy(ctx, progress); err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageVerifying}

	if err := app.VerifyHTTP(ctx); err != nil {
		app.Destroy(ctx, true)
		m.doneChan <- installDoneMsg{err: err}
		return
	}

	m.doneChan <- installDoneMsg{app: app}
}
