package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"

	agentpkg "omnillm/internal/agent"
	toolspkg "omnillm/internal/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	tuiTitleStyle           = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")).Padding(0, 1)
	tuiUserLabelStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#06B6D4"))
	tuiAssistantLabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A855F7"))
	tuiErrorStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	tuiHelpStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Padding(0, 1)
	tuiDivStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))
	tuiUserBlockStyle       = lipgloss.NewStyle().Background(lipgloss.Color("#171717")).Foreground(lipgloss.Color("#F9FAFB")).Padding(0, 1)
	tuiAssistantBlockStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Padding(0, 1)
	tuiThinkingStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Italic(true).PaddingLeft(1)
	tuiSidebarStyle         = lipgloss.NewStyle().Background(lipgloss.Color("#111111")).Foreground(lipgloss.Color("#E5E7EB")).Padding(1, 2)
	tuiSidebarHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB"))
	tuiSidebarLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	tuiSidebarValueStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	tuiPermissionLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	tuiPermissionBlockStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#F59E0B")).Padding(0, 1)
	ansiPrefixPattern       = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)*`)
)

const (
	tuiSidebarWidth    = 30
	tuiMinSidebarWidth = 96
)

type transcriptEntryType string

const (
	transcriptUser       transcriptEntryType = "user"
	transcriptAssistant  transcriptEntryType = "assistant"
	transcriptInfo       transcriptEntryType = "info"
	transcriptError      transcriptEntryType = "error"
	transcriptPermission transcriptEntryType = "permission"
)

type transcriptEntry struct {
	kind    transcriptEntryType
	content string
}

type progReadyMsg struct{ p *tea.Program }
type streamDeltaMsg string
type streamDoneMsg struct{ err error }
type appendLineMsg string
type modelChangedMsg string
type agentBackendChangedMsg string
type openModelPickerMsg struct{ models []ModelInfo }
type agentDoneMsg struct {
	content string
	err     error
}

type agentProgressMsg struct {
	text string
}

type pendingPermissionState struct {
	req    toolspkg.PermissionRequest
	respCh chan bool
}

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
	client       Client
	sessionID    string
	model        string
	mode         string
	agentBackend string
	prog         *tea.Program

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	spinning bool

	entries            []transcriptEntry
	streamActive       bool
	streamBuf          string
	picker             *modelPickerState
	pendingPermission  *pendingPermissionState
	agentTurnCancel    context.CancelFunc
	normalPlaceholder  string
	approvalPromptText string
	autopilot          bool

	promptHistory       []string
	historyIndex        int
	historyDraft        string
	historySearchMode   bool
	historySearchQuery  string
	historySearchCursor int

	width        int
	height       int
	mainWidth    int
	sidebarWidth int
	ready        bool
	mdRenderer   *glamour.TermRenderer
}

func newChatTUIModel(c Client, sessionID, model, mode, agentBackend string, history []Message) chatTUIModel {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A855F7"))
	sp.Spinner = spinner.Dot

	ta := textarea.New()
	ta.Placeholder = "Type a message… (Enter to send, Shift+Enter for newline)"
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true)

	mdR, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(80))

	m := chatTUIModel{
		client:              c,
		sessionID:           sessionID,
		model:               model,
		mode:                mode,
		agentBackend:        agentBackend,
		spinner:             sp,
		textarea:            ta,
		mdRenderer:          mdR,
		historyIndex:        -1,
		historySearchCursor: -1,
		normalPlaceholder:   ta.Placeholder,
		approvalPromptText:  "y: approve  n: deny  Enter: confirm",
	}
	for _, msg := range history {
		switch msg.Role {
		case "user":
			m.appendEntry(transcriptUser, msg.Content)
			m.recordPromptHistory(msg.Content)
		case "assistant":
			m.appendEntry(transcriptAssistant, msg.Content)
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
		m.sidebarWidth = 0
		m.mainWidth = msg.Width - 2
		if msg.Width >= tuiMinSidebarWidth {
			m.sidebarWidth = tuiSidebarWidth
			m.mainWidth = msg.Width - m.sidebarWidth - 3
		}
		if m.mainWidth < 24 {
			m.mainWidth = 24
		}
		vpH := max(msg.Height-9, 3)
		if !m.ready {
			m.viewport = viewport.New(m.mainWidth, vpH)
			m.ready = true
		} else {
			m.viewport.Width = m.mainWidth
			m.viewport.Height = vpH
		}
		m.textarea.SetWidth(tuiMax(20, m.mainWidth-2))
		if m.mdRenderer != nil {
			m.mdRenderer, _ = glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(tuiMax(20, m.mainWidth-6)))
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
				if m.pendingPermission != nil {
					select {
					case m.pendingPermission.respCh <- false:
					default:
					}
					m.pendingPermission = nil
				}
				if m.agentTurnCancel != nil {
					m.agentTurnCancel()
					m.agentTurnCancel = nil
				}
				m.textarea.Placeholder = m.normalPlaceholder
				m.syncViewport()
				return m, nil
			}
			if m.agentTurnCancel != nil {
				m.agentTurnCancel()
				m.agentTurnCancel = nil
			}
			return m, tea.Quit
		case tea.KeyEscape:
			if m.historySearchMode {
				m.exitHistorySearch()
				return m, nil
			}
			if m.agentTurnCancel != nil {
				m.agentTurnCancel()
				m.agentTurnCancel = nil
			}
			return m, nil
		case tea.KeyCtrlR:
			if !m.textarea.Focused() || m.streamActive {
				break
			}
			m.enterHistorySearch()
			return m, nil
		case tea.KeyShiftTab:
			m.autopilot = !m.autopilot
			m.syncViewport()
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
			if !m.textarea.Focused() || m.streamActive && m.pendingPermission == nil {
				break
			}
			text := strings.TrimSpace(m.textarea.Value())
			if m.pendingPermission != nil {
				return m.handleApprovalInput(text)
			}
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
			m.appendEntry(transcriptUser, text)
			m.syncViewport()
			m.streamActive = true
			m.spinning = true
			m.streamBuf = ""
			userText := text
			return m, tea.Batch(m.spinner.Tick, m.sendAndStream(userText))
		}
	case permissionRequestMsg:
		m.pendingPermission = &pendingPermissionState{req: msg.req, respCh: msg.respCh}
		m.appendEntry(transcriptPermission, agentpkg.EncodePermissionPrompt(msg.req.ToolName, msg.req.Arguments))
		m.textarea.Reset()
		m.textarea.Placeholder = "y/n (approve tool execution)"
		m.syncViewport()
		return m, nil
	case streamDeltaMsg:
		m.streamBuf += string(msg)
		m.syncViewport()
		return m, nil
	case streamDoneMsg:
		m.streamActive = false
		m.spinning = false
		m.pendingPermission = nil
		m.textarea.Placeholder = m.normalPlaceholder
		if msg.err != nil {
			m.appendEntry(transcriptError, "Error: "+msg.err.Error())
			m.streamBuf = ""
			m.syncViewport()
			return m, nil
		}
		content := m.streamBuf
		m.streamBuf = ""
		if strings.TrimSpace(content) != "" {
			m.appendEntry(transcriptAssistant, content)
		}
		m.syncViewport()
		save := content
		return m, func() tea.Msg {
			if err := PostMessage(m.client, m.sessionID, "assistant", save); err != nil {
				return appendLineMsg("Warning: failed to save response: " + err.Error())
			}
			return appendLineMsg("")
		}
	case agentDoneMsg:
		m.streamActive = false
		m.spinning = false
		m.pendingPermission = nil
		m.agentTurnCancel = nil
		m.textarea.Placeholder = m.normalPlaceholder
		if msg.err != nil {
			m.appendEntry(transcriptError, "Error: "+msg.err.Error())
			m.syncViewport()
			return m, nil
		}
		m.appendEntry(transcriptAssistant, msg.content)
		m.syncViewport()
		return m, nil
	case agentProgressMsg:
		m.appendEntry(transcriptInfo, msg.text)
		m.syncViewport()
		return m, nil
	case appendLineMsg:
		if string(msg) != "" {
			m.appendEntry(transcriptInfo, string(msg))
			m.syncViewport()
		}
		return m, nil
	case openModelPickerMsg:
		m.picker = newModelPickerState(msg.models)
		return m, nil
	case modelChangedMsg:
		m.model = string(msg)
		m.picker = nil
		m.appendEntry(transcriptInfo, fmt.Sprintf("Switched model to `%s`", m.model))
		m.syncViewport()
		return m, nil
	case agentBackendChangedMsg:
		m.agentBackend = string(msg)
		m.appendEntry(transcriptInfo, fmt.Sprintf("Switched agent backend to `%s`", m.agentBackend))
		m.syncViewport()
		return m, nil
	case spinner.TickMsg:
		if m.spinning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.syncViewport()
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

	title := "OmniLLM Chat"
	if m.mode != "" {
		title += "  │  " + m.mode
	}
	if m.model != "" {
		title += "  │  " + m.model
	}

	div := tuiDivStyle.Render(strings.Repeat("─", tuiMax(0, m.mainWidth)))
	var main strings.Builder
	main.WriteString(tuiTitleStyle.Width(m.mainWidth).Render(title))
	main.WriteString("\n")
	main.WriteString(div)
	main.WriteString("\n")
	main.WriteString(m.viewport.View())
	main.WriteString("\n")
	main.WriteString(div)
	main.WriteString("\n")
	main.WriteString(m.textarea.View())
	if m.pendingPermission != nil {
		main.WriteString("\n")
		main.WriteString(tuiHelpStyle.Render(m.approvalPromptText))
	} else if status := m.historySearchStatus(); status != "" {
		main.WriteString("\n")
		main.WriteString(status)
	}
	main.WriteString("\n")
	main.WriteString(tuiHelpStyle.Render("Ctrl+C: quit  ↑↓: history  PgUp/PgDn: scroll  Ctrl+R: search history  Shift+Tab: autopilot  /help: commands  /mode: switch mode"))

	base := main.String()
	if m.sidebarWidth > 0 {
		base = lipgloss.JoinHorizontal(lipgloss.Top, base, m.renderSidebar())
	}
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
	visibleItems := max(1, min(20, len(m.picker.entries)))
	start := min(max(0, m.picker.scrollOffset), len(m.picker.entries))
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
	m.viewport.SetContent(m.renderTranscript())
	m.viewport.GotoBottom()
}

func (m *chatTUIModel) appendEntry(kind transcriptEntryType, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	m.entries = append(m.entries, transcriptEntry{kind: kind, content: content})
}

func (m chatTUIModel) renderTranscript() string {
	blocks := make([]string, 0, len(m.entries)+1)
	for _, entry := range m.entries {
		switch entry.kind {
		case transcriptUser:
			blocks = append(blocks, m.renderUserSection(entry.content))
		case transcriptAssistant:
			blocks = append(blocks, m.renderAssistantSection(entry.content))
		case transcriptError:
			blocks = append(blocks, tuiErrorStyle.Render(entry.content))
		case transcriptPermission:
			blocks = append(blocks, m.renderPermissionSection(entry.content))
		default:
			blocks = append(blocks, m.renderInfoSection(entry.content))
		}
	}
	if m.streamActive || strings.TrimSpace(m.streamBuf) != "" {
		blocks = append(blocks, m.renderThinkingSection(m.streamBuf))
	}
	return strings.Join(blocks, "\n\n")
}

func (m chatTUIModel) renderUserSection(text string) string {
	label := tuiUserLabelStyle.Render("You")
	block := tuiUserBlockStyle.Width(tuiMax(8, m.mainWidth-2)).Render(text)
	return lipgloss.JoinVertical(lipgloss.Left, label, block)
}

func (m chatTUIModel) renderAssistantSection(text string) string {
	rendered := m.renderMD(text)
	label := tuiAssistantLabelStyle.Render("Assistant")
	body := tuiAssistantBlockStyle.Width(tuiMax(8, m.mainWidth-2)).Render(rendered)
	return lipgloss.JoinVertical(lipgloss.Left, label, body)
}

func (m chatTUIModel) renderThinkingSection(text string) string {
	body := strings.TrimSpace(text)
	if body == "" {
		body = m.spinner.View() + " thinking…"
	} else if m.spinning {
		body = m.spinner.View() + " " + body
	}
	return tuiThinkingStyle.Width(tuiMax(8, m.mainWidth-1)).Render(body)
}

func (m chatTUIModel) renderInfoSection(text string) string {
	return tuiHelpStyle.Width(tuiMax(8, m.mainWidth-1)).Render(text)
}

func (m chatTUIModel) renderPermissionSection(text string) string {
	label := tuiPermissionLabelStyle.Render("Permission required")
	body := tuiPermissionBlockStyle.Width(tuiMax(8, m.mainWidth-2)).Render(text)
	return lipgloss.JoinVertical(lipgloss.Left, label, body)
}

func (m chatTUIModel) renderSidebar() string {
	status := "Idle"
	statusColor := lipgloss.Color("#6B7280")
	if m.streamActive || m.spinning {
		status = "Active"
		statusColor = lipgloss.Color("#10B981")
	}
	statusDot := lipgloss.NewStyle().Foreground(statusColor).Render("●")
	valueWidth := tuiMax(8, m.sidebarWidth-11)
	sections := []string{
		tuiSidebarHeaderStyle.Render("Disk usage check"),
		"",
		tuiSidebarHeaderStyle.Render("Context"),
		tuiSidebarLabelStyle.Render("Session") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(truncateString(m.sessionID, valueWidth)),
		tuiSidebarLabelStyle.Render("Model") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(truncateString(m.model, valueWidth)),
		tuiSidebarLabelStyle.Render("Mode") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(truncateString(m.mode, valueWidth)),
	}
	if m.agentBackend != "" {
		sections = append(sections, tuiSidebarLabelStyle.Render("Agent")+"\n"+tuiSidebarValueStyle.Width(valueWidth).Render(truncateString(m.agentBackend, valueWidth)))
	}
	sections = append(sections,
		tuiSidebarLabelStyle.Render("Status")+"\n"+statusDot+" "+status,
		tuiSidebarLabelStyle.Render("Messages")+"\n"+tuiSidebarValueStyle.Render(fmt.Sprintf("%d total", len(m.entries))),
	)
	if m.autopilot {
		sections = append(sections, tuiSidebarHeaderStyle.Render("AUTOPILOT")+"\n"+tuiSidebarValueStyle.Render("Tools auto-approved"))
	}
	sections = append(sections,
		tuiSidebarHeaderStyle.Render("LSP"),
		tuiSidebarLabelStyle.Render("LSPs will activate as files are read"),
	)
	return lipgloss.NewStyle().Width(m.sidebarWidth).Render(tuiSidebarStyle.Width(m.sidebarWidth).Height(tuiMax(8, m.height-1)).Render(strings.Join(sections, "\n\n")))
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 3 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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

func (m *chatTUIModel) sendAndStream(userText string) tea.Cmd {
	if m.mode == "agent" {
		ctx, cancel := context.WithCancel(context.Background())
		m.agentTurnCancel = cancel
		checker := m.makeTUIPermissionChecker()
		prog := m.prog
		client := m.client
		sessionID := m.sessionID
		model := m.model
		backend := m.agentBackend
		return func() tea.Msg {
			defer cancel()
			eventCh, err := StreamAgentTurnWithChecker(ctx, client, sessionID, model, backend, userText, checker)
			if err != nil {
				return agentDoneMsg{err: err}
			}
			var finalContent string
			for event := range eventCh {
				switch event.Type {
				case agentpkg.EventToken:
					finalContent += event.Content
				case agentpkg.EventToolCall:
					if prog != nil {
						prog.Send(agentProgressMsg{text: fmt.Sprintf("🔧 Calling tool `%s`…", event.Tool)})
					}
				case agentpkg.EventToolResult:
					if prog != nil {
						prog.Send(agentProgressMsg{text: fmt.Sprintf("✅ Tool `%s` returned: %s", event.Tool, truncateString(event.Content, 120))})
					}
				case agentpkg.EventError:
					return agentDoneMsg{content: finalContent, err: fmt.Errorf("%s", event.Content)}
				case agentpkg.EventDone:
					// loop will end when channel closes
				}
			}
			if finalContent != "" {
				if err := PostMessage(client, sessionID, "assistant", finalContent); err != nil {
					return agentDoneMsg{content: finalContent, err: fmt.Errorf("store assistant message: %w", err)}
				}
			}
			return agentDoneMsg{content: finalContent}
		}
	}
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

func (m *chatTUIModel) makeTUIPermissionChecker() toolspkg.PermissionChecker {
	var mu sync.Mutex
	return func(ctx context.Context, req toolspkg.PermissionRequest) (bool, error) {
		mu.Lock()
		defer mu.Unlock()

		if m.autopilot {
			return true, nil
		}

		if m.prog == nil {
			return false, fmt.Errorf("tui program not ready")
		}

		respCh := make(chan bool, 1)
		m.prog.Send(permissionRequestMsg{req: req, respCh: respCh})

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case approved := <-respCh:
			return approved, nil
		}
	}
}

func (m *chatTUIModel) handleApprovalInput(text string) (tea.Model, tea.Cmd) {
	approved := strings.EqualFold(text, "y") || strings.EqualFold(text, "yes")
	decision := "Denied"
	if approved {
		decision = "Approved"
	}
	m.appendEntry(transcriptInfo, fmt.Sprintf("%s `%s`", decision, m.pendingPermission.req.ToolName))
	select {
	case m.pendingPermission.respCh <- approved:
	default:
	}
	m.pendingPermission = nil
	m.textarea.Reset()
	m.textarea.Placeholder = m.normalPlaceholder
	m.syncViewport()
	return m, nil
}

func (m chatTUIModel) runStream(messages []Message) {
	if m.prog == nil {
		return
	}
	reqModel := m.model
	if reqModel == "" {
		reqModel = "gpt-4"
	}
	body := map[string]any{"model": reqModel, "messages": messages, "stream": true}
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
			var chunk map[string]any
			if err := json.Unmarshal(trimmed, &chunk); err != nil {
				continue
			}
			choices, _ := chunk["choices"].([]any)
			if len(choices) == 0 {
				continue
			}
			choice, _ := choices[0].(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			content, _ := delta["content"].(string)
			if content != "" {
				m.prog.Send(streamDeltaMsg(content))
			}
			continue
		}
		if after, ok := bytes.CutPrefix(raw, []byte("data: ")); ok {
			buf.Write(after)
		} else if after, ok := bytes.CutPrefix(raw, []byte("data:")); ok {
			buf.Write(after)
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
			m.appendEntry(transcriptInfo, line)
			m.syncViewport()
		}
	}
	switch fields[0] {
	case "/quit", "/exit":
		return m, tea.Quit
	case "/clear", "/cls":
		m.entries = nil
		m.syncViewport()
		return m, nil
	case "/help":
		add(m.renderMD("**Commands:**\n\n- `/help` — show this help\n- `/mode` — show current mode\n- `/mode <chat|agent>` — switch mode\n- `/model` — show current model\n- `/model <id>` — switch model\n- `/agent` — show current backend and supported backends\n- `/agent <backend>` — switch agent backend\n- `/models` — open model picker\n- `/session` — show session info\n- `/clear` or `/cls` — clear the screen\n- `/quit` or `/exit` — quit\n"))
		return m, nil
	case "/session":
		add(m.renderMD(fmt.Sprintf("**Session:** `%s`\n**Mode:** `%s`\n**Model:** `%s`", m.sessionID, m.mode, m.model)))
		return m, nil
	case "/mode":
		result := agentpkg.ParseCommand(text, m.mode)
		if result.NewMode != nil {
			m.mode = *result.NewMode
		}
		add(m.renderMD(result.Response))
		return m, nil
	case "/agent":
		if len(fields) == 1 {
			currentBackend, err := CurrentAgentBackend(m.client, m.sessionID, "")
			if err != nil {
				add(tuiErrorStyle.Render("Error: " + err.Error()))
				return m, nil
			}
			if currentBackend == "" {
				currentBackend = supportedAgentBackends()[0]
			}
			add(m.renderMD(fmt.Sprintf("Current agent backend: `%s`\n\nSupported backends: `%s`", currentBackend, supportedAgentBackendsText())))
			return m, nil
		}
		newBackend := fields[1]
		if !isSupportedAgentBackend(newBackend) {
			add(tuiErrorStyle.Render(fmt.Sprintf("Error: unknown agent backend %q — supported backends: %s", newBackend, supportedAgentBackendsText())))
			return m, nil
		}
		return m, func() tea.Msg {
			if err := UpdateSessionAgentBackend(m.client, m.sessionID, newBackend); err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Error: " + err.Error()))
			}
			return agentBackendChangedMsg(newBackend)
		}
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

func RunTUI(c Client, sessionID, model, mode, agentBackend string, history []Message) error {
	m := newChatTUIModel(c, sessionID, model, mode, agentBackend, history)
	p := tea.NewProgram(m, tea.WithAltScreen())
	go func() { p.Send(progReadyMsg{p: p}) }()
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}
