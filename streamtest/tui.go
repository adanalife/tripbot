package main

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewState int

const (
	viewList viewState = iota
	viewRunning
	viewPrompt
	viewSummary
	viewHelp
)

type runDoneMsg struct {
	result Result
}

type Model struct {
	cmds     []Command
	included []bool
	results  map[string]Result
	cursor   int
	state    viewState
	prevView viewState

	promptIdx int

	runner *Runner
	log    *Log

	channel   string
	bot       string
	login     string
	sessionID string
	version   string

	notesBuf    string
	commentMode bool

	status string
	err    error

	width, height int
}

func NewModel(runner *Runner, log *Log, prior []Result, channel, bot, login, sessionID, version string) *Model {
	m := &Model{
		cmds:      Catalog,
		included:  make([]bool, len(Catalog)),
		results:   map[string]Result{},
		runner:    runner,
		log:       log,
		channel:   channel,
		bot:       bot,
		login:     login,
		sessionID: sessionID,
		version:   version,
		state:     viewList,
	}
	for i := range m.included {
		m.included[i] = true
	}
	for _, r := range prior {
		m.results[r.Trigger] = r
	}
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case runDoneMsg:
		return m.handleRunDone(msg.result)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.state {
	case viewList:
		return m.keyList(msg)
	case viewPrompt:
		return m.keyPrompt(msg)
	case viewSummary:
		return m.keySummary(msg)
	case viewHelp:
		m.state = m.prevView
		return m, nil
	case viewRunning:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) keyList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.cmds)-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.cmds) - 1
	case " ":
		m.included[m.cursor] = !m.included[m.cursor]
	case "a":
		for i := range m.included {
			m.included[i] = true
		}
	case "n":
		for i := range m.included {
			m.included[i] = false
		}
	case "enter", "r":
		if !m.included[m.cursor] {
			m.status = "row not included; press space to include"
			return m, nil
		}
		m.status = ""
		m.notesBuf = ""
		m.commentMode = false
		m.state = viewRunning
		return m, m.runIdx(m.cursor)
	case "tab":
		m.prevView = viewList
		m.state = viewSummary
	case "?":
		m.prevView = viewList
		m.state = viewHelp
	}
	return m, nil
}

func (m *Model) keyPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commentMode {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyEnter:
			m.commentMode = false
		case tea.KeyBackspace, tea.KeyDelete:
			if r := []rune(m.notesBuf); len(r) > 0 {
				m.notesBuf = string(r[:len(r)-1])
			}
		case tea.KeySpace:
			m.notesBuf += " "
		case tea.KeyRunes:
			m.notesBuf += string(msg.Runes)
		}
		return m, nil
	}

	cmd := m.cmds[m.promptIdx]
	res, ok := m.results[cmd.Trigger]
	if !ok {
		res = Result{Trigger: cmd.Trigger}
	}
	finalize := func(status string) (tea.Model, tea.Cmd) {
		res.Status = status
		res.Notes = strings.TrimSpace(m.notesBuf)
		m.results[cmd.Trigger] = res
		if m.log != nil {
			_ = m.log.Append(m.stamp(res))
		}
		m.notesBuf = ""
		m.commentMode = false
		m.advanceCursor()
		m.state = viewList
		return m, nil
	}

	switch msg.String() {
	case "y":
		return finalize("manual-pass")
	case "n":
		return finalize("manual-fail")
	case "s":
		return finalize("skip")
	case "enter":
		return finalize(res.Status)
	case "r":
		m.notesBuf = ""
		m.commentMode = false
		m.state = viewRunning
		return m, m.runIdx(m.promptIdx)
	case "c":
		m.commentMode = true
	case "esc":
		m.notesBuf = ""
		m.commentMode = false
		m.state = viewList
	case "q":
		return m, tea.Quit
	case "?":
		m.prevView = viewPrompt
		m.state = viewHelp
	}
	return m, nil
}

func (m *Model) keySummary(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "l", "tab", "enter":
		m.state = viewList
	case "?":
		m.prevView = viewSummary
		m.state = viewHelp
	}
	return m, nil
}

func (m *Model) runIdx(idx int) tea.Cmd {
	cmd := m.cmds[idx]
	return func() tea.Msg {
		res := m.runner.Run(cmd, cmd.SampleParams)
		return runDoneMsg{result: res}
	}
}

func (m *Model) handleRunDone(res Result) (tea.Model, tea.Cmd) {
	m.results[res.Trigger] = res
	m.promptIdx = m.indexOf(res.Trigger)
	m.notesBuf = ""
	m.commentMode = false
	m.state = viewPrompt
	return m, nil
}

func (m *Model) advanceCursor() {
	for i := m.cursor + 1; i < len(m.cmds); i++ {
		if m.included[i] {
			m.cursor = i
			return
		}
	}
}

func (m *Model) stamp(r Result) Result {
	r.SessionID = m.sessionID
	r.Login = m.login
	r.Channel = m.channel
	r.Bot = m.bot
	if r.Version == "" {
		r.Version = m.version
	}
	return r
}

func (m *Model) indexOf(trigger string) int {
	for i := range m.cmds {
		if m.cmds[i].Trigger == trigger {
			return i
		}
	}
	return 0
}

