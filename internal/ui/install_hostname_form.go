package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
)

type InstallHostnameBackMsg struct{}

type InstallHostnameForm struct {
	form     Form
	imageRef string
}

func NewInstallHostnameForm(imageRef string) InstallHostnameForm {
	hostnameField := NewTextField("app.example.com")
	appName := docker.NameFromImageRef(imageRef)
	if appName != "" {
		hostnameField.SetPlaceholder(appName + ".example.com")
	}

	m := InstallHostnameForm{
		form: NewForm("Install",
			FormItem{
				Label:    "Hostname",
				Field:    hostnameField,
				Required: true,
			},
		),
		imageRef: imageRef,
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		return func() tea.Msg {
			return InstallFormSubmitMsg{
				ImageRef: imageRef,
				Hostname: f.TextField(0).Value(),
			}
		}
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return InstallHostnameBackMsg{} }
	})

	return m
}

func (m InstallHostnameForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m InstallHostnameForm) Update(msg tea.Msg) (InstallHostnameForm, tea.Cmd) {
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

func (m InstallHostnameForm) View() string {
	return m.form.View()
}

func (m InstallHostnameForm) Hostname() string {
	return m.form.TextField(0).Value()
}
