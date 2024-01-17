package agent

import (
	"fmt"
	"io"
	"strings"

	"github.com/buildkite/cli/v3/pkg/style"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

type itemStyles struct {
	normalStatus   lipgloss.Style
	selectedStatus lipgloss.Style
	dimmedStatus   lipgloss.Style

	normalName   lipgloss.Style
	selectedName lipgloss.Style
	dimmedName   lipgloss.Style

	normalVersion   lipgloss.Style
	selectedVersion lipgloss.Style
	dimmedVersion   lipgloss.Style

	normalQueue   lipgloss.Style
	selectedQueue lipgloss.Style
	dimmedQueue   lipgloss.Style

	filterMatch lipgloss.Style
}

func defaultItemStyles() (s itemStyles) {
	// apply a width of the longest expected string
	s.normalStatus = lipgloss.NewStyle().Width(len("connected"))
	s.selectedStatus = s.normalStatus.Copy()
	s.dimmedStatus = s.normalStatus.Copy()

	s.normalName = lipgloss.NewStyle().PaddingLeft(2)
	s.selectedName = s.normalName.Copy()
	s.dimmedName = s.normalName.Copy()

	s.normalVersion = s.normalName.Copy().Foreground(style.Grey) //.Width(len("v0.00.00"))
	s.selectedVersion = s.normalVersion.Copy()
	s.dimmedVersion = s.normalVersion.Copy()

	s.normalQueue = s.normalName.Copy().Foreground(style.Teal)
	s.selectedQueue = s.normalQueue.Copy()
	s.dimmedQueue = s.normalQueue.Copy()

	s.filterMatch = lipgloss.NewStyle().Underline(true)

	return
}

func NewDelegate() listAgentDelegate {
	return listAgentDelegate{
		Styles: defaultItemStyles(),
	}
}

// listAgentDelegate implements list.ItemDelegate to customise how each agent is rendered in a list
type listAgentDelegate struct {
	Styles itemStyles
}

// Height implements list.ItemDelegate.
func (listAgentDelegate) Height() int {
	return 1
}

// Render implements list.ItemDelegate.
// This is mostly a reimplementation of list.DefaultDelegate#Render
func (d listAgentDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var (
		status, name, version, queue string
		matchedRunes                 []int
		s                            = &d.Styles
	)
	if agent, ok := item.(AgentListItem); ok {
		name = *agent.Name
		status = *agent.ConnectedState
		version = *agent.Version
		queue = agent.QueueName()
	} else {
		return
	}

	// Prevent text from exceeding list width
	// TODO: add more truncation to name so other colums are displayed fully
	nameWidth := uint(m.Width() - s.normalName.GetPaddingLeft() - s.normalName.GetPaddingRight())
	name = truncate.StringWithTail(name, nameWidth, style.Ellipsis)
	status = s.normalStatus.Foreground(mapStatusToColour(status)).Render(status)
	version = s.normalVersion.Render(version)
	queue = s.normalQueue.Render(queue)

	// Conditions
	var (
		isSelected  = index == m.Index()
		emptyFilter = m.FilterState() == list.Filtering && m.FilterValue() == ""
		isFiltered  = m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied
	)

	if isFiltered && index < len(m.VisibleItems()) {
		// Get indices of matched characters
		matchedRunes = m.MatchesForItem(index)
	}

	if emptyFilter {
		name = s.dimmedName.Render(name)
		status = s.dimmedStatus.Render(status)
		version = s.dimmedVersion.Render(version)
		queue = s.dimmedQueue.Render(queue)
	} else if isSelected && m.FilterState() != list.Filtering {
		if isFiltered {
			// Highlight matches
			unmatched := s.selectedName.Inline(true)
			matched := unmatched.Copy().Inherit(s.filterMatch)
			name = lipgloss.StyleRunes(name, matchedRunes, matched, unmatched)
		}
		name = s.selectedName.Render(name)
		status = s.selectedStatus.Render(status)
		version = s.selectedVersion.Render(version)
		queue = s.selectedQueue.Render(queue)
	} else {
		if isFiltered {
			// Highlight matches
			unmatched := s.normalName.Inline(true)
			matched := unmatched.Copy().Inherit(s.filterMatch)
			name = lipgloss.StyleRunes(name, matchedRunes, matched, unmatched)
		}
		name = s.normalName.Render(name)
		status = s.normalStatus.Render(status)
		version = s.normalVersion.Render(version)
		queue = s.normalQueue.Render(queue)
	}

	fmt.Fprintf(w, "%s %s %s %s", status, name, version, queue)
}

// Spacing implements list.ItemDelegate.
func (listAgentDelegate) Spacing() int {
	return 0
}

// Update implements list.ItemDelegate.
func (listAgentDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func mapStatusToColour(s string) lipgloss.Color {
	switch strings.ToLower(s) {
	case "connected":
		return style.Green
	default:
		return style.Black
	}
}
