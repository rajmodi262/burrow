package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time

type model struct {
	pid  int
	snap snapshot
}

func runTUI(pid int) {
	p := tea.NewProgram(model{pid: pid, snap: gather(pid)}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "burrow inspect:", err)
		os.Exit(1)
	}
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd { return tick() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		m.snap = gather(m.pid)
		if m.snap.err != nil {
			return m, tea.Quit
		}
		return m, tick()
	}
	return m, nil
}

func (m model) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(" Burrow Inspector ")
	help := lipgloss.NewStyle().Faint(true).Render("refreshing every 0.5s  -  press q to quit")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1)
	return box.Render(title + "\n\n" + m.snap.render() + "\n" + help)
}
