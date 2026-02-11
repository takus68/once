package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	m.Run()
}

func TestForm_FocusCycling(t *testing.T) {
	form := NewForm("Submit",
		FormItem{Label: "First", Field: NewTextField("first")},
		FormItem{Label: "Second", Field: NewTextField("second")},
	)
	assert.Equal(t, 0, form.Focused())

	form = formPressTab(form)
	assert.Equal(t, 1, form.Focused())

	form = formPressTab(form)
	assert.Equal(t, 2, form.Focused(), "submit button")

	form = formPressTab(form)
	assert.Equal(t, 3, form.Focused(), "cancel button")

	form = formPressTab(form)
	assert.Equal(t, 0, form.Focused(), "wraps to first field")
}

func TestForm_ShiftTabCycling(t *testing.T) {
	form := NewForm("Submit",
		FormItem{Label: "First", Field: NewTextField("first")},
		FormItem{Label: "Second", Field: NewTextField("second")},
	)

	form = formPressShiftTab(form)
	assert.Equal(t, 3, form.Focused(), "cancel button")

	form = formPressShiftTab(form)
	assert.Equal(t, 2, form.Focused(), "submit button")

	form = formPressShiftTab(form)
	assert.Equal(t, 1, form.Focused())

	form = formPressShiftTab(form)
	assert.Equal(t, 0, form.Focused())
}

func TestForm_EnterAdvancesFocus(t *testing.T) {
	form := NewForm("Submit",
		FormItem{Label: "First", Field: NewTextField("first")},
		FormItem{Label: "Second", Field: NewTextField("second")},
	)

	form = formPressEnter(form)
	assert.Equal(t, 1, form.Focused())

	form = formPressEnter(form)
	assert.Equal(t, 2, form.Focused(), "submit button")
}

func TestForm_SubmitAction(t *testing.T) {
	form := NewForm("Done",
		FormItem{Label: "Field", Field: NewTextField("val")},
	)

	form = formPressTab(form)
	assert.Equal(t, 1, form.Focused(), "submit button")

	_, action, _ := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, FormSubmitted, action)
}

func TestForm_CancelAction(t *testing.T) {
	form := NewForm("Done",
		FormItem{Label: "Field", Field: NewTextField("val")},
	)

	form = formPressTab(form)
	form = formPressTab(form)
	assert.Equal(t, 2, form.Focused(), "cancel button")

	_, action, _ := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, FormCancelled, action)
}

func TestForm_NoFields(t *testing.T) {
	form := NewForm("Done")
	assert.Equal(t, 0, form.Focused(), "submit button")

	form = formPressTab(form)
	assert.Equal(t, 1, form.Focused(), "cancel button")

	form = formPressTab(form)
	assert.Equal(t, 0, form.Focused(), "wraps to submit")
}

func TestTextField_DigitsOnly(t *testing.T) {
	field := NewTextField("number")
	field.SetDigitsOnly(true)
	field.Focus()

	field.Update(tea.KeyPressMsg{Code: '5', Text: "5"})
	assert.Equal(t, "5", field.Value())

	field.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.Equal(t, "5", field.Value(), "non-digit rejected")

	field.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	assert.Equal(t, "53", field.Value())
}

func TestCheckboxField_Toggle(t *testing.T) {
	field := NewCheckboxField("Enable", false)
	assert.False(t, field.Checked())

	field.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.True(t, field.Checked())

	field.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.False(t, field.Checked())
}

func TestCheckboxField_View(t *testing.T) {
	field := NewCheckboxField("TLS", true)
	assert.Equal(t, "[✓] TLS", field.View())

	field.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.Equal(t, "[ ] TLS", field.View())
}

func TestCheckboxField_DisabledWhen(t *testing.T) {
	disabled := true
	field := NewCheckboxField("TLS", false)
	field.SetDisabledWhen(func() (bool, string) {
		return disabled, "Not available"
	})

	field.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.False(t, field.Checked(), "toggle ignored when disabled")
	assert.Equal(t, "Not available", field.View())

	disabled = false
	field.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.True(t, field.Checked(), "toggle works when enabled")
	assert.Equal(t, "[✓] TLS", field.View())
}

func TestForm_FieldValuesAccessible(t *testing.T) {
	form := NewForm("Submit",
		FormItem{Label: "Name", Field: NewTextField("name")},
	)

	formTypeText(&form, "hello")
	assert.Equal(t, "hello", form.TextField(0).Value())
}

func TestForm_ClickFieldFocuses(t *testing.T) {
	form := NewForm("Submit",
		FormItem{Label: "First", Field: NewTextField("first")},
		FormItem{Label: "Second", Field: NewTextField("second")},
	)
	assert.Equal(t, 0, form.Focused())

	zone.Scan(form.View())
	time.Sleep(15 * time.Millisecond)

	zi := zone.Get(form.fieldZoneID(1))
	require.False(t, zi.IsZero())

	form, action, _ := form.Update(tea.MouseClickMsg{X: zi.StartX, Y: zi.StartY, Button: tea.MouseLeft})
	assert.Equal(t, FormNoAction, action)
	assert.Equal(t, 1, form.Focused())
}

func TestForm_ClickSubmit(t *testing.T) {
	form := NewForm("Done",
		FormItem{Label: "Field", Field: NewTextField("val")},
	)

	zone.Scan(form.View())
	time.Sleep(15 * time.Millisecond)

	zi := zone.Get(form.submitZoneID())
	require.False(t, zi.IsZero())

	_, action, _ := form.Update(tea.MouseClickMsg{X: zi.StartX, Y: zi.StartY, Button: tea.MouseLeft})
	assert.Equal(t, FormSubmitted, action)
}

func TestForm_ClickCancel(t *testing.T) {
	form := NewForm("Done",
		FormItem{Label: "Field", Field: NewTextField("val")},
	)

	zone.Scan(form.View())
	time.Sleep(15 * time.Millisecond)

	zi := zone.Get(form.cancelZoneID())
	require.False(t, zi.IsZero())

	_, action, _ := form.Update(tea.MouseClickMsg{X: zi.StartX, Y: zi.StartY, Button: tea.MouseLeft})
	assert.Equal(t, FormCancelled, action)
}

// Helpers

func formPressTab(form Form) Form {
	form, _, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	return form
}

func formPressShiftTab(form Form) Form {
	form, _, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	return form
}

func formPressEnter(form Form) Form {
	form, _, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return form
}

func formTypeText(form *Form, text string) {
	for _, r := range text {
		*form, _, _ = form.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}
