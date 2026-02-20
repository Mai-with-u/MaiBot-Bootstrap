package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"maibot/internal/version"
)

type tuiField struct {
	labelKey string
	def      string
}

type tuiAction struct {
	id       string
	labelKey string
	fields   []tuiField
	build    func([]string) []string
	quit     bool
}

type tuiMode int

const (
	tuiModeMenu tuiMode = iota
	tuiModeInput
)

type commandFinishedMsg struct {
	err error
}

type tuiModel struct {
	i18n          *tuiI18n
	actions       []tuiAction
	menuStack     [][]tuiAction
	rootMenu      []tuiAction
	instanceMenu  []tuiAction
	serviceMenu   []tuiAction
	modulesMenu   []tuiAction
	cursor        int
	mode          tuiMode
	activeAction  int
	fieldIndex    int
	fieldValues   []string
	statusMessage string
}

func (a *App) runInteractiveTUI() {
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		a.printHelp()
		return
	}
	model := newTUIModel(newTUII18n(a.cfg.Installer.Language))
	program := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout))
	if _, err := program.Run(); err != nil {
		a.log.Errorf("tui error: %v", err)
	}
}

func newTUIModel(i18n *tuiI18n) tuiModel {
	instanceMenu := []tuiAction{
		{id: "install", labelKey: "action.install", build: func(_ []string) []string { return []string{"install"} }},
		{id: "start", labelKey: "action.start", build: func(_ []string) []string { return []string{"start"} }},
		{id: "stop", labelKey: "action.stop", build: func(_ []string) []string { return []string{"stop"} }},
		{id: "restart", labelKey: "action.restart", build: func(_ []string) []string { return []string{"restart"} }},
		{id: "status", labelKey: "action.status", build: func(_ []string) []string { return []string{"status"} }},
		{id: "logs", labelKey: "action.logs", fields: []tuiField{{labelKey: "field.tail_lines", def: "50"}}, build: func(v []string) []string {
			return []string{"logs", "--tail", nonEmpty(v[0], "50")}
		}},
		{id: "update", labelKey: "action.update", build: func(_ []string) []string { return []string{"update"} }},
		{id: "back", labelKey: "action.back", build: func(_ []string) []string { return nil }},
	}

	serviceMenu := []tuiAction{
		{id: "service-install", labelKey: "action.service_install", build: func(_ []string) []string { return []string{"service", "install"} }},
		{id: "service-start", labelKey: "action.service_start", build: func(_ []string) []string { return []string{"service", "start"} }},
		{id: "service-stop", labelKey: "action.service_stop", build: func(_ []string) []string { return []string{"service", "stop"} }},
		{id: "service-status", labelKey: "action.service_status", build: func(_ []string) []string { return []string{"service", "status"} }},
		{id: "service-uninstall", labelKey: "action.service_uninstall", build: func(_ []string) []string { return []string{"service", "uninstall"} }},
		{id: "back", labelKey: "action.back", build: func(_ []string) []string { return nil }},
	}

	modulesMenu := []tuiAction{
		{id: "modules-list", labelKey: "action.modules_list", build: func(_ []string) []string { return []string{"modules", "list"} }},
		{id: "modules-install", labelKey: "action.modules_install", fields: []tuiField{{labelKey: "field.module_name", def: "napcat"}}, build: func(v []string) []string {
			return []string{"modules", "install", nonEmpty(v[0], "napcat")}
		}},
		{id: "back", labelKey: "action.back", build: func(_ []string) []string { return nil }},
	}

	rootMenu := []tuiAction{
		{id: "instances", labelKey: "menu.instances", build: func(_ []string) []string { return nil }},
		{id: "services", labelKey: "menu.services", build: func(_ []string) []string { return nil }},
		{id: "modules", labelKey: "menu.modules", build: func(_ []string) []string { return nil }},
		{id: "self-update", labelKey: "action.self_update", build: func(_ []string) []string { return []string{"upgrade"} }},
		{id: "version", labelKey: "action.version", build: func(_ []string) []string { return []string{"version"} }},
		{id: "help", labelKey: "action.help", build: func(_ []string) []string { return []string{"help"} }},
		{id: "quit", labelKey: "action.quit", quit: true, build: func(_ []string) []string { return nil }},
	}

	return tuiModel{
		i18n:         i18n,
		actions:      rootMenu,
		menuStack:    [][]tuiAction{rootMenu},
		rootMenu:     rootMenu,
		instanceMenu: instanceMenu,
		serviceMenu:  serviceMenu,
		modulesMenu:  modulesMenu,
		mode:         tuiModeMenu,
		activeAction: -1,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case tuiModeMenu:
			return m.updateMenu(msg)
		case tuiModeInput:
			return m.updateInput(msg)
		}
	case commandFinishedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf(m.i18n.T("status.command_failed"), msg.err)
		} else {
			m.statusMessage = m.i18n.T("status.command_completed")
		}
	}
	return m, nil
}

