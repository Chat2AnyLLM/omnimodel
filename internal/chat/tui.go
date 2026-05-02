package chat

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	tuiTitleStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")).Padding(0, 1)
	tuiUserLabelStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#06B6D4"))
	tuiAssistantLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A855F7"))
	tuiErrorStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	tuiHelpStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Padding(0, 1)
	tuiDivStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))
	ansiPrefixPattern      = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)*`)
)

type progReadyMsg struct{ p *tea.Program }
type streamDeltaMsg string
type streamDoneMsg struct{ err error }
type appendLineMsg string
type modelChangedMsg string
type openModelPickerMsg struct{ models []ModelInfo }

type modelPickerGroup struct {
	owner    string
	models   []ModelInfo
	expanded bool
}

type modelPickerEntry struct {
	isGroup bool
	owner   string
	model   ModelInfo
}

type modelPickerState struct {
	models       []ModelInfo
	filtered     []ModelInfo
	filter       string
	selectedIdx  int
	scrollOffset int
	groups       []modelPickerGroup
	entries      []modelPickerEntry
}

func newModelPickerState(models []ModelInfo) *modelPickerState {
	sorted := append([]ModelInfo(nil), models...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Owner != sorted[j].Owner {
			return sorted[i].Owner < sorted[j].Owner
		}
		return sorted[i].Selector < sorted[j].Selector
	})
	p := &modelPickerState{
		models:       sorted,
		filtered:     sorted,
		filter:       "",
		selectedIdx:  0,
		scrollOffset: 0,
	}
	p.rebuildGroups(true)
	return p
}

func (p *modelPickerState) rebuildGroups(collapsedByDefault bool) {
	prev := make(map[string]bool)
	if !collapsedByDefault {
		for _, group := range p.groups {
			prev[group.owner] = group.expanded
		}
	}

	groupMap := make(map[string]*modelPickerGroup)
	order := make([]string, 0)
	newGroups := make([]modelPickerGroup, 0)
	for _, model := range p.filtered {
		owner := model.ProviderName
		if owner == "" {
			owner = model.OwnerName
		}
		if owner == "" {
			owner = model.Owner
		}
		if owner == "" {
			owner = "Other"
		}
		group := groupMap[owner]
		if group == nil {
			expanded := true
			if collapsedByDefault {
				expanded = false
			} else if prevExpanded, ok := prev[owner]; ok {
				expanded = prevExpanded
			}
			newGroups = append(newGroups, modelPickerGroup{owner: owner, expanded: expanded})
			group = &newGroups[len(newGroups)-1]
			groupMap[owner] = group
			order = append(order, owner)
		}
		group.models = append(group.models, model)
	}
	p.groups = newGroups

	p.entries = p.entries[:0]
	for _, owner := range order {
		for i := range p.groups {
			group := &p.groups[i]
			if group.owner != owner {
				continue
			}
			p.entries = append(p.entries, modelPickerEntry{isGroup: true, owner: group.owner})
			if group.expanded {
				for _, model := range group.models {
					p.entries = append(p.entries, modelPickerEntry{isGroup: false, owner: group.owner, model: model})
				}
			}
			break
		}
	}

	if len(p.entries) == 0 {
		p.selectedIdx = 0
		p.scrollOffset = 0
		return
	}
	if p.selectedIdx >= len(p.entries) {
		p.selectedIdx = len(p.entries) - 1
	}
	if p.selectedIdx < 0 {
		p.selectedIdx = 0
	}
	if p.scrollOffset > p.selectedIdx {
		p.scrollOffset = p.selectedIdx
	}
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
}

func (p *modelPickerState) toggleSelectedGroup() {
	if len(p.entries) == 0 || p.selectedIdx >= len(p.entries) {
		return
	}
	entry := p.entries[p.selectedIdx]
	if !entry.isGroup {
		return
	}
	for i := range p.groups {
		if p.groups[i].owner == entry.owner {
			p.groups[i].expanded = !p.groups[i].expanded
			break
		}
	}
	p.rebuildGroups(false)
}

func (p *modelPickerState) selectedModel() (ModelInfo, bool) {
	if len(p.entries) == 0 || p.selectedIdx >= len(p.entries) {
		return ModelInfo{}, false
	}
	entry := p.entries[p.selectedIdx]
	if entry.isGroup {
		return ModelInfo{}, false
	}
	return entry.model, true
}

func (p *modelPickerState) updateFilter() {
	p.filtered = fuzzyFilterChatModels(p.models, p.filter)
	p.selectedIdx = 0
	p.scrollOffset = 0
	p.groups = nil
	p.entries = nil
	p.rebuildGroups(strings.TrimSpace(p.filter) == "")
}

type fuzzyModelMatch struct {
	model ModelInfo
	score int
}

func fuzzyFilterChatModels(models []ModelInfo, filter string) []ModelInfo {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return append([]ModelInfo(nil), models...)
	}

	matches := make([]fuzzyModelMatch, 0, len(models))
	for _, model := range models {
		score, ok := bestFuzzyModelScore(model, filter)
		if !ok {
			continue
		}
		matches = append(matches, fuzzyModelMatch{model: model, score: score})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].model.Owner != matches[j].model.Owner {
			return matches[i].model.Owner < matches[j].model.Owner
		}
		return matches[i].model.Selector < matches[j].model.Selector
	})

	filtered := make([]ModelInfo, 0, len(matches))
	for _, match := range matches {
		filtered = append(filtered, match.model)
	}
	return filtered
}

func bestFuzzyModelScore(model ModelInfo, filter string) (int, bool) {
	fields := []struct {
		text   string
		weight int
	}{
		{text: strings.ToLower(model.Selector), weight: 4000},
		{text: strings.ToLower(model.Name), weight: 3000},
		{text: strings.ToLower(model.Owner), weight: 2000},
		{text: strings.ToLower(model.ID), weight: 1000},
	}

	best := 0
	matched := false
	for _, field := range fields {
		if field.text == "" {
			continue
		}
		if score, ok := fuzzyScore(field.text, filter); ok {
			matched = true
			candidate := field.weight + score
			if candidate > best {
				best = candidate
			}
		}
	}
	return best, matched
}

func fuzzyScore(text, filter string) (int, bool) {
	if strings.HasPrefix(text, filter) {
		return 1000 - len(text) + len(filter), true
	}
	if idx := strings.Index(text, filter); idx >= 0 {
		return 900 - idx, true
	}

	last := -1
	gaps := 0
	for _, r := range filter {
		idx := strings.IndexRune(text[last+1:], r)
		if idx < 0 {
			return 0, false
		}
		actual := last + 1 + idx
		if last >= 0 {
			gaps += actual - last - 1
		}
		last = actual
	}
	return 500 - gaps - len(text), true
}

type chatTUIModel struct {
	client    Client
	sessionID string
	model     string
	prog      *tea.Program

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	spinning bool

	lines        []string
	streamActive bool
	streamBuf    string
	picker       *modelPickerState

	promptHistory       []string
	historyIndex        int
	historyDraft        string
	historySearchMode   bool
	historySearchQuery  string
	historySearchCursor int

	width      int
	height     int
	ready      bool
	mdRenderer *glamour.TermRenderer
}

func newChatTUIModel(c Client, sessionID, model string, history []Message) chatTUIModel {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A855F7"))
	sp.Spinner = spinner.Dot

	ta := textarea.New()
	ta.Placeholder = "Type a message… (Enter to send, Shift+Enter for newline)"
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true)

	mdR, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(80))

	m := chatTUIModel{client: c, sessionID: sessionID, model: model, spinner: sp, textarea: ta, mdRenderer: mdR, historyIndex: -1, historySearchCursor: -1}
	for _, msg := range history {
		switch msg.Role {
		case "user":
			m.lines = append(m.lines, tuiUserLabelStyle.Render("You>"), m.renderMessageBody(msg.Content), "")
			m.recordPromptHistory(msg.Content)
		case "assistant":
			rendered := m.renderMD(msg.Content)
			m.lines = append(m.lines, tuiAssistantLabelStyle.Render("Assistant>"), m.renderMessageBody(rendered), "")
		}
	}
	return m
}

func (m chatTUIModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m chatTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progReadyMsg:
		m.prog = msg.p
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpH := msg.Height - 9
		if vpH < 3 {
			vpH = 3
		}
		if !m.ready {
			m.viewport = viewport.New(msg.Width-2, vpH)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 2
			m.viewport.Height = vpH
		}
		m.textarea.SetWidth(msg.Width - 4)
		if m.mdRenderer != nil {
			m.mdRenderer, _ = glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(msg.Width-6))
		}
		m.syncViewport()
		return m, nil
	case tea.KeyMsg:
		if m.picker != nil {
			visibleItems := 20
			if len(msg.Runes) == 1 && msg.Runes[0] == ' ' {
				m.picker.toggleSelectedGroup()
				return m, nil
			}
			switch msg.Type {
			case tea.KeyEscape:
				m.picker = nil
				return m, nil
			case tea.KeyUp:
				if m.picker.selectedIdx > 0 {
					m.picker.selectedIdx--
					if m.picker.selectedIdx < m.picker.scrollOffset {
						m.picker.scrollOffset = m.picker.selectedIdx
					}
				}
				return m, nil
			case tea.KeyDown:
				if m.picker.selectedIdx < len(m.picker.entries)-1 {
					m.picker.selectedIdx++
					if m.picker.selectedIdx >= m.picker.scrollOffset+visibleItems {
						m.picker.scrollOffset = m.picker.selectedIdx - visibleItems + 1
					}
				}
				return m, nil
			case tea.KeySpace:
				m.picker.toggleSelectedGroup()
				return m, nil
			case tea.KeyEnter:
				model, ok := m.picker.selectedModel()
				if !ok {
					return m, nil
				}
				return m, func() tea.Msg {
					if err := UpdateSessionModel(m.client, m.sessionID, model.Selector); err != nil {
						return appendLineMsg(tuiErrorStyle.Render("Error: " + err.Error()))
					}
					return modelChangedMsg(model.Selector)
				}
			case tea.KeyBackspace, tea.KeyDelete:
				if len(m.picker.filter) > 0 {
					m.picker.filter = m.picker.filter[:len(m.picker.filter)-1]
					m.picker.updateFilter()
				}
				return m, nil
			default:
				if len(msg.Runes) > 0 {
					for _, r := range msg.Runes {
						if r == ' ' {
							continue
						}
						m.picker.filter += string(r)
					}
					m.picker.updateFilter()
				}
				return m, nil
			}
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.streamActive {
				m.streamActive = false
				m.spinning = false
				m.streamBuf = ""
				if len(m.lines) > 0 && m.lines[len(m.lines)-1] != "" {
					m.lines = append(m.lines[:len(m.lines)-1], "")
				}
				m.syncViewport()
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEscape:
			if m.historySearchMode {
				m.exitHistorySearch()
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyCtrlR:
			if !m.textarea.Focused() || m.streamActive {
				break
			}
			m.enterHistorySearch()
			return m, nil
		case tea.KeyUp, tea.KeyCtrlP:
			if !m.textarea.Focused() || m.streamActive {
				break
			}
			if m.historySearchMode {
				m.applyHistorySearchResult()
				return m, nil
			}
			m.cyclePromptHistory(-1)
			return m, nil
		case tea.KeyDown, tea.KeyCtrlN:
			if !m.textarea.Focused() || m.streamActive {
				break
			}
			if m.historySearchMode {
				cursor := m.historySearchCursor
				query := strings.ToLower(strings.TrimSpace(m.historySearchQuery))
				for i := cursor + 1; i < len(m.promptHistory); i++ {
					if query == "" || strings.Contains(strings.ToLower(m.promptHistory[i]), query) {
						m.historySearchCursor = i
						m.applyTextareaValue(m.promptHistory[i])
						return m, nil
					}
				}
				return m, nil
			}
			m.cyclePromptHistory(1)
			return m, nil
		case tea.KeyEnter:
			if m.historySearchMode {
				m.exitHistorySearch()
				return m, nil
			}
			if !m.textarea.Focused() || m.streamActive {
				break
			}
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			m.recordPromptHistory(text)
			m.textarea.Reset()
			m.resetHistoryNavigation()
			m.exitHistorySearch()
			if strings.HasPrefix(text, "/") {
				return m.handleSlash(text)
			}
			m.lines = append(m.lines, tuiUserLabelStyle.Render("You>"), m.renderMessageBody(text), "")
			m.syncViewport()
			m.streamActive = true
			m.spinning = true
			m.streamBuf = ""
			userText := text
			return m, tea.Batch(m.spinner.Tick, m.sendAndStream(userText))
		}
	case streamDeltaMsg:
		chunk := string(msg)
		m.streamBuf += chunk
		partial := m.streamBuf
		if len(m.lines) > 0 && m.lines[len(m.lines)-1] != "" {
			m.lines[len(m.lines)-1] = partial
		} else {
			m.lines = append(m.lines, partial)
		}
		m.syncViewport()
		return m, nil
	case streamDoneMsg:
		m.streamActive = false
		m.spinning = false
		if msg.err != nil {
			if len(m.lines) > 0 && m.lines[len(m.lines)-1] != "" {
				m.lines = m.lines[:len(m.lines)-1]
			}
			m.lines = append(m.lines, tuiErrorStyle.Render("Error: "+msg.err.Error()), "")
			m.syncViewport()
			return m, nil
		}
		content := m.streamBuf
		m.streamBuf = ""
		if len(m.lines) > 0 && m.lines[len(m.lines)-1] != "" {
			m.lines = m.lines[:len(m.lines)-1]
		}
		rendered := m.renderMD(content)
		m.lines = append(m.lines, tuiAssistantLabelStyle.Render("Assistant>"), m.renderMessageBody(rendered), "")
		m.syncViewport()
		save := content
		return m, func() tea.Msg {
			if err := PostMessage(m.client, m.sessionID, "assistant", save); err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Warning: failed to save response: " + err.Error()))
			}
			return appendLineMsg("")
		}
	case appendLineMsg:
		if string(msg) != "" {
			m.lines = append(m.lines, string(msg), "")
			m.syncViewport()
		}
		return m, nil
	case openModelPickerMsg:
		m.picker = newModelPickerState(msg.models)
		return m, nil
	case modelChangedMsg:
		m.model = string(msg)
		m.picker = nil
		m.lines = append(m.lines, m.renderMD(fmt.Sprintf("Switched model to `%s`", m.model)), "")
		m.syncViewport()
		return m, nil
	case spinner.TickMsg:
		if m.spinning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	var cmds []tea.Cmd
	var vpCmd, taCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.textarea, taCmd = m.textarea.Update(msg)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.historySearchMode && !m.streamActive {
			switch keyMsg.Type {
			case tea.KeyRunes, tea.KeyBackspace, tea.KeyDelete, tea.KeySpace:
				m.historySearchQuery = strings.TrimSpace(m.textarea.Value())
				m.historySearchCursor = len(m.promptHistory)
				m.applyHistorySearchResult()
			}
		} else if !m.streamActive {
			m.resetHistoryNavigation()
		}
	}
	cmds = append(cmds, vpCmd, taCmd)
	return m, tea.Batch(cmds...)
}

func (m chatTUIModel) View() string {
	if !m.ready {
		return "\n  Initializing…"
	}
	div := tuiDivStyle.Render(strings.Repeat("─", tuiMax(0, m.width-2)))
	title := "OmniLLM Chat"
	if m.model != "" {
		title += "  │  " + m.model
	}
	var b strings.Builder
	b.WriteString(tuiTitleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(div)
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	if m.spinning {
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" thinking…")
	}
	b.WriteString("\n")
	b.WriteString(div)
	b.WriteString("\n")
	b.WriteString(m.textarea.View())
	if status := m.historySearchStatus(); status != "" {
		b.WriteString("\n")
		b.WriteString(status)
	}
	b.WriteString("\n")
	b.WriteString(tuiHelpStyle.Render("Ctrl+C/Esc: quit  ↑↓: history  PgUp/PgDn: scroll  Ctrl+R: search history  /help: commands"))
	base := b.String()
	if m.picker == nil {
		return base
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderPickerModal())
}

func (m chatTUIModel) renderPickerModal() string {
	if m.picker == nil {
		return ""
	}
	modalWidth := minInt(90, tuiMax(58, m.width-8))
	visibleItems := 20
	if len(m.picker.entries) < visibleItems {
		visibleItems = len(m.picker.entries)
	}
	if visibleItems < 1 {
		visibleItems = 1
	}
	start := m.picker.scrollOffset
	if start < 0 {
		start = 0
	}
	if start > len(m.picker.entries) {
		start = len(m.picker.entries)
	}
	end := minInt(len(m.picker.entries), start+visibleItems)

	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	groupStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	selectedGroupStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("13")).Padding(0, 1).Width(modalWidth - 6)
	selectedModelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Padding(0, 1).Width(modalWidth - 6)
	normalModelStyle := lipgloss.NewStyle().Padding(0, 1).Width(modalWidth - 6)

	var rows strings.Builder
	if len(m.picker.entries) == 0 {
		rows.WriteString(muted.Render("No models match your filter."))
	} else {
		for i := start; i < end; i++ {
			entry := m.picker.entries[i]
			if entry.isGroup {
				prefix := "▸"
				count := 0
				expanded := false
				for _, group := range m.picker.groups {
					if group.owner == entry.owner {
						count = len(group.models)
						expanded = group.expanded
						if expanded {
							prefix = "▾"
						}
						break
					}
				}
				line := fmt.Sprintf("%s %s", prefix, entry.owner)
				if strings.TrimSpace(m.picker.filter) == "" {
					line += muted.Render(fmt.Sprintf("  (%d)", count))
				}
				if i == m.picker.selectedIdx {
					rows.WriteString(selectedGroupStyle.Render(line))
				} else {
					rows.WriteString(groupStyle.Render(line))
				}
			} else {
				mdl := entry.model
				primary := "  " + mdl.Name
				if strings.TrimSpace(mdl.Name) == "" {
					primary = "  " + mdl.ID
				}
				secondaryText := mdl.Selector
				if mdl.ProviderName != "" && mdl.ProviderName != mdl.Selector {
					secondaryText += "  •  " + mdl.ProviderName
				}
				secondary := muted.Render("  " + secondaryText)
				rowText := primary + "\n" + secondary
				if i == m.picker.selectedIdx {
					rows.WriteString(selectedModelStyle.Render(rowText))
				} else {
					rows.WriteString(normalModelStyle.Render(rowText))
				}
			}
			if i < end-1 {
				rows.WriteString("\n")
			}
		}
	}

	filterValue := m.picker.filter
	if filterValue == "" {
		filterValue = muted.Render("Type to search")
	}
	filterBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(modalWidth - 6).Render(filterValue)
	header := lipgloss.NewStyle().Bold(true).Render("Select model")
	subtitle := muted.Render("Space expands • Enter selects • Esc closes • ↑↓ navigate")
	footer := muted.Render(fmt.Sprintf("Showing %d-%d of %d", start+1, end, len(m.picker.entries)))
	content := lipgloss.JoinVertical(lipgloss.Left, header, subtitle, "", lipgloss.NewStyle().Bold(true).Render("Search"), filterBox, "", rows.String(), "", footer)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(1, 2).Width(modalWidth).Render(content)
}

func (m *chatTUIModel) syncViewport() {
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
	m.viewport.GotoBottom()
}

func (m *chatTUIModel) recordPromptHistory(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if len(m.promptHistory) > 0 && m.promptHistory[len(m.promptHistory)-1] == text {
		return
	}
	m.promptHistory = append(m.promptHistory, text)
}

func (m *chatTUIModel) applyTextareaValue(text string) {
	m.textarea.SetValue(text)
	m.textarea.CursorEnd()
}

func (m *chatTUIModel) resetHistoryNavigation() {
	m.historyIndex = -1
	m.historyDraft = ""
}

func (m *chatTUIModel) exitHistorySearch() {
	m.historySearchMode = false
	m.historySearchQuery = ""
	m.historySearchCursor = -1
}

func (m *chatTUIModel) enterHistorySearch() {
	m.historySearchMode = true
	m.historySearchQuery = strings.TrimSpace(m.textarea.Value())
	m.historySearchCursor = len(m.promptHistory)
	m.applyHistorySearchResult()
}

func (m *chatTUIModel) applyHistorySearchResult() {
	if len(m.promptHistory) == 0 {
		return
	}
	query := strings.ToLower(strings.TrimSpace(m.historySearchQuery))
	if query == "" {
		if m.historySearchCursor < 0 || m.historySearchCursor >= len(m.promptHistory) {
			m.historySearchCursor = len(m.promptHistory) - 1
		}
		m.applyTextareaValue(m.promptHistory[m.historySearchCursor])
		return
	}
	for i := minInt(m.historySearchCursor-1, len(m.promptHistory)-1); i >= 0; i-- {
		if strings.Contains(strings.ToLower(m.promptHistory[i]), query) {
			m.historySearchCursor = i
			m.applyTextareaValue(m.promptHistory[i])
			return
		}
	}
}

func (m *chatTUIModel) cyclePromptHistory(direction int) {
	if len(m.promptHistory) == 0 {
		return
	}
	if m.historyIndex == -1 {
		m.historyDraft = m.textarea.Value()
		if direction < 0 {
			m.historyIndex = len(m.promptHistory) - 1
		} else {
			return
		}
	} else {
		m.historyIndex += direction
	}
	if m.historyIndex < 0 {
		m.historyIndex = -1
		m.applyTextareaValue(m.historyDraft)
		return
	}
	if m.historyIndex >= len(m.promptHistory) {
		m.historyIndex = -1
		m.applyTextareaValue(m.historyDraft)
		return
	}
	m.applyTextareaValue(m.promptHistory[m.historyIndex])
}

func (m *chatTUIModel) historySearchStatus() string {
	if !m.historySearchMode {
		return ""
	}
	query := m.historySearchQuery
	if query == "" {
		query = "*"
	}
	return tuiHelpStyle.Render("history search: " + query + "  Enter/Esc: close")
}

func (m chatTUIModel) renderMD(md string) string {
	if m.mdRenderer == nil || strings.TrimSpace(md) == "" {
		return md
	}
	out, err := m.mdRenderer.Render(md)
	if err != nil {
		return md
	}
	return trimCommonLeadingSpaces(strings.TrimRight(out, "\n"))
}

func (m chatTUIModel) renderMessageBody(body string) string {
	if body == "" {
		return body
	}
	return lipgloss.NewStyle().PaddingLeft(2).Render(body)
}

func trimCommonLeadingSpaces(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, line := range lines {
		visible := strings.TrimSpace(ansiPrefixPattern.ReplaceAllString(line, ""))
		if visible == "" {
			continue
		}
		indent := leadingVisibleSpaces(line)
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return s
	}
	for i, line := range lines {
		lines[i] = trimLeadingVisibleSpaces(line, minIndent)
	}
	return strings.Join(lines, "\n")
}

func leadingVisibleSpaces(line string) int {
	line = ansiPrefixPattern.ReplaceAllString(line, "")
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	return indent
}

func trimLeadingVisibleSpaces(line string, maxTrim int) string {
	prefix := ansiPrefixPattern.FindString(line)
	rest := line[len(prefix):]
	trimmed := 0
	for trimmed < len(rest) && trimmed < maxTrim && rest[trimmed] == ' ' {
		trimmed++
	}
	return prefix + rest[trimmed:]
}

func tuiMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m chatTUIModel) sendAndStream(userText string) tea.Cmd {
	return func() tea.Msg {
		if err := PostMessage(m.client, m.sessionID, "user", userText); err != nil {
			return streamDoneMsg{err: fmt.Errorf("store message: %w", err)}
		}
		_, messages, err := LoadSessionMessages(m.client, m.sessionID)
		if err != nil {
			return streamDoneMsg{err: fmt.Errorf("load messages: %w", err)}
		}
		go m.runStream(messages)
		return nil
	}
}

func (m chatTUIModel) runStream(messages []Message) {
	if m.prog == nil {
		return
	}
	reqModel := m.model
	if reqModel == "" {
		reqModel = "gpt-4"
	}
	body := map[string]interface{}{"model": reqModel, "messages": messages, "stream": true}
	resp, err := m.client.PostStream("/v1/chat/completions", body)
	if err != nil {
		m.prog.Send(streamDoneMsg{err: fmt.Errorf("request: %w", err)})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		m.prog.Send(streamDoneMsg{err: fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))})
		return
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	var buf bytes.Buffer
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			if buf.Len() == 0 {
				continue
			}
			trimmed := bytes.TrimSpace(buf.Bytes())
			buf.Reset()
			if len(trimmed) == 0 || string(trimmed) == "[DONE]" {
				continue
			}
			var chunk map[string]interface{}
			if err := json.Unmarshal(trimmed, &chunk); err != nil {
				continue
			}
			choices, _ := chunk["choices"].([]interface{})
			if len(choices) == 0 {
				continue
			}
			choice, _ := choices[0].(map[string]interface{})
			delta, _ := choice["delta"].(map[string]interface{})
			content, _ := delta["content"].(string)
			if content != "" {
				m.prog.Send(streamDeltaMsg(content))
			}
			continue
		}
		if bytes.HasPrefix(raw, []byte("data: ")) {
			buf.Write(bytes.TrimPrefix(raw, []byte("data: ")))
		} else if bytes.HasPrefix(raw, []byte("data:")) {
			buf.Write(bytes.TrimPrefix(raw, []byte("data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		m.prog.Send(streamDoneMsg{err: fmt.Errorf("stream: %w", err)})
		return
	}
	m.prog.Send(streamDoneMsg{})
}

func (m chatTUIModel) handleSlash(text string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return m, nil
	}
	add := func(line string) {
		if line != "" {
			m.lines = append(m.lines, line, "")
			m.syncViewport()
		}
	}
	switch fields[0] {
	case "/quit", "/exit":
		return m, tea.Quit
	case "/help":
		add(m.renderMD("**Commands:**\n\n- `/help` — show this help\n- `/model` — show current model\n- `/model <id>` — switch model\n- `/models` — open model picker\n- `/session` — show session info\n- `/quit` or `/exit` — quit\n"))
		return m, nil
	case "/session":
		add(m.renderMD(fmt.Sprintf("**Session:** `%s`\n**Model:** `%s`", m.sessionID, m.model)))
		return m, nil
	case "/model":
		if len(fields) == 1 {
			add(m.renderMD(fmt.Sprintf("Current model: `%s`", m.model)))
			return m, nil
		}
		newModel := fields[1]
		return m, func() tea.Msg {
			if err := UpdateSessionModel(m.client, m.sessionID, newModel); err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Error: " + err.Error()))
			}
			return modelChangedMsg(newModel)
		}
	case "/models":
		return m, func() tea.Msg {
			models, err := ListModels(m.client)
			if err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Error: " + err.Error()))
			}
			if len(models) == 0 {
				return appendLineMsg("No models available.")
			}
			return openModelPickerMsg{models: models}
		}
	default:
		add(tuiErrorStyle.Render(fmt.Sprintf("Unknown command: %s — use /help", fields[0])))
		return m, nil
	}
}

func RunTUI(c Client, sessionID, model string, history []Message) error {
	m := newChatTUIModel(c, sessionID, model, history)
	p := tea.NewProgram(m, tea.WithAltScreen())
	go func() { p.Send(progReadyMsg{p: p}) }()
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}
