package app

import (
	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/i18n"
	"github.com/bankai-tensei/nople/internal/pty"
	"strings"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/field"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// settingsID is the panel of the settings in the dock.
const settingsID = "settings"

// The settings show in a tab of their own, where each change takes effect at
// once and is written to settings.json; the file can be edited by hand too,
// and the tab follows it.

// openSettingsTab shows the tab of the settings, opening it beside the
// editors if it is not open.
func (a *app) openSettingsTab() {
	if !a.settingsOpen {
		a.settingsOpen = true
		a.refresh()
		a.dock.Move(settingsID, a.anchor(), nav.DockCenter)
	}
	a.dock.Activate(settingsID)
	a.dock.HideFlyout()
	a.refresh()
}

// settingsClosed takes in that the user closed the tab of the settings.
func (a *app) settingsClosed() {
	a.settingsOpen = false
	if len(a.tabs) == 0 {
		arr := a.dock.Arrangement()
		arr.Root = prune(removePanel(cloneNode(arr.Root), settingsID))
		a.dock.SetArrangement(placeInEditors(arr, welcomeID, ""))
	}
	a.refresh()
}

// changeSettings changes the settings by fn, puts them into effect and writes
// them to their file.
func (a *app) changeSettings(fn func(s *config.Settings)) {
	s := a.settings
	fn(&s)
	old := a.settings
	a.settings = s
	if s.Language != old.Language {
		i18n.Set(s.Language)
	}
	if s.FontSize != old.FontSize {
		a.fontSize = s.FontSize
	}
	if s.Wrap != old.Wrap {
		a.wrap = s.Wrap
	}
	if s.Shell != old.Shell && a.shellOverride == nil {
		a.shellCmd = splitFields(s.Shell)
	}
	if s.Gitignore != old.Gitignore {
		a.followGitignore()
	}
	if err := a.saveSettings(); err != nil {
		a.toast(err.Error())
	}
	a.refresh()
}

// settingsView is the content of the tab of the settings.
type settingsView struct {
	core.StatelessBase
	app *app
}

// Build implements core.StatelessWidget.
func (v settingsView) Build(ctx core.BuildContext) core.Widget {
	a := v.app
	th := basic.ThemeOf(ctx)
	s := a.settings

	langs := []field.DropdownOption[string]{{Value: "system", Label: T("lang.system")}}
	for _, id := range i18n.Languages() {
		langs = append(langs, field.DropdownOption[string]{Value: id, Label: i18n.Name(id)})
	}
	themes := []string{"system", "light", "dark"}
	themeIndex := 0
	for i, t := range themes {
		if t == s.Theme {
			themeIndex = i
		}
	}
	autosaves := []field.DropdownOption[string]{
		{Value: config.AutosaveAfterDelay, Label: T("settings.autosaveAfterDelay")},
		{Value: config.AutosaveFocusChange, Label: T("settings.autosaveFocus")},
		{Value: config.AutosaveOff, Label: T("settings.autosaveOff")},
	}

	appearance := collection.Group{Title: T("settings.appearance"), Children: []core.Widget{
		collection.GroupRow{Label: T("settings.language"), Child: field.DropdownMenu[string]{
			Value: orDefault(s.Language, "system"), Options: langs,
			OnChange: func(id string) { a.changeSettings(func(s *config.Settings) { s.Language = id }) },
		}},
		collection.GroupRow{Label: T("settings.theme"), Child: control.SegmentedControl{
			Options:  []string{T("settings.themeSystem"), T("settings.themeLight"), T("settings.themeDark")},
			Selected: themeIndex, Label: T("settings.theme"),
			OnChange: func(i int) { a.changeSettings(func(s *config.Settings) { s.Theme = themes[i] }) },
		}},
	}}
	editorGroup := collection.Group{Title: T("settings.editor"), Children: []core.Widget{
		collection.GroupRow{Label: T("settings.fontFamily"), Detail: T("settings.fontFamilyDetail"), Child: basic.SizedBox{Width: 220, Child: field.Input{
			Value: s.FontFamily, Placeholder: theme.MonoFamilies,
			OnSubmit: func(v string) {
				a.changeSettings(func(s *config.Settings) { s.FontFamily = orDefault(v, theme.MonoFamilies) })
			},
		}}},
		collection.GroupRow{Label: T("settings.fontSize"), Child: basic.SizedBox{Width: 120, Child: field.NumberField{
			Value: float64(s.FontSize), Min: 8, Max: 40, Step: 1,
			OnChange: func(v float64) { a.changeSettings(func(s *config.Settings) { s.FontSize = float32(v) }) },
		}}},
		collection.GroupRow{Label: T("settings.wrap"), Child: control.Switch{
			Value: s.Wrap, Label: T("settings.wrap"),
			OnChange: func(on bool) { a.changeSettings(func(s *config.Settings) { s.Wrap = on }) },
		}},
		collection.GroupRow{Label: T("settings.autosave"), Child: field.DropdownMenu[string]{
			Value: s.Autosave, Options: autosaves,
			OnChange: func(m string) { a.changeSettings(func(s *config.Settings) { s.Autosave = m }) },
		}},
	}}
	filesGroup := collection.Group{Title: T("settings.files"), Children: []core.Widget{
		collection.GroupRow{Label: T("settings.gitignore"), Detail: T("settings.gitignoreDetail"), Child: control.Switch{
			Value: s.Gitignore, Label: T("settings.gitignore"),
			OnChange: func(on bool) { a.changeSettings(func(s *config.Settings) { s.Gitignore = on }) },
		}},
	}}
	if s.Autosave == config.AutosaveAfterDelay {
		editorGroup.Children = append(editorGroup.Children, collection.GroupRow{
			Label: T("settings.autosaveDelay"), Detail: T("settings.autosaveDelayDetail"),
			Child: basic.SizedBox{Width: 120, Child: field.NumberField{
				Value: float64(s.AutosaveDelay), Min: 100, Max: 60000, Step: 100,
				OnChange: func(v float64) { a.changeSettings(func(s *config.Settings) { s.AutosaveDelay = int(v) }) },
			}},
		})
	}
	terminalGroup := collection.Group{Title: T("settings.terminal"), Children: []core.Widget{
		collection.GroupRow{Label: T("settings.shell"), Detail: T("settings.shellDetail"), Child: basic.SizedBox{Width: 260, Child: field.Input{
			Value: s.Shell, Placeholder: joinFields(pty.DefaultShell()),
			OnSubmit: func(v string) { a.changeSettings(func(s *config.Settings) { s.Shell = v }) },
		}}},
	}}

	children := []core.Widget{
		basic.Text{Content: T("settings.title"), Style: th.Text.Title.WithColor(th.Color.Ink)},
		appearance, editorGroup, filesGroup, terminalGroup,
		basic.Row{Cross: layout.CrossCenter, Spacing: theme.Space12, Children: []core.Widget{
			control.Button{Label: T("settings.openJSON"), OnPress: a.openSettings},
			basic.Expanded{Child: basic.Text{Content: a.settingsPath, Style: th.Text.Caption.WithColor(th.Color.Ink2)}},
		}},
	}
	if a.settingsErr != nil {
		children = append(children[:1], append([]core.Widget{control.InfoBar{
			Open: true, Severity: control.SeverityError, Title: T("doc.settingsBroken"), Message: a.settingsErr.Error(),
		}}, children[1:]...)...)
	}
	return basic.Container{Color: th.Color.Canvas, Child: basic.Scroll{Child: basic.Center{Child: basic.ConstrainedBox{
		MaxWidth: 720, Child: basic.Padding{Insets: geom.Symmetric(theme.Space24, theme.Space24), Child: basic.Column{
			Size: layout.MainMin, Cross: layout.CrossStretch, Spacing: theme.Space20, Children: children,
		}},
	}}}}
}

// orDefault is s, or def if s is empty.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// splitFields splits a command line at its spaces; joinFields joins one.
func splitFields(s string) []string   { return strings.Fields(s) }
func joinFields(args []string) string { return strings.Join(args, " ") }