func (m tuiModel) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		m = m.popMenu()
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.actions)-1 {
			m.cursor++
		}
	case "enter":
		action := m.actions[m.cursor]
		if action.quit {
			return m, tea.Quit
		}
		if action.id == "instances" {
			return m.pushMenu(m.instanceMenu), nil
		}
		if action.id == "services" {
			return m.pushMenu(m.serviceMenu), nil
		}
		if action.id == "modules" {
			return m.pushMenu(m.modulesMenu), nil
		}
		if action.id == "back" {
			return m.popMenu(), nil
		}
		if len(action.fields) == 0 {
			args := action.build(nil)
			return m, runCommandCmd(args)
		}
		m.mode = tuiModeInput
		m.activeAction = m.cursor
		m.fieldIndex = 0
		m.fieldValues = make([]string, len(action.fields))
		for i := range action.fields {
			m.fieldValues[i] = action.fields[i].def
		}
	}
	return m, nil
}

func (m tuiModel) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.actions[m.activeAction]
	fieldValue := m.fieldValues[m.fieldIndex]

	switch msg.String() {
	case "esc":
		m.mode = tuiModeMenu
		m.activeAction = -1
		m.fieldValues = nil
		m.fieldIndex = 0
		return m, nil
	case "enter":
		if m.fieldIndex < len(action.fields)-1 {
			m.fieldIndex++
			return m, nil
		}
		args := action.build(m.fieldValues)
		m.mode = tuiModeMenu
		m.activeAction = -1
		m.fieldValues = nil
		m.fieldIndex = 0
		return m, runCommandCmd(args)
	case "backspace":
		if len(fieldValue) > 0 {
			m.fieldValues[m.fieldIndex] = fieldValue[:len(fieldValue)-1]
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.fieldValues[m.fieldIndex] += string(msg.Runes)
	}
	return m, nil
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(m.i18n.T("title") + "\n")
	b.WriteString(renderBanner())
	b.WriteString(m.i18n.T("hint") + "\n\n")

	if m.mode == tuiModeMenu {
		for i, action := range m.actions {
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			b.WriteString(fmt.Sprintf("%s %2d) %s\n", cursor, i+1, m.i18n.T(action.labelKey)))
		}
		b.WriteString("\n")
		b.WriteString(m.i18n.T("installer_version") + ": " + version.InstallerVersion + "\n")
		if strings.TrimSpace(m.statusMessage) != "" {
			b.WriteString(m.i18n.T("status_label") + ": " + m.statusMessage + "\n")
		}
		return b.String()
	}

	action := m.actions[m.activeAction]
	b.WriteString(m.i18n.T("action") + ": " + m.i18n.T(action.labelKey) + "\n")
	b.WriteString(m.i18n.T("input_hint") + "\n\n")
	for i, field := range action.fields {
		marker := " "
		if i == m.fieldIndex {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s: %s\n", marker, m.i18n.T(field.labelKey), m.fieldValues[i]))
	}
	return b.String()
}

func (m tuiModel) pushMenu(actions []tuiAction) tuiModel {
	if len(actions) == 0 {
		return m
	}
	m.menuStack = append(m.menuStack, actions)
	m.actions = actions
	m.cursor = 0
	return m
}

func (m tuiModel) popMenu() tuiModel {
	if len(m.menuStack) <= 1 {
		m.actions = m.rootMenu
		m.cursor = 0
		m.menuStack = [][]tuiAction{m.rootMenu}
		return m
	}
	m.menuStack = m.menuStack[:len(m.menuStack)-1]
	m.actions = m.menuStack[len(m.menuStack)-1]
	m.cursor = 0
	return m
}

func runCommandCmd(args []string) tea.Cmd {
	return tea.ExecProcess(execCommand(args), func(err error) tea.Msg {
		return commandFinishedMsg{err: err}
	})
}

func execCommand(args []string) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		cmd := exec.Command("sh", "-c", "exit 1")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func nonEmpty(v, def string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return def
	}
	return trimmed
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}

func renderBanner() string {
	lines := []string{
		" __  __       _ ____        _   ",
		"|  \\ /  | __ _| | __ )  ___ | |_ ",
		"| |\\/| |/ _` | |  _ \\ / _ \\| __|",
		"| |  | | (_| | | |_) | (_) | |_ ",
		"|_|  |_|\\__,_|_|____/ \\___/ \\__|",
	}
	return colorizeBanner(lines)
}

func colorizeBanner(lines []string) string {
	if !isTerminal(os.Stdout) {
		return strings.Join(lines, "\n") + "\n"
	}
	colors := []string{"34", "35", "36", "32", "33"}
	var out strings.Builder
	for i, line := range lines {
		code := colors[i%len(colors)]
		out.WriteString("\x1b[" + code + "m" + line + "\x1b[0m")
		out.WriteString("\n")
	}
	return out.String()
}
