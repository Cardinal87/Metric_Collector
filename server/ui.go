package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func ticker() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return t })
}

func initModel(readyChan chan Options) model {
	m := model{
		dots:      0,
		viewport:  viewport.New(0, 0),
		state:     -1,
		readyChan: readyChan,
	}
	return m
}

func restoreConnection(opt Options) tea.Cmd {
	return func() tea.Msg {
		if len(failed_nodes) == 0 {
			return "1-complete"
		}
		cached := make([]Node, len(failed_nodes))
		copy(cached, failed_nodes)
		clear(failed_nodes)

		var outerWg sync.WaitGroup
		for _, node := range cached {
			outerWg.Go(func() { handleNode(node, opt.stopChan, opt.wg, opt.methodConfig) })
		}
		outerWg.Wait()
		return "1-complete"
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg {
		opt := <-m.readyChan
		return ServerInitializedMessage{opt: opt}
	}, ticker())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Height = msg.Height
		m.viewport.Width = msg.Width
		content := lipgloss.NewStyle().Width(msg.Width).Render(m.rawLogs)
		m.viewport.SetContent(content)

	case ServerInitializedMessage:
		m.state = 0
		m.options = msg.opt
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		if msg.String() == "1" {
			m.state = 1
			return m, tea.Batch(tea.ClearScreen, ticker(), restoreConnection(m.options))
		}
		if msg.String() == "2" {
			m.state = 2
			return m, tea.ClearScreen
		}
		if msg.String() == "3" {
			m.state = 3

			bytes, _ := os.ReadFile("logs/app.log")
			rawContent := string(bytes)
			m.rawLogs = rawContent

			content := lipgloss.NewStyle().MaxWidth(m.viewport.Width).Render(rawContent)
			m.viewport.SetContent(content)
			return m, tea.ClearScreen
		}
		if msg.String() == "esc" && (m.state == 2 || m.state == 3) {
			m.state = 0
		}
		if m.state == 3 {
			viewport, cmd := m.viewport.Update(msg)
			m.viewport = viewport
			return m, cmd
		}
		return m, nil

	case string:
		if msg == "init-complete" {
			m.state = 0
			return m, tea.ClearScreen
		}
		if msg == "1-complete" {
			m.state = 0
			return m, tea.ClearScreen
		}

	case time.Time:
		m.dots = (m.dots + 1) % 4
		return m, ticker()

	}
	return m, nil
}

func (m model) View() string {
	s := ""
	switch m.state {
	case -1:
		s = "Server initialization" + strings.Repeat(".", m.dots)
	case 0:
		s = "Server managment commands:\n"
		s += "1 - Restore failed nodes\n"
		s += "2 - Show failed nodes\n"
		s += "3 - Show logs\n"
		s += "q - Quit\n"

	case 1:
		s = "Restoring connection" + strings.Repeat(".", m.dots)

	case 2:
		if len(failed_nodes) == 0 {
			s = "No one node is failed\n"
		} else {
			s = "Failed nodes:\n"
			for _, node := range failed_nodes {
				s += fmt.Sprintf("IP - %s\n", node.Ip)
			}
		}
		s += "Press ESC to exit"
	case 3:
		s = fmt.Sprintf("%s\nPress ESC to exit", m.viewport.View())
	}

	return s
}