var (
	styleHeader  = lipgloss.NewStyle().Bold(true).Underline(true)
	styleCursor  = lipgloss.NewStyle().Bold(true)
	styleDim     = lipgloss.NewStyle().Faint(true)
	stylePass    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleFail    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleSkip    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleManual  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleCategory = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

func statusGlyph(status string) string {
	switch status {
	case "pass", "manual-pass":
		return stylePass.Render("[+]")
	case "fail", "timeout", "manual-fail":
		return styleFail.Render("[!]")
	case "skip":
		return styleSkip.Render("[-]")
	case "pending-manual":
		return styleManual.Render("[?]")
	}
	return styleDim.Render("[ ]")
}

func (m *Model) View() string {
	switch m.state {
	case viewHelp:
		return m.viewHelp()
	case viewSummary:
		return m.viewSummary()
	case viewPrompt:
		return m.viewPrompt()
	default:
		return m.viewList()
	}
}

func (m *Model) viewList() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("streamtest — tripbot command coverage"))
	b.WriteString("\n")
	b.WriteString(styleDim.Render(fmt.Sprintf("as %s in #%s · listening for %s", m.login, m.channel, m.bot)))
	b.WriteString("\n\n")
	for i, cmd := range m.cmds {
		mark := " "
		if m.included[i] {
			mark = "*"
		}
		row := fmt.Sprintf(" %s %s %-22s %s",
			statusGlyph(m.results[cmd.Trigger].Status),
			mark,
			cmd.Trigger,
			styleCategory.Render(cmd.Category),
		)
		if cmd.OnscreenEffect != "" {
			row += styleDim.Render(" ·" + cmd.OnscreenEffect)
		}
		if i == m.cursor && m.state == viewList {
			b.WriteString(styleCursor.Render("> " + row))
		} else {
			b.WriteString("  " + row)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.state == viewRunning {
		cur := m.cmds[m.cursor]
		b.WriteString(styleManual.Render(fmt.Sprintf("running %s …", cur.Trigger)))
	} else {
		b.WriteString(styleDim.Render("↑/↓ move · space toggle · enter run focused · a all · n none · tab summary · ? help · q quit"))
	}
	if m.status != "" {
		b.WriteString("\n" + styleDim.Render(m.status))
	}
	return b.String()
}

func (m *Model) viewPrompt() string {
	cmd := m.cmds[m.promptIdx]
	res := m.results[cmd.Trigger]
	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("review: %s", cmd.Trigger)))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("auto status: %s %s\n", statusGlyph(res.Status), styleDim.Render(res.Status)))
	if cmd.OnscreenEffect != "" {
		b.WriteString(fmt.Sprintf("expected onscreen effect: %s\n", styleManual.Render(cmd.OnscreenEffect)))
	}
	if res.BotReply != "" {
		b.WriteString(fmt.Sprintf("bot replied: %q\n", res.BotReply))
	} else {
		b.WriteString(styleDim.Render("(no chat reply within timeout)") + "\n")
	}
	b.WriteString("\n")
	if m.commentMode {
		b.WriteString(styleManual.Render("notes (typing)") + ": " + m.notesBuf + "_\n")
		b.WriteString("\n" + styleDim.Render("type to edit · esc/enter exit comment mode"))
	} else {
		notesShown := m.notesBuf
		if notesShown == "" {
			notesShown = styleDim.Render("(none — press c to add)")
		}
		b.WriteString(styleDim.Render("notes") + ": " + notesShown + "\n")
		b.WriteString("\n" + styleDim.Render("enter accept · y pass · n fail · s skip · c comment · r re-run · esc back · q quit"))
	}
	return b.String()
}

func (m *Model) viewSummary() string {
	type bucket struct {
		category string
		pass, fail, skip, manual, missing int
	}
	byCat := map[string]*bucket{}
	for _, c := range m.cmds {
		b, ok := byCat[c.Category]
		if !ok {
			b = &bucket{category: c.Category}
			byCat[c.Category] = b
		}
		switch m.results[c.Trigger].Status {
		case "pass", "manual-pass":
			b.pass++
		case "fail", "timeout", "manual-fail":
			b.fail++
		case "skip":
			b.skip++
		case "pending-manual":
			b.manual++
		default:
			b.missing++
		}
	}
	keys := make([]string, 0, len(byCat))
	for k := range byCat {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(styleHeader.Render("summary"))
	b.WriteString("\n\n")
	for _, k := range keys {
		bk := byCat[k]
		b.WriteString(fmt.Sprintf("  %s  %s %d  %s %d  %s %d  %s %d  %s %d\n",
			styleCategory.Render(fmt.Sprintf("%-18s", bk.category)),
			stylePass.Render("pass"), bk.pass,
			styleFail.Render("fail"), bk.fail,
			styleSkip.Render("skip"), bk.skip,
			styleManual.Render("?"), bk.manual,
			styleDim.Render("--"), bk.missing,
		))
	}
	b.WriteString("\n" + styleDim.Render("esc/l/tab/enter back to list · q quit · ? help"))
	return b.String()
}

func (m *Model) viewHelp() string {
	help := []string{
		"list view",
		"  ↑/k, ↓/j     move cursor",
		"  g / G        first / last",
		"  space        toggle include for row",
		"  a / n        include all / none",
		"  enter, r     run focused row",
		"  tab          jump to summary",
		"  ?            help",
		"  q, ctrl+c    quit",
		"",
		"review screen (after each run)",
		"  enter        accept auto status, advance",
		"  y / n        override as pass / fail",
		"  s            skip",
		"  c            edit notes (type, esc/enter to exit)",
		"  r            re-run this command",
		"  esc          back to list without saving",
		"",
		"summary view",
		"  esc / l / tab / enter   back to list",
		"  q                       quit",
		"",
		"any key returns to previous screen from help.",
	}
	return styleHeader.Render("help") + "\n\n" + strings.Join(help, "\n")
}
