package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	agentpkg "omnillm/internal/agent"
	toolspkg "omnillm/internal/tools"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	liptable "github.com/charmbracelet/lipgloss/table"
	xansi "github.com/charmbracelet/x/ansi"
)

var (
	tuiTitleStyle             = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")).Padding(0, 1)
	tuiUserLabelStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67E8F9"))
	tuiAssistantLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D8B4FE"))
	tuiThinkingLabelStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	tuiErrorStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	tuiHelpStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Padding(0, 1)
	tuiStatusStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Padding(0, 1)
	tuiStatusAccentStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#C4B5FD"))
	tuiInputMetaStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Padding(0, 1)
	tuiPermManualChipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDE68A")).Background(lipgloss.Color("#3A2A10")).Padding(0, 1)
	tuiPermAutoChipStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1FAE5")).Background(lipgloss.Color("#123225")).Padding(0, 1)
	tuiPermPendingChipStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFE7B3")).Background(lipgloss.Color("#4A3415")).Padding(0, 1)
	tuiDivStyle               = lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))
	tuiUserBlockStyle         = lipgloss.NewStyle().Background(lipgloss.Color("#10161B")).Foreground(lipgloss.Color("#E6F7FB")).Padding(0, 1)
	tuiAssistantBlockStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#17141D")).Foreground(lipgloss.Color("#F0EAF7")).Padding(0, 1)
	tuiThinkingBlockStyle     = lipgloss.NewStyle().Background(lipgloss.Color("#141723")).Foreground(lipgloss.Color("#D6CCFF")).Padding(0, 1)
	tuiUserHoverStyle         = lipgloss.NewStyle().Background(lipgloss.Color("#17232A")).Foreground(lipgloss.Color("#ECFBFF")).Padding(0, 1)
	tuiAssistantHoverStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#211A2B")).Foreground(lipgloss.Color("#F7F1FF")).Padding(0, 1)
	tuiThinkingHoverStyle     = lipgloss.NewStyle().Background(lipgloss.Color("#1A1E2E")).Foreground(lipgloss.Color("#E6DEFF")).Padding(0, 1)
	tuiToolResultLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9AD5A0"))
	tuiToolResultNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8FA3B8"))
	tuiToolResultLineStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#151917")).Foreground(lipgloss.Color("#9CAAA0"))
	tuiToolResultFocusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CAAA0")).PaddingLeft(1).BorderStyle(lipgloss.Border{Left: "▌"}).BorderLeft(true).BorderForeground(lipgloss.Color("#4E8A57"))
	tuiToolResultBlurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CAAA0")).PaddingLeft(2)
	tuiThinkingStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Italic(true)
	tuiSidebarStyle           = lipgloss.NewStyle().Background(lipgloss.Color("#101014")).Foreground(lipgloss.Color("#E5E7EB")).Padding(1, 2)
	tuiSidebarHeaderStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB"))
	tuiSidebarLabelStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	tuiSidebarValueStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	tuiPermissionLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	tuiPermissionBlockStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#2A1F0F")).Foreground(lipgloss.Color("#FDE68A")).Padding(0, 1)
	tuiSelectionStyle         = lipgloss.NewStyle().Background(lipgloss.Color("#E8E8E8")).Foreground(lipgloss.Color("#111111"))
	tuiInputShellStyle        = lipgloss.NewStyle().Background(lipgloss.Color("#1C1C1C")).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#3B82F6"))
	tuiInputAccentStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))
	tuiTableBorderStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	tuiTableHeaderStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#C084FC")).Bold(true).Padding(0, 1)
	tuiTableCellStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Padding(0, 1)
	tuiTableAccentStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
	ansiPrefixPattern         = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)*`)
	omnicodeLogoLeft          = []string{
		"                    ",
		"█▀▀█ █▄  ▄█ █▄  █  █ ",
		"█  █ █ ▀▀ █ █ ▀▄█  █ ",
		"▀▀▀▀ ▀    ▀ ▀  ▀▀  ▀ ",
	}
	omnicodeLogoRight = []string{
		"                   ",
		"█▀▀▀ █▀▀█ █▀▀▄ █▀▀▀",
		"█    █  █ █  █ █▀▀▀",
		"▀▀▀▀ ▀▀▀▀ ▀▀▀  ▀▀▀▀",
	}
	omnicodeLogoStyle = logoStyle{
		left:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")),
		right:         tuiSidebarHeaderStyle,
		leftShadow:    lipgloss.NewStyle().Foreground(lipgloss.Color("#24242A")),
		rightShadow:   lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A42")),
		leftShadowBg:  lipgloss.NewStyle().Background(lipgloss.Color("#24242A")),
		rightShadowBg: lipgloss.NewStyle().Background(lipgloss.Color("#3A3A42")),
	}
	omnicodeLogo = renderOmnicodeLogo()
)

// ConfigSaveCallback is invoked when the TUI state changes so the hosting binary
// (e.g. omnicode) can persist user preferences.
var ConfigSaveCallback func(model, mode, apiShape, agentBackend, specMode string, autopilot bool, maxTurns int)

// SetConfigSaveCallback sets the callback for persisting TUI preferences.
func SetConfigSaveCallback(cb func(model, mode, apiShape, agentBackend, specMode string, autopilot bool, maxTurns int)) {
	ConfigSaveCallback = cb
}

// InitialConfig holds values restored from persisted config that the TUI
// should apply on startup.
var InitialConfig struct {
	Mode      string
	APIShape  string
	Autopilot bool
	MaxTurns  int
	SpecMode  string
}

const (
	tuiSidebarWidth    = 30
	tuiMinSidebarWidth = 60
	// Align tool-result click hit-testing with perceived terminal row placement.
	tuiToolResultHitRowOffset = 4
	slashPickerVisible        = 20
	tuiViewportMinHeight      = 1
	tuiBaseRows               = 6
	tuiSlashPickerFrameRows   = 3
)

type logoStyle struct {
	left          lipgloss.Style
	right         lipgloss.Style
	leftShadow    lipgloss.Style
	rightShadow   lipgloss.Style
	leftShadowBg  lipgloss.Style
	rightShadowBg lipgloss.Style
}

type transcriptEntryType string

const (
	transcriptUser       transcriptEntryType = "user"
	transcriptAssistant  transcriptEntryType = "assistant"
	transcriptInfo       transcriptEntryType = "info"
	transcriptError      transcriptEntryType = "error"
	transcriptPermission transcriptEntryType = "permission"
	transcriptToolResult transcriptEntryType = "tool_result"
)

// toolResultMaxLines is the number of output lines shown before collapsing.
const toolResultMaxLines = 10

type transcriptEntry struct {
	id       int64
	kind     transcriptEntryType
	content  string
	toolName string // only set for transcriptToolResult entries
}

type transcriptLayoutEntry struct {
	kind               transcriptEntryType
	startLine          int
	endLine            int
	clickableStartLine int
	clickableEndLine   int
}

type clipboardWriter interface {
	ReadAll() (string, error)
	WriteAll(text string) error
}

type systemClipboard struct{}

func (systemClipboard) ReadAll() (string, error) {
	return clipboard.ReadAll()
}

func (systemClipboard) WriteAll(text string) error {
	return clipboard.WriteAll(text)
}

type transcriptSelection struct {
	active    bool
	selecting bool
	startX    int
	startY    int
	endX      int
	endY      int
	pressLine int
	pressIdx  int
	pressID   int64
}

type progReadyMsg struct{ p *tea.Program }
type streamDeltaMsg string
type streamDoneMsg struct{ err error }
type submitInputMsg uint64
type appendLineMsg string
type modelChangedMsg string
type agentBackendChangedMsg string
type apiShapeChangedMsg string
type openModelPickerMsg struct{ models []ModelInfo }
type openSessionPickerMsg struct{ sessions []SessionSummary }
type sessionPickerState struct {
	sessions     []SessionSummary
	selectedIdx  int
	scrollOffset int
}
type sessionSelectedMsg struct{ session SessionSummary }
type sessionCreatedMsg struct {
	sessionID string
	apiShape  string
}
type sessionLoadedMsg struct {
	sessionID    string
	model        string
	apiShape     string
	agentBackend string
	messages     []Message
}
type agentDoneMsg struct {
	content string
	err     error
}

type agentProgressMsg struct {
	text string
}

type agentToolResultMsg struct {
	tool    string
	content string
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
	apiShape     string
	specMode     string
	prog         *tea.Program

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	spinning bool

	entries              []transcriptEntry
	nextEntryID          int64
	expandedEntries      map[int64]bool
	transcriptLayout     []transcriptLayoutEntry
	hoveredEntry         int
	selectionMessage     string
	streamActive         bool
	streamBuf            string
	queuedPrompt         string
	picker               *modelPickerState
	slashPicker          *slashPickerState
	sessionPicker        *sessionPickerState
	pendingPermission    *pendingPermissionState
	agentTurnCancel      context.CancelFunc
	normalPlaceholder    string
	approvalPromptText   string
	autopilot            bool
	autoFollow           bool
	maxTurns             int
	clipboard            clipboardWriter
	selection            transcriptSelection
	submitSeq            uint64
	pendingSubmitNewline bool

	promptHistory       []string
	historyIndex        int
	historyDraft        string
	historySearchMode   bool
	historySearchQuery  string
	historySearchCursor int

	onConfigSave func(model, mode, apiShape, agentBackend, specMode string, autopilot bool, maxTurns int)

	textareaExpanded bool
	ctrlCPrimed      bool

	width                 int
	height                int
	mainWidth             int
	sidebarWidth          int
	ready                 bool
	mdRenderer            *glamour.TermRenderer
	middleDragging        bool
	middleDragStartY      int
	middleDragStartOffset int
}

func newChatTUIModel(c Client, sessionID, model, mode, apiShape, agentBackend string, history []Message, onConfigSave func(model, mode, apiShape, agentBackend, specMode string, autopilot bool, maxTurns int)) chatTUIModel {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A855F7"))
	sp.Spinner = spinner.Dot

	ta := textarea.New()
	ta.Placeholder = "Type a message… (Enter to send, Ctrl+J for newline)"
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true)

	mdR, _ := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(78), glamour.WithTableWrap(false))

	m := chatTUIModel{
		client:              c,
		sessionID:           sessionID,
		model:               model,
		mode:                defaultMode(mode, InitialConfig.Mode),
		apiShape:            defaultAPIShape(apiShape, InitialConfig.APIShape),
		agentBackend:        agentBackend,
		specMode:            InitialConfig.SpecMode,
		spinner:             sp,
		textarea:            ta,
		mdRenderer:          mdR,
		historyIndex:        -1,
		historySearchCursor: -1,
		normalPlaceholder:   ta.Placeholder,
		approvalPromptText:  "y: approve  n: deny  Enter: confirm",
		onConfigSave:        onConfigSave,
		autopilot:           InitialConfig.Autopilot,
		autoFollow:          true,
		maxTurns:            max(InitialConfig.MaxTurns, 1),
		clipboard:           systemClipboard{},
		hoveredEntry:        -1,
		expandedEntries:     make(map[int64]bool),
		selectionMessage:    "",
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

func deferredSubmitInput(seq uint64) tea.Cmd {
	return tea.Tick(20*time.Millisecond, func(time.Time) tea.Msg {
		return submitInputMsg(seq)
	})
}

func (m *chatTUIModel) absorbPendingSubmitNewline() {
	if !m.pendingSubmitNewline {
		return
	}
	m.pendingSubmitNewline = false
	m.textarea.InsertString("\n")
	m.textareaExpanded = true
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	m.textarea.SetHeight(tuiMax(3, lines))
	m.textarea.CursorEnd()
}

func (m chatTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progReadyMsg:
		m.prog = msg.p
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Always show the sidebar when the terminal is wide enough.
		// Below the minimum width we collapse it to avoid squashing the main area.
		if msg.Width >= tuiMinSidebarWidth {
			m.sidebarWidth = tuiSidebarWidth
		} else {
			m.sidebarWidth = 0
		}
		if m.sidebarWidth > 0 {
			m.mainWidth = msg.Width - m.sidebarWidth - 3
		} else {
			m.mainWidth = msg.Width - 2
		}
		if m.mainWidth < 24 {
			m.mainWidth = 24
		}
		// Layout: title(1) + div(1) + viewport + div(1) + textarea(≥1) + status(1) + help(2) = 7 fixed rows
		// Reserve an extra row for the conditional permission/search status line.
		vpH := m.viewportHeight(msg.Height)
		if !m.ready {
			m.viewport = viewport.New(m.mainWidth, vpH)
			// Disable single-letter viewport keybindings (b, u, d, f, j, k, h, l)
			// that conflict with text input in the textarea.
			m.viewport.KeyMap.PageUp.SetKeys("pgup")
			m.viewport.KeyMap.HalfPageUp.SetKeys("ctrl+u")
			m.viewport.KeyMap.HalfPageDown.SetKeys("ctrl+d")
			m.viewport.KeyMap.Down.SetKeys("down")
			m.viewport.KeyMap.Up.SetKeys("up")
			m.viewport.KeyMap.Left.SetKeys("left")
			m.viewport.KeyMap.Right.SetKeys("right")
			m.ready = true
		} else {
			m.viewport.Width = m.mainWidth
			m.viewport.Height = vpH
		}
		m.textarea.SetWidth(tuiTextareaInputWidth(m.mainWidth))
		if m.mdRenderer != nil {
			m.mdRenderer, _ = glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(m.transcriptContentWidth()), glamour.WithTableWrap(false))
		}
		m.syncViewport()
		return m, nil
	case tea.MouseMsg:
		if handled := m.handleMouseEvent(msg); handled {
			return m, nil
		}
	case submitInputMsg:
		if uint64(msg) != m.submitSeq || !m.pendingSubmitNewline {
			return m, nil
		}
		m.pendingSubmitNewline = false
		return m.submitTextareaInput()
	case tea.KeyMsg:
		if msg.Type != tea.KeyEnter && (msg.Paste || len(msg.Runes) > 0) {
			if m.pendingSubmitNewline {
				m.absorbPendingSubmitNewline()
				m.submitSeq++
			}
		}
		m.clearSelectionStatus()
		// Session picker overlay takes priority.
		if m.sessionPicker != nil {
			visibleItems := 15
			switch msg.Type {
			case tea.KeyEscape:
				m.sessionPicker = nil
				return m, nil
			case tea.KeyUp:
				if m.sessionPicker.selectedIdx > 0 {
					m.sessionPicker.selectedIdx--
					if m.sessionPicker.selectedIdx < m.sessionPicker.scrollOffset {
						m.sessionPicker.scrollOffset = m.sessionPicker.selectedIdx
					}
				}
				return m, nil
			case tea.KeyDown:
				if m.sessionPicker.selectedIdx < len(m.sessionPicker.sessions)-1 {
					m.sessionPicker.selectedIdx++
					if m.sessionPicker.selectedIdx >= m.sessionPicker.scrollOffset+visibleItems {
						m.sessionPicker.scrollOffset = m.sessionPicker.selectedIdx - visibleItems + 1
					}
				}
				return m, nil
			case tea.KeyEnter:
				if len(m.sessionPicker.sessions) > 0 {
					selected := m.sessionPicker.sessions[m.sessionPicker.selectedIdx]
					m.sessionPicker = nil
					return m, func() tea.Msg { return sessionSelectedMsg{session: selected} }
				}
				return m, nil
			}
			return m, nil
		}
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
		if m.slashPicker != nil {
			switch msg.Type {
			case tea.KeyEscape:
				m.slashPicker = nil
				return m, nil
			case tea.KeyUp, tea.KeyCtrlP:
				m.slashPicker.moveSelection(-1, slashPickerVisible)
				return m, nil
			case tea.KeyDown, tea.KeyCtrlN:
				m.slashPicker.moveSelection(1, slashPickerVisible)
				return m, nil
			case tea.KeyEnter:
				sel, ok := m.slashPicker.selected()
				if !ok {
					return m, nil
				}
				m.slashPicker = nil
				current := strings.TrimSpace(m.textarea.Value())
				if sel.TakesArgs && !strings.EqualFold(current, sel.Name) {
					m.applyTextareaValue(sel.Name + " ")
					return m, nil
				}
				m.applyTextareaValue(sel.Name)
				return m.submitTextareaInput()
			case tea.KeySpace:
				current := strings.TrimSpace(m.textarea.Value())
				if current == specSlashCommandName() {
					m.applyTextareaValue(specSlashCommandName() + " ")
					return m, nil
				}
			}
			// Key not consumed by the picker — let the outer switch and
			// the textarea handle it; updateSlashPicker reruns the filter.
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.textarea.Value() != "" {
				m.applyTextareaValue("")
				m.pendingSubmitNewline = false
				m.submitSeq++
				m.resetHistoryNavigation()
				m.exitHistorySearch()
				m.ctrlCPrimed = true
				return m, nil
			}
			if !m.ctrlCPrimed {
				m.ctrlCPrimed = true
				return m, nil
			}
			if m.streamActive {
				m.cancelOngoingTurn(false)
				return m, nil
			}
			if m.agentTurnCancel != nil {
				m.agentTurnCancel()
				m.agentTurnCancel = nil
			}
			m.saveConfig()
			return m, tea.Quit
		case tea.KeyEscape:
			if m.historySearchMode {
				m.exitHistorySearch()
			}
			m.ctrlCPrimed = false
			m.cancelOngoingTurn(true)
			if m.textarea.Value() != "" {
				m.applyTextareaValue("")
				m.pendingSubmitNewline = false
				m.submitSeq++
				m.resetHistoryNavigation()
				m.exitHistorySearch()
			}
			m.syncViewport()
			return m, nil
		case tea.KeyCtrlR:
			if !m.textarea.Focused() || m.streamActive {
				break
			}
			m.enterHistorySearch()
			return m, nil
		case tea.KeyEnd:
			m.setAutoFollow(true)
			m.syncViewport()
			return m, nil
		case tea.KeyShiftTab:
			m.autopilot = !m.autopilot
			m.saveConfig()
			m.syncViewport()
			return m, nil
		case tea.KeyRunes:
			if (msg.Paste || len(msg.Runes) > 1) && m.textarea.Focused() {
				if !m.streamActive {
					m.textarea.InsertString(string(msg.Runes))
					lines := strings.Count(m.textarea.Value(), "\n") + 1
					if lines > 1 {
						m.textareaExpanded = true
						m.textarea.SetHeight(tuiMax(3, lines))
					}
					m.textarea.CursorEnd()
					m.syncViewport()
					return m, textarea.Blink
				}
			}
		case tea.KeyCtrlO:
			if m.textarea.Focused() && strings.TrimSpace(m.textarea.Value()) == "" {
				// Toggle all tool results: if any are collapsed, expand all; otherwise collapse all
				anyCollapsed := false
				for _, entry := range m.entries {
					if entry.kind == transcriptToolResult && !m.expandedEntries[entry.id] {
						anyCollapsed = true
						break
					}
				}
				for _, entry := range m.entries {
					if entry.kind == transcriptToolResult {
						if anyCollapsed {
							m.expandedEntries[entry.id] = true
						} else {
							delete(m.expandedEntries, entry.id)
						}
					}
				}
				m.syncViewport()
				return m, nil
			}
		case tea.KeyUp, tea.KeyCtrlP:
			if !m.textarea.Focused() || m.streamActive {
				break
			}
			if m.historySearchMode {
				m.applyHistorySearchResult()
				return m, nil
			}
			m.setAutoFollow(false)
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
			m.updateAutoFollowFromViewport()
			m.cyclePromptHistory(1)
			return m, nil
		case tea.KeyCtrlJ:
			if m.textarea.Focused() && !m.streamActive {
				m.pendingSubmitNewline = false
				m.submitSeq++
				m.textarea.InsertString("\n")
				m.textareaExpanded = true
				lines := strings.Count(m.textarea.Value(), "\n") + 1
				m.textarea.SetHeight(tuiMax(3, lines))
				m.textarea.CursorEnd()
				m.syncViewport()
				return m, textarea.Blink
			}
		case tea.KeyCtrlV:
			if m.textarea.Focused() && m.clipboard != nil {
				text, err := m.clipboard.ReadAll()
				if err == nil && text != "" {
					m.pendingSubmitNewline = false
					m.submitSeq++
					m.textarea.InsertString(text)
					lines := strings.Count(m.textarea.Value(), "\n") + 1
					if lines > 1 {
						m.textareaExpanded = true
						m.textarea.SetHeight(tuiMax(3, lines))
					}
					m.textarea.CursorEnd()
					m.syncViewport()
					return m, textarea.Blink
				}
			}
		case tea.KeyEnter:
			if msg.Paste {
				if m.pendingSubmitNewline && m.textarea.Focused() && !m.streamActive && m.pendingPermission == nil {
					m.pendingSubmitNewline = false
					m.submitSeq++
				}
				break
			}
			if m.historySearchMode {
				m.exitHistorySearch()
				return m, nil
			}
			if !m.textarea.Focused() || m.streamActive && m.pendingPermission == nil {
				if m.streamActive && m.pendingPermission == nil {
					text := strings.TrimSpace(m.textarea.Value())
					if text == "" {
						break
					}
					m.queuedPrompt = text
					m.textarea.Reset()
					m.pendingSubmitNewline = false
					m.submitSeq++
					m.appendEntry(transcriptInfo, fmt.Sprintf("(queued: %s)", truncateString(text, 60)))
					m.syncViewport()
					return m, nil
				}
				break
			}
			if m.pendingPermission != nil {
				return m.handleApprovalInput(strings.TrimSpace(m.textarea.Value()))
			}
			m.submitSeq++
			m.pendingSubmitNewline = true
			return m, deferredSubmitInput(m.submitSeq)
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
		m.autoFollow = true
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
		cmds := []tea.Cmd{func() tea.Msg {
			if err := PostMessage(m.client, m.sessionID, "assistant", save); err != nil {
				return appendLineMsg("Warning: failed to save response: " + err.Error())
			}
			return appendLineMsg("")
		}}
		if m.queuedPrompt != "" {
			queued := m.queuedPrompt
			m.queuedPrompt = ""
			m.appendEntry(transcriptUser, queued)
			m.streamActive = true
			m.spinning = true
			m.streamBuf = ""
			cmds = append(cmds, m.spinner.Tick, m.sendAndStream(queued))
		}
		return m, tea.Batch(cmds...)
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
		content := strings.TrimSpace(msg.content)
		if content == "" {
			m.appendEntry(transcriptInfo, "(agent completed with no text response)")
		} else {
			m.appendEntry(transcriptAssistant, content)
		}
		m.syncViewport()
		if m.queuedPrompt != "" {
			queued := m.queuedPrompt
			m.queuedPrompt = ""
			m.appendEntry(transcriptUser, queued)
			m.streamActive = true
			m.spinning = true
			m.streamBuf = ""
			return m, tea.Batch(m.spinner.Tick, m.sendAndStream(queued))
		}
		return m, nil
	case agentProgressMsg:
		m.appendEntry(transcriptInfo, msg.text)
		m.syncViewport()
		return m, nil
	case agentToolResultMsg:
		m.nextEntryID++
		m.entries = append(m.entries, transcriptEntry{
			id:       m.nextEntryID,
			kind:     transcriptToolResult,
			content:  strings.TrimSpace(msg.content),
			toolName: msg.tool,
		})
		m.autoFollow = true
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
	case openSessionPickerMsg:
		m.sessionPicker = &sessionPickerState{sessions: msg.sessions}
		return m, nil
	case sessionSelectedMsg:
		// Load the selected session's history and switch to it.
		sess := msg.session
		return m, func() tea.Msg {
			state, messages, err := LoadSessionMessages(m.client, sess.ID)
			if err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Error loading session: " + err.Error()))
			}
			// Rebuild transcript from the loaded messages.
			return sessionLoadedMsg{sessionID: sess.ID, model: state.Model, apiShape: state.APIShape, agentBackend: state.AgentBackend, messages: messages}
		}
	case sessionLoadedMsg:
		m.sessionID = msg.sessionID
		if msg.model != "" {
			m.model = msg.model
		}
		if msg.apiShape != "" {
			m.apiShape = canonicalAPIShape(msg.apiShape)
		}
		if msg.agentBackend != "" {
			m.agentBackend = msg.agentBackend
		}
		m.entries = nil
		for _, message := range msg.messages {
			switch message.Role {
			case "user":
				m.appendEntry(transcriptUser, message.Content)
			case "assistant":
				m.appendEntry(transcriptAssistant, message.Content)
			}
		}
		m.appendEntry(transcriptInfo, fmt.Sprintf("Resumed session `%s`", msg.sessionID))
		m.syncViewport()
		return m, nil
	case sessionCreatedMsg:
		m.sessionID = msg.sessionID
		if msg.apiShape != "" {
			m.apiShape = canonicalAPIShape(msg.apiShape)
		}
		m.entries = nil
		m.appendEntry(transcriptInfo, fmt.Sprintf("Started new session `%s`", msg.sessionID))
		m.syncViewport()
		return m, nil
	case modelChangedMsg:
		m.model = string(msg)
		m.picker = nil
		m.appendEntry(transcriptInfo, fmt.Sprintf("Switched model to `%s`", m.model))
		m.saveConfig()
		m.syncViewport()
		return m, nil
	case agentBackendChangedMsg:
		m.agentBackend = string(msg)
		m.appendEntry(transcriptInfo, fmt.Sprintf("Switched agent backend to `%s`", m.agentBackend))
		m.saveConfig()
		m.syncViewport()
		return m, nil
	case apiShapeChangedMsg:
		m.apiShape = canonicalAPIShape(string(msg))
		m.appendEntry(transcriptInfo, fmt.Sprintf("Switched API shape to `%s`", formatAPIShape(m.apiShape)))
		m.saveConfig()
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
	prevYOffset := m.viewport.YOffset
	prevAtBottom := m.viewport.AtBottom()
	m.viewport, vpCmd = m.viewport.Update(msg)
	if _, isMouse := msg.(tea.MouseMsg); !isMouse {
		m.textarea, taCmd = m.textarea.Update(msg)
	}
	if m.viewport.YOffset != prevYOffset {
		if !prevAtBottom || m.viewport.YOffset < prevYOffset {
			m.setAutoFollow(false)
		}
		m.updateAutoFollowFromViewport()
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Paste && keyMsg.Type != tea.KeyRunes {
			lines := strings.Count(m.textarea.Value(), "\n") + 1
			if lines > 1 {
				m.textareaExpanded = true
				m.textarea.SetHeight(tuiMax(3, lines))
			}
		}
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
	// Auto-collapse expanded textarea when it becomes empty.
	if m.textareaExpanded && m.textarea.Value() == "" {
		m.textareaExpanded = false
		m.textarea.SetHeight(1)
	}
	m.updateSlashPicker()
	m.resizeViewportForCurrentLayout()
	if !m.textarea.Focused() || m.streamActive || m.pendingPermission != nil {
		m.pendingSubmitNewline = false
	}
	cmds = append(cmds, vpCmd, taCmd)
	return m, tea.Batch(cmds...)
}

// viewportHeight returns the transcript viewport height for the current layout.
func (m *chatTUIModel) viewportHeight(totalHeight int) int {
	if totalHeight <= 0 {
		return tuiViewportMinHeight
	}
	reserved := tuiBaseRows + m.extraTextareaRows() + m.slashPickerHeight()
	return max(totalHeight-reserved, tuiViewportMinHeight)
}

func (m *chatTUIModel) resizeViewportForCurrentLayout() {
	if !m.ready || m.height <= 0 {
		return
	}
	nextHeight := m.viewportHeight(m.height)
	if m.viewport.Height != nextHeight {
		m.viewport.Height = nextHeight
	}
}

func (m *chatTUIModel) slashPickerVisibleRows() int {
	if m.height <= 0 {
		return slashPickerVisible
	}
	available := m.height - tuiBaseRows - tuiViewportMinHeight - tuiSlashPickerFrameRows - m.extraTextareaRows()
	return max(minInt(slashPickerVisible, available), 0)
}

func (m *chatTUIModel) slashPickerHeight() int {
	if m.slashPicker == nil {
		return 0
	}
	visible := m.slashPickerVisibleRows()
	if visible == 0 {
		return 0
	}
	if visible > len(m.slashPicker.filtered) {
		visible = len(m.slashPicker.filtered)
	}
	if visible == 0 {
		visible = 1
	}
	return tuiSlashPickerFrameRows + visible
}

func (m *chatTUIModel) extraTextareaRows() int {
	return max(m.textarea.Height()-1, 0)
}

func (m *chatTUIModel) updateSlashPicker() {
	if m.streamActive || m.pendingPermission != nil || m.historySearchMode {
		m.slashPicker = nil
		return
	}
	value := m.textarea.Value()
	if strings.Contains(value, "\n") {
		m.slashPicker = nil
		return
	}
	trimmed := strings.TrimLeft(value, " \t")
	if !strings.HasPrefix(trimmed, "/") {
		m.slashPicker = nil
		return
	}
	if m.slashPicker == nil {
		m.slashPicker = newSlashPickerState()
	}
	m.slashPicker.setFilter(trimmed)
}

func (m chatTUIModel) View() string {
	if !m.ready {
		return "\n  Initializing…"
	}

	title := m.titleText()

	div := tuiDivStyle.Render(strings.Repeat("─", tuiMax(0, m.mainWidth)))
	var main strings.Builder
	main.WriteString(tuiTitleStyle.Width(m.mainWidth).Render(title))
	main.WriteString("\n")
	main.WriteString(div)
	main.WriteString("\n")
	main.WriteString(m.viewport.View())
	main.WriteString("\n")
	if overlay := m.renderSlashPicker(); overlay != "" {
		main.WriteString(overlay)
		main.WriteString("\n")
	}
	main.WriteString(m.renderTextarea())
	if status := m.renderFooterStatus(); status != "" {
		main.WriteString("\n")
		main.WriteString(status)
	}

	base := main.String()
	if m.sidebarWidth > 0 {
		sidebar := m.renderSidebar()
		base = placeOverlayTopRight(base, sidebar, m.width)
	}
	if m.picker == nil && m.sessionPicker == nil {
		return base
	}
	if m.sessionPicker != nil {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderSessionPickerModal())
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderPickerModal())
}

func (m chatTUIModel) titleText() string {
	title := "OmniLLM Chat"
	if m.mode != "" {
		title += "  │  " + m.mode
	}
	if m.model != "" {
		title += "  │  " + m.model
	}
	if m.specMode != "" {
		title += "  │  " + m.specMode
	}
	return title
}

func (m chatTUIModel) renderTextarea() string {
	contentWidth := m.transcriptBlockMaxWidth()
	inputWidth := tuiTextareaInputWidth(contentWidth)
	taView := m.textarea.View()
	if !strings.Contains(m.textarea.Value(), "\n") && m.textarea.Height() == 1 {
		taView = m.renderSingleLineTextarea(inputWidth)
	}

	shellInnerWidth := tuiMax(0, contentWidth-2)
	inner := lipgloss.NewStyle().Padding(0, 2, 0, 2).Width(inputWidth).Render(taView)
	return tuiInputShellStyle.Width(shellInnerWidth).Render(inner)
}

func (m chatTUIModel) renderSlashPicker() string {
	if m.slashPicker == nil {
		return ""
	}
	visible := m.slashPickerVisibleRows()
	if visible == 0 {
		return ""
	}
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Padding(0, 1)
	normalStyle := lipgloss.NewStyle().Padding(0, 1)

	width := tuiMax(40, m.transcriptBlockMaxWidth())
	if visible > len(m.slashPicker.filtered) {
		visible = len(m.slashPicker.filtered)
	}
	start := m.slashPicker.scrollOffset
	end := start + visible
	if end > len(m.slashPicker.filtered) {
		end = len(m.slashPicker.filtered)
	}

	var rows strings.Builder
	if len(m.slashPicker.filtered) == 0 {
		rows.WriteString(muted.Render("  No matching commands"))
	} else {
		for i := start; i < end; i++ {
			c := m.slashPicker.filtered[i]
			label := fmt.Sprintf("%-14s %s", c.Name, muted.Render(c.Summary))
			if i == m.slashPicker.selectedIdx {
				rows.WriteString(selectedStyle.Width(width - 4).Render(label))
			} else {
				rows.WriteString(normalStyle.Render(label))
			}
			if i < end-1 {
				rows.WriteString("\n")
			}
		}
	}

	header := lipgloss.NewStyle().Bold(true).Render("Commands")
	hint := muted.Render("Enter selects • Esc closes • ↑↓ navigate")
	body := lipgloss.JoinVertical(lipgloss.Left, header+"  "+hint, rows.String())
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(0, 1).Width(width - 2).Render(body)
}

func (m chatTUIModel) renderSingleLineTextarea(width int) string {
	if width <= 0 || m.textarea.Value() == "" {
		return m.textarea.View()
	}

	prompt := m.textarea.Prompt
	textWidth := tuiMax(1, width-xansi.StringWidth(prompt))
	line := []rune(m.textarea.Value())
	lineInfo := m.textarea.LineInfo()
	cursorIndex := lineInfo.StartColumn + lineInfo.ColumnOffset
	if cursorIndex < 0 {
		cursorIndex = 0
	}
	if cursorIndex > len(line) {
		cursorIndex = len(line)
	}

	start := 0
	if cursorIndex >= textWidth {
		start = cursorIndex - textWidth + 1
	}
	end := minInt(len(line), start+textWidth)
	if end < start {
		end = start
	}

	cursor := m.textarea.Cursor
	if cursorIndex < end {
		cursor.SetChar(string(line[cursorIndex]))
		return prompt + string(line[start:cursorIndex]) + cursor.View() + string(line[cursorIndex+1:end])
	}

	cursor.SetChar(" ")
	return prompt + string(line[start:end]) + cursor.View()
}

func tuiTextareaInputWidth(contentWidth int) int {
	return tuiMax(1, contentWidth-6)
}

func (m chatTUIModel) renderSessionPickerModal() string {
	sp := m.sessionPicker
	if sp == nil {
		return ""
	}
	modalWidth := minInt(90, tuiMax(60, m.width-8))
	visibleItems := 15
	start := minInt(tuiMax(0, sp.scrollOffset), len(sp.sessions))
	end := minInt(len(sp.sessions), start+visibleItems)

	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Padding(0, 1).Width(modalWidth - 6)
	normalStyle := lipgloss.NewStyle().Padding(0, 1).Width(modalWidth - 6)

	var rows strings.Builder
	if len(sp.sessions) == 0 {
		rows.WriteString(muted.Render("No sessions found."))
	} else {
		for i := start; i < end; i++ {
			sess := sp.sessions[i]
			title := sess.Title
			if title == "" {
				title = sess.ID
			}
			meta := sess.Model
			if meta == "" {
				meta = "default model"
			}
			if !sess.UpdatedAt.IsZero() {
				meta += "  •  " + sess.UpdatedAt.Format("2006-01-02 15:04")
			}
			if sess.MessageCount > 0 {
				meta += fmt.Sprintf("  •  %d msgs", sess.MessageCount)
			}
			rowText := title + "\n" + muted.Render("  "+meta)
			if i == sp.selectedIdx {
				rows.WriteString(selectedStyle.Render(rowText))
			} else {
				rows.WriteString(normalStyle.Render(rowText))
			}
			if i < end-1 {
				rows.WriteString("\n")
			}
		}
	}

	header := lipgloss.NewStyle().Bold(true).Render("Sessions")
	subtitle := muted.Render("Enter: resume  Esc: close  ↑↓: navigate")
	footer := muted.Render(fmt.Sprintf("%d sessions", len(sp.sessions)))
	content := lipgloss.JoinVertical(lipgloss.Left, header, subtitle, "", rows.String(), "", footer)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(1, 2).Width(modalWidth).Render(content)
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

func (m chatTUIModel) submitTextareaInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.textarea.Value())
	if text == "" {
		return m, nil
	}
	m.recordPromptHistory(text)
	m.textarea.Reset()
	m.pendingSubmitNewline = false
	m.resetHistoryNavigation()
	m.exitHistorySearch()
	if strings.HasPrefix(text, "?") && strings.TrimSpace(text) == "?" {
		return m.handleSlash(text)
	}
	if strings.HasPrefix(text, "/") {
		return m.handleSlash(text)
	}
	if strings.HasPrefix(text, "!") {
		command, _ := strings.CutPrefix(text, "!")
		command = strings.TrimSpace(command)
		if command == "" {
			return m, nil
		}
		m.appendEntry(transcriptUser, text)
		result := toolspkg.RunShellCommand(context.Background(), command, 0)
		m.appendEntry(transcriptAssistant, result.Output)
		m.syncViewport()
		return m, nil
	}
	m.appendEntry(transcriptUser, text)
	m.syncViewport()
	m.streamActive = true
	m.spinning = true
	m.streamBuf = ""
	return m, tea.Batch(m.spinner.Tick, m.sendAndStream(text))
}

func (m *chatTUIModel) syncViewport() {
	offset := m.viewport.YOffset
	m.viewport.SetContent(m.renderTranscript())
	if m.autoFollow {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(offset)
}

func (m *chatTUIModel) appendEntry(kind transcriptEntryType, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	m.nextEntryID++
	m.entries = append(m.entries, transcriptEntry{id: m.nextEntryID, kind: kind, content: content})
	m.autoFollow = true
}

func (m *chatTUIModel) renderTranscript() string {
	// Show welcome banner when transcript is empty and not streaming.
	if len(m.entries) == 0 && !m.streamActive && strings.TrimSpace(m.streamBuf) == "" {
		return m.renderWelcomeBanner()
	}

	blocks := make([]string, 0, len(m.entries)+1)
	layout := make([]transcriptLayoutEntry, 0, len(m.entries)+1)
	lineOffset := 0
	for idx, entry := range m.entries {
		var block string
		switch entry.kind {
		case transcriptUser:
			block = m.renderUserSection(entry.content, idx == m.hoveredEntry)
		case transcriptAssistant:
			block = m.renderAssistantSection(entry.content, idx == m.hoveredEntry)
		case transcriptError:
			block = tuiErrorStyle.Render(entry.content)
		case transcriptPermission:
			block = m.renderPermissionSection(entry.content)
		case transcriptToolResult:
			expanded := m.expandedEntries[entry.id]
			block = m.renderToolResultSection(entry.toolName, entry.content, expanded, idx == m.hoveredEntry)
		default:
			block = m.renderInfoSection(entry.content)
		}
		blocks = append(blocks, block)
		lineCount := strings.Count(block, "\n") + 1
		layoutEntry := transcriptLayoutEntry{kind: entry.kind, startLine: lineOffset, endLine: lineOffset + lineCount - 1}
		if entry.kind == transcriptToolResult {
			layoutEntry.clickableStartLine = layoutEntry.startLine
			layoutEntry.clickableEndLine = layoutEntry.endLine
		}
		layout = append(layout, layoutEntry)
		lineOffset += lineCount + 2
	}
	if m.streamActive || strings.TrimSpace(m.streamBuf) != "" {
		block := m.renderThinkingSection(m.streamBuf, len(m.entries) == m.hoveredEntry)
		blocks = append(blocks, block)
		lineCount := strings.Count(block, "\n") + 1
		layout = append(layout, transcriptLayoutEntry{kind: transcriptAssistant, startLine: lineOffset, endLine: lineOffset + lineCount - 1})
		lineOffset += lineCount + 2
	}
	m.transcriptLayout = layout
	transcript := strings.Join(blocks, "\n\n")
	if !m.selection.active {
		return transcript
	}
	return m.renderSelectionHighlight(transcript)
}

func renderOmnicodeLogo() string {
	rows := make([]string, 0, len(omnicodeLogoLeft))
	draw := func(line string, fg, shadow, bg lipgloss.Style) string {
		var row strings.Builder
		fgOnBg := fg.Background(bg.GetBackground())
		for _, char := range line {
			switch char {
			case '_':
				row.WriteString(bg.Render(" "))
			case '^':
				row.WriteString(fgOnBg.Render("▀"))
			case '~':
				row.WriteString(shadow.Render("▀"))
			case ' ':
				row.WriteRune(char)
			default:
				row.WriteString(fg.Render(string(char)))
			}
		}
		return row.String()
	}

	for i, left := range omnicodeLogoLeft {
		right := ""
		if i < len(omnicodeLogoRight) {
			right = omnicodeLogoRight[i]
		}
		rows = append(rows,
			draw(left, omnicodeLogoStyle.left, omnicodeLogoStyle.leftShadow, omnicodeLogoStyle.leftShadowBg)+" "+
				draw(right, omnicodeLogoStyle.right, omnicodeLogoStyle.rightShadow, omnicodeLogoStyle.rightShadowBg),
		)
	}
	return strings.Join(rows, "\n")
}

func (m chatTUIModel) renderWelcomeBanner() string {
	cwd, _ := os.Getwd()
	version := "v0.0.1"
	if data, err := os.ReadFile("VERSION"); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			version = v
		}
	}

	accentColor := lipgloss.Color("#7C3AED")
	subtleColor := lipgloss.Color("#9CA3AF")
	highlightColor := lipgloss.Color("#C4B5FD")

	title := omnicodeLogo

	subtitle := lipgloss.NewStyle().
		Foreground(subtleColor).
		Render(version + "  •  " + cwd)
	if m.model != "" {
		subtitle = lipgloss.NewStyle().
			Foreground(subtleColor).
			Render(version+"  •  "+cwd+"  •  ") +
			lipgloss.NewStyle().Foreground(highlightColor).Render(m.model)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(1, 4).
		Width(m.mainWidth - 4).
		Align(lipgloss.Center).
		Render(title + "\n" + subtitle)

	return lipgloss.PlaceHorizontal(m.mainWidth, lipgloss.Center, box) + "\n"
}

func (m chatTUIModel) renderSelectionHighlight(transcript string) string {
	content := strings.Split(transcript, "\n")
	if len(content) == 0 {
		return transcript
	}

	startLine := m.viewport.YOffset + m.selection.startY
	endLine := m.viewport.YOffset + m.selection.endY
	startCol := m.selection.startX
	endCol := m.selection.endX
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startLine, endLine = endLine, startLine
		startCol, endCol = endCol, startCol
	}
	startLine = minInt(max(0, startLine), len(content)-1)
	endLine = minInt(max(0, endLine), len(content)-1)

	for lineIdx := startLine; lineIdx <= endLine; lineIdx++ {
		line := content[lineIdx]
		left := 0
		right := xansi.StringWidth(line)
		if lineIdx == startLine {
			left = startCol
		}
		if lineIdx == endLine {
			right = endCol + 1
		}
		if right < left {
			left, right = right, left
		}
		content[lineIdx] = highlightVisibleRange(line, left, right)
	}
	return strings.Join(content, "\n")
}

func highlightVisibleRange(line string, left, right int) string {
	if right <= left {
		return line
	}
	prefix := xansi.Cut(line, 0, left)
	segment := xansi.Strip(xansi.Cut(line, left, right))
	suffix := xansi.Cut(line, right, xansi.StringWidth(line))
	if strings.TrimSpace(segment) == "" {
		return line
	}
	return prefix + tuiSelectionStyle.Render(segment) + suffix
}

func (m chatTUIModel) renderFooterStatus() string {
	if m.pendingPermission != nil {
		return tuiStatusStyle.Width(m.transcriptBlockMaxWidth()).Render(m.approvalPromptText)
	}
	if status := m.historySearchStatus(); status != "" {
		return status
	}
	if status := m.selectionStatus(); status != "" {
		return status
	}
	if m.streamActive {
		return tuiStatusStyle.Width(m.transcriptBlockMaxWidth()).Render("Streaming response… Esc cancels")
	}
	if m.queuedPrompt != "" {
		return tuiStatusStyle.Width(m.transcriptBlockMaxWidth()).Render("Queued prompt ready")
	}
	return tuiStatusStyle.Width(m.transcriptBlockMaxWidth()).Render("Enter send · Ctrl+J newline · Ctrl+O toggle all blocks · Ctrl+R search · Shift+Tab autopilot · /help commands")
}

func (m chatTUIModel) renderPermissionChip() string {
	if m.pendingPermission != nil {
		return tuiPermPendingChipStyle.Render("APPROVAL REQUIRED")
	}
	if m.autopilot {
		return tuiPermAutoChipStyle.Render("AUTOPILOT")
	}
	return tuiPermManualChipStyle.Render("MANUAL APPROVAL")
}

func (m chatTUIModel) transcriptContentWidth() int {
	return tuiMax(20, m.transcriptBlockMaxWidth()-2)
}

func (m chatTUIModel) transcriptBlockMaxWidth() int {
	return tuiMax(8, m.mainWidth)
}

func (m chatTUIModel) renderUserSection(text string, hovered bool) string {
	label := tuiUserLabelStyle.Render("User")
	style := tuiUserBlockStyle
	if hovered {
		style = tuiUserHoverStyle
	}
	blockWidth := m.transcriptBlockMaxWidth()
	block := style.Width(blockWidth).MaxWidth(blockWidth).Render(text)
	return lipgloss.JoinVertical(lipgloss.Left, label, block)
}

func (m chatTUIModel) renderAssistantSection(text string, hovered bool) string {
	rendered := m.renderMD(text)
	label := tuiAssistantLabelStyle.Render("Assistant")
	body := m.renderAssistantBody(rendered, hovered)
	return lipgloss.JoinVertical(lipgloss.Left, label, body)
}

func (m chatTUIModel) renderThinkingSection(text string, hovered bool) string {
	body := strings.TrimSpace(text)
	if body == "" {
		body = m.spinner.View() + " thinking…"
	} else if m.spinning {
		body = m.spinner.View() + " " + body
	}
	label := tuiThinkingLabelStyle.Render("Thinking")
	block := m.renderThinkingBody(tuiThinkingStyle.Render(body), hovered)
	return lipgloss.JoinVertical(lipgloss.Left, label, block)
}

func (m chatTUIModel) renderAssistantBody(text string, hovered bool) string {
	plain := xansi.Strip(text)
	lines := strings.Split(plain, "\n")
	styled := make([]string, len(lines))
	style := tuiAssistantBlockStyle
	if hovered {
		style = tuiAssistantHoverStyle
	}
	blockWidth := m.transcriptBlockMaxWidth()
	for i, line := range lines {
		styled[i] = style.Width(blockWidth).MaxWidth(blockWidth).Render(line)
	}
	return strings.Join(styled, "\n")
}

func (m chatTUIModel) renderThinkingBody(text string, hovered bool) string {
	plain := xansi.Strip(text)
	lines := strings.Split(plain, "\n")
	styled := make([]string, len(lines))
	style := tuiThinkingBlockStyle
	if hovered {
		style = tuiThinkingHoverStyle
	}
	blockWidth := m.transcriptBlockMaxWidth()
	for i, line := range lines {
		styled[i] = style.Width(blockWidth).MaxWidth(blockWidth).Render(line)
	}
	return strings.Join(styled, "\n")
}

func (m chatTUIModel) renderInfoSection(text string) string {
	return tuiStatusStyle.Width(tuiMax(8, m.transcriptBlockMaxWidth())).Render(text)
}

func (m chatTUIModel) renderToolResultSection(toolName, content string, expanded, hovered bool) string {
	label := tuiToolResultLabelStyle.Render("●") + " " + tuiToolResultNameStyle.Render(fmt.Sprintf("Tool %s", toolName))
	rendered := m.renderToolResultOutput(content, expanded)
	return m.renderToolResultItem(lipgloss.JoinVertical(lipgloss.Left, label, rendered), hovered)
}

func (m chatTUIModel) renderToolResultOutput(content string, expanded bool) string {
	content = normalizeToolResultContent(content)
	lines := strings.Split(content, "\n")
	maxLines := toolResultMaxLines
	if expanded {
		maxLines = len(lines)
	}

	width := tuiMax(8, m.transcriptBlockMaxWidth()-2)
	out := make([]string, 0, minInt(len(lines), maxLines)+1)
	for i, line := range lines {
		if i >= maxLines {
			break
		}
		line = " " + line
		if xansi.StringWidth(line) > width {
			line = xansi.Truncate(line, width, "…")
		}
		out = append(out, tuiToolResultLineStyle.Width(width).Render(line))
	}

	if !expanded && len(lines) > toolResultMaxLines {
		out = append(out, tuiToolResultLineStyle.Width(width).Render(fmt.Sprintf("… (%d lines hidden)", len(lines)-toolResultMaxLines)))
	}
	return strings.Join(out, "\n")
}

func (m chatTUIModel) renderToolResultItem(text string, focused bool) string {
	style := tuiToolResultBlurredStyle
	if focused {
		style = tuiToolResultFocusStyle
	}
	prefix := style.Render("")
	contentWidth := tuiMax(1, m.transcriptBlockMaxWidth()-lipgloss.Width(prefix))
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + xansi.Truncate(line, contentWidth, "…")
	}
	return strings.Join(lines, "\n")
}

func normalizeToolResultContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\t", "    ")
	return strings.TrimSpace(content)
}

func (m chatTUIModel) renderPermissionSection(text string) string {
	label := tuiPermissionLabelStyle.Render("Permission required")
	body := tuiPermissionBlockStyle.Width(m.transcriptBlockMaxWidth()).MaxWidth(m.transcriptBlockMaxWidth()).Render(text)
	return lipgloss.JoinVertical(lipgloss.Left, label, body)
}

func displayAgentBackendName(agentBackend string) string {
	switch strings.TrimSpace(strings.ToLower(agentBackend)) {
	case "google-adk":
		return "Omnicode"
	default:
		return agentBackend
	}
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

	permMode := "Manual approval"
	if m.autopilot {
		permMode = "Autopilot"
	}
	if m.pendingPermission != nil {
		permMode = "Waiting: " + m.pendingPermission.req.ToolName
	}
	workingDir, err := os.Getwd()
	if err != nil || strings.TrimSpace(workingDir) == "" {
		workingDir = "unknown"
	}

	sections := []string{
		tuiSidebarHeaderStyle.Render("Context"),
		tuiSidebarLabelStyle.Render("Permissions") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(permMode),
		tuiSidebarLabelStyle.Render("Working dir") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(workingDir),
		tuiSidebarLabelStyle.Render("Session") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(m.sessionID),
		tuiSidebarLabelStyle.Render("Model") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(m.model),
		tuiSidebarLabelStyle.Render("Mode") + "\n" + tuiSidebarValueStyle.Width(valueWidth).Render(m.mode),
		tuiSidebarLabelStyle.Render("Status") + "\n" + statusDot + " " + status,
	}
	if m.agentBackend != "" {
		agentLabel := displayAgentBackendName(m.agentBackend)
		sections = append(sections, tuiSidebarLabelStyle.Render("Agent")+"\n"+tuiSidebarValueStyle.Width(valueWidth).Render(agentLabel))
	}
	if m.specMode != "" {
		specLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render(m.specMode)
		sections = append(sections, tuiSidebarLabelStyle.Render("Spec")+"\n"+specLabel)
	}
	sections = append(sections,
		tuiSidebarHeaderStyle.Render("Actions"),
		tuiSidebarValueStyle.Width(valueWidth).Render("Shift+Tab  Toggle autopilot"),
		tuiSidebarValueStyle.Width(valueWidth).Render("/models    Switch model"),
		tuiSidebarValueStyle.Width(valueWidth).Render("/sessions  Resume chat"),
	)
	return lipgloss.NewStyle().Width(m.sidebarWidth).Render(tuiSidebarStyle.Width(m.sidebarWidth).Height(tuiMax(8, m.height-1)).Render(strings.Join(sections, "\n\n")))
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 3 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// placeOverlayTopRight overlays the sidebar at the right edge of the terminal.
// The sidebar stays fixed in position regardless of base content height.
func placeOverlayTopRight(base, sidebar string, totalWidth int) string {
	baseLines := strings.Split(base, "\n")
	sideLines := strings.Split(sidebar, "\n")
	sidebarWidth := lipgloss.Width(sidebar)

	result := make([]string, 0, len(baseLines))
	for i, baseLine := range baseLines {
		if i < len(sideLines) {
			sideLine := sideLines[i]
			padding := totalWidth - lipgloss.Width(baseLine) - sidebarWidth
			if padding < 0 {
				padding = 0
			}
			result = append(result, baseLine+strings.Repeat(" ", padding)+sideLine)
		} else {
			result = append(result, baseLine)
		}
	}
	for i := len(baseLines); i < len(sideLines); i++ {
		padding := totalWidth - sidebarWidth
		if padding < 0 {
			padding = 0
		}
		result = append(result, strings.Repeat(" ", padding)+sideLines[i])
	}
	return strings.Join(result, "\n")
}

func (m *chatTUIModel) cancelOngoingTurn(recordCancellation bool) {
	wasActive := m.streamActive || m.agentTurnCancel != nil || m.pendingPermission != nil || strings.TrimSpace(m.streamBuf) != ""
	m.streamActive = false
	m.spinning = false
	m.streamBuf = ""
	m.pendingSubmitNewline = false
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
	if recordCancellation && wasActive {
		m.appendEntry(transcriptInfo, "(cancelled)")
	}
}

func (m *chatTUIModel) saveConfig() {
	if m.onConfigSave != nil {
		m.onConfigSave(m.model, m.mode, m.apiShape, m.agentBackend, m.specMode, m.autopilot, m.maxTurns)
	}
}

func (m *chatTUIModel) clearSelection() {
	m.selection = transcriptSelection{pressLine: -1, pressIdx: -1, pressID: -1}
}

func (m *chatTUIModel) copySelection() {
	selected := m.selectedTranscriptText()
	if selected == "" {
		m.clearSelection()
		m.selectionMessage = ""
		return
	}
	if m.clipboard == nil {
		m.selectionMessage = fmt.Sprintf("Selected %d chars", len([]rune(selected)))
		return
	}
	_ = m.clipboard.WriteAll(selected)
	m.selectionMessage = fmt.Sprintf("Copied %d chars", len([]rune(selected)))
}

func (m *chatTUIModel) setAutoFollow(enabled bool) {
	m.autoFollow = enabled
	if enabled {
		m.viewport.GotoBottom()
	}
}

func (m *chatTUIModel) updateAutoFollowFromViewport() {
	m.autoFollow = m.viewport.AtBottom()
}

func (m *chatTUIModel) viewportBounds() (left, top, right, bottom int) {
	left = 0
	title := tuiTitleStyle.Width(m.mainWidth).Render(m.titleText())
	top = lipgloss.Height(title) + 1
	right = m.mainWidth
	bottom = top + m.viewport.Height
	return left, top, right, bottom
}

func (m *chatTUIModel) viewportMousePosition(x, y int) (int, int, bool) {
	left, top, right, bottom := m.viewportBounds()
	if x < left || x >= right || y < top || y >= bottom {
		return 0, 0, false
	}
	return x - left, y - top, true
}

func (m *chatTUIModel) clearSelectionStatus() {
	m.selectionMessage = ""
}

func (m *chatTUIModel) hoveredTranscriptEntry(localY int) int {
	lineIdx := m.viewport.YOffset + localY
	for idx, entry := range m.transcriptLayout {
		if lineIdx >= entry.startLine && lineIdx <= entry.endLine {
			switch entry.kind {
			case transcriptUser, transcriptAssistant, transcriptToolResult:
				return idx
			}
			return -1
		}
	}
	return -1
}

func (m *chatTUIModel) updateHoveredEntry(localY int, insideViewport bool) bool {
	next := -1
	if insideViewport {
		next = m.hoveredTranscriptEntry(localY)
	}
	if next == m.hoveredEntry {
		return false
	}
	m.hoveredEntry = next
	m.syncViewport()
	return true
}

func (m *chatTUIModel) toolResultIsExpandable(entry transcriptEntry) bool {
	return entry.kind == transcriptToolResult
}

func (m *chatTUIModel) scrollEntryIntoView(entryIdx int) {
	if entryIdx < 0 || entryIdx >= len(m.transcriptLayout) {
		return
	}
	layout := m.transcriptLayout[entryIdx]
	if layout.startLine < m.viewport.YOffset {
		m.viewport.SetYOffset(layout.startLine)
		m.setAutoFollow(false)
		return
	}
	bottom := m.viewport.YOffset + m.viewport.Height - 1
	if layout.endLine > bottom {
		offset := layout.endLine - m.viewport.Height + 1
		if offset < 0 {
			offset = 0
		}
		m.viewport.SetYOffset(offset)
		m.setAutoFollow(false)
	}
}

func (m *chatTUIModel) handleMouseEvent(msg tea.MouseMsg) bool {
	if !m.ready || m.picker != nil || m.sessionPicker != nil {
		return false
	}

	if m.middleDragging && msg.Action == tea.MouseActionRelease {
		m.middleDragging = false
		m.updateAutoFollowFromViewport()
		return true
	}
	if m.selection.selecting && msg.Action == tea.MouseActionRelease {
		m.finishTranscriptSelection(msg)
		return true
	}

	mouseX, mouseY, insideViewport := m.viewportMousePosition(msg.X, msg.Y)
	if msg.Action == tea.MouseActionMotion {
		if m.updateHoveredEntry(mouseY, insideViewport) {
			return true
		}
	} else if !insideViewport && m.hoveredEntry != -1 {
		m.hoveredEntry = -1
		m.syncViewport()
		return true
	}

	if m.middleDragging {
		if msg.Action == tea.MouseActionMotion {
			delta := msg.Y - m.middleDragStartY
			m.viewport.SetYOffset(m.middleDragStartOffset + delta)
			m.setAutoFollow(false)
			m.updateAutoFollowFromViewport()
			return true
		}
		return true
	}

	if m.selection.selecting {
		if msg.Action == tea.MouseActionMotion && insideViewport {
			m.selection.endX = mouseX
			m.selection.endY = mouseY
			m.selection.active = true
			m.syncViewport()
			return true
		}
		return false
	}

	if !insideViewport {
		return false
	}

	switch {
	case msg.Button == tea.MouseButtonMiddle && msg.Action == tea.MouseActionPress:
		m.middleDragging = true
		m.middleDragStartY = msg.Y
		m.middleDragStartOffset = m.viewport.YOffset
		m.setAutoFollow(false)
		return true
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		pressIdx := m.hoveredTranscriptEntry(mouseY)
		pressID := int64(-1)
		if pressIdx >= 0 && pressIdx < len(m.entries) {
			pressID = m.entries[pressIdx].id
		}
		m.selection.active = true
		m.selection.selecting = true
		m.selection.startX = mouseX
		m.selection.startY = mouseY
		m.selection.endX = mouseX
		m.selection.endY = mouseY
		m.selection.pressLine = m.viewport.YOffset + mouseY
		m.selection.pressIdx = pressIdx
		m.selection.pressID = pressID
		m.syncViewport()
		return true
	default:
		return false
	}
}

func (m *chatTUIModel) finishTranscriptSelection(msg tea.MouseMsg) {
	m.selection.selecting = false
	mouseX, mouseY, insideViewport := m.viewportMousePosition(msg.X, msg.Y)
	if insideViewport {
		m.selection.endX = mouseX
		m.selection.endY = mouseY
	}
	plainClick := m.selection.startX == m.selection.endX && m.selection.startY == m.selection.endY
	if plainClick {
		pressIdx := m.selection.pressIdx
		pressLine := m.selection.pressLine
		pressID := m.selection.pressID
		m.clearSelection()
		if insideViewport {
			idx := m.transcriptEntryIndexByID(pressID)
			if idx < 0 {
				idx = pressIdx
			}
			if idx < 0 || idx >= len(m.entries) {
				idx = m.hoveredTranscriptEntry(mouseY)
			}
			if pressIdx >= 0 && idx >= 0 && idx != pressIdx {
				m.syncViewport()
				return
			}
			if idx >= 0 && idx < len(m.entries) {
				entry := m.entries[idx]
				layout := m.transcriptLayout[idx]
				if pressLine <= 0 {
					pressLine = m.viewport.YOffset + mouseY
				}
				hitPressLine := pressLine + tuiToolResultHitRowOffset
				if entry.kind == transcriptToolResult &&
					hitPressLine >= layout.clickableStartLine && hitPressLine <= layout.clickableEndLine {
					if m.expandedEntries[entry.id] {
						delete(m.expandedEntries, entry.id)
					} else {
						m.expandedEntries[entry.id] = true
					}
				}
			}
		}
		m.syncViewport()
		return
	}
	selected := m.selectedTranscriptText()
	if selected == "" {
		m.clearSelection()
		m.syncViewport()
		return
	}
	m.selection.active = true
	m.copySelection()
	m.syncViewport()
}

func (m *chatTUIModel) transcriptEntryIndexByID(id int64) int {
	if id <= 0 {
		return -1
	}
	for idx, entry := range m.entries {
		if entry.id == id {
			return idx
		}
	}
	return -1
}

func (m chatTUIModel) selectedTranscriptText() string {
	content := strings.Split(m.renderTranscript(), "\n")
	if len(content) == 0 {
		return ""
	}

	startLine := m.viewport.YOffset + m.selection.startY
	endLine := m.viewport.YOffset + m.selection.endY
	startCol := m.selection.startX
	endCol := m.selection.endX
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startLine, endLine = endLine, startLine
		startCol, endCol = endCol, startCol
	}
	startLine = minInt(max(0, startLine), len(content)-1)
	endLine = minInt(max(0, endLine), len(content)-1)

	parts := make([]string, 0, endLine-startLine+1)
	for lineIdx := startLine; lineIdx <= endLine; lineIdx++ {
		line := content[lineIdx]
		left := 0
		right := xansi.StringWidth(line)
		if lineIdx == startLine {
			left = startCol
		}
		if lineIdx == endLine {
			right = endCol + 1
		}
		if right < left {
			left, right = right, left
		}
		segment := strings.TrimRight(xansi.Strip(xansi.Cut(line, left, right)), " ")
		parts = append(parts, segment)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (m *chatTUIModel) selectionStatus() string {
	if strings.TrimSpace(m.selectionMessage) == "" {
		if m.selection.active {
			return tuiHelpStyle.Render("Selection active · drag to adjust · release copies")
		}
		return ""
	}
	return tuiHelpStyle.Render(m.selectionMessage)
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
	if strings.TrimSpace(md) == "" {
		return md
	}
	if rendered, ok := m.renderMarkdownWithTables(md); ok {
		return rendered
	}
	return m.renderMarkdownFragment(md)
}

func (m chatTUIModel) renderMarkdownFragment(md string) string {
	if m.mdRenderer == nil {
		return md
	}
	out, err := m.mdRenderer.Render(md)
	if err != nil {
		return md
	}
	plain := xansi.Strip(strings.TrimRight(out, "\n"))
	return trimCommonLeadingSpaces(plain)
}

func (m chatTUIModel) renderMarkdownWithTables(md string) (string, bool) {
	lines := strings.Split(md, "\n")
	blocks := make([]string, 0, len(lines))
	proseStart := 0
	hasTable := false

	flushProse := func(end int) {
		if end <= proseStart {
			return
		}
		fragment := strings.Join(lines[proseStart:end], "\n")
		if strings.TrimSpace(fragment) == "" {
			proseStart = end
			return
		}
		blocks = append(blocks, m.renderMarkdownFragment(fragment))
		proseStart = end
	}

	for i := 0; i < len(lines); {
		headers, rows, next, ok := parseMarkdownTableBlock(lines, i)
		if !ok {
			i++
			continue
		}
		flushProse(i)
		blocks = append(blocks, m.renderTableGrid(headers, rows))
		hasTable = true
		i = next
		proseStart = next
	}
	flushProse(len(lines))
	if !hasTable {
		return "", false
	}
	return strings.Join(blocks, "\n"), true
}

func parseMarkdownTableBlock(lines []string, start int) ([]string, [][]string, int, bool) {
	if start >= len(lines) {
		return nil, nil, start, false
	}
	trimmed := strings.TrimSpace(lines[start])
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil, nil, start, false
	}

	end := start
	for end < len(lines) {
		trimmed = strings.TrimSpace(lines[end])
		if trimmed == "" {
			break
		}
		if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
			break
		}
		end++
	}

	rows := make([][]string, 0, end-start)
	hasSeparator := false
	for i := start; i < end; i++ {
		trimmed = strings.TrimSpace(lines[i])
		if isMarkdownTableSeparator(trimmed) {
			hasSeparator = true
			continue
		}
		rows = append(rows, parseMarkdownTableRow(trimmed))
	}
	if !hasSeparator || len(rows) < 2 {
		return nil, nil, start, false
	}
	colCount := len(rows[0])
	if colCount == 0 {
		return nil, nil, start, false
	}
	for _, row := range rows[1:] {
		if len(row) != colCount {
			return nil, nil, start, false
		}
	}
	return rows[0], rows[1:], end, true
}

func (m chatTUIModel) renderMarkdownTable(md string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(md), "\n")
	headers, rows, _, ok := parseMarkdownTableBlock(lines, 0)
	if !ok {
		return "", false
	}
	return m.renderTableGrid(headers, rows), true
}

func isMarkdownTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', '-', ':', ' ':
		default:
			return false
		}
	}
	return true
}

func parseMarkdownTableRow(line string) []string {
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts)-2)
	for i := 1; i < len(parts)-1; i++ {
		cells = append(cells, strings.TrimSpace(parts[i]))
	}
	return cells
}

func (m chatTUIModel) renderTableGrid(headers []string, rows [][]string) string {
	availableWidth := m.transcriptContentWidth()
	tbl := liptable.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(true).
		BorderRight(true).
		BorderHeader(true).
		BorderColumn(true).
		BorderRow(true).
		BorderStyle(tuiTableBorderStyle).
		Width(availableWidth).
		Wrap(false).
		Headers(headers...).
		Rows(rows...)
	tbl.StyleFunc(func(row, col int) lipgloss.Style {
		switch {
		case row == liptable.HeaderRow:
			return tuiTableHeaderStyle
		case col == 0:
			return tuiTableAccentStyle.Padding(0, 1)
		default:
			return tuiTableCellStyle
		}
	})
	return tbl.String()
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

func defaultMode(current, saved string) string {
	if saved != "" {
		return saved
	}
	return current
}

func defaultAPIShape(current, saved string) string {
	if saved != "" {
		return canonicalAPIShape(saved)
	}
	return canonicalAPIShape(current)
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
		apiShape := m.apiShape
		maxTurns := m.maxTurns
		return func() tea.Msg {
			defer cancel()
			eventCh, err := StreamAgentTurnWithChecker(ctx, client, sessionID, model, backend, apiShape, userText, checker, maxTurns)
			if err != nil {
				return agentDoneMsg{err: err}
			}
			var finalContent string
			for event := range eventCh {
				switch event.Type {
				case agentpkg.EventToken:
					finalContent += event.Content
				case agentpkg.EventToolCall:
					finalContent = ""
					if prog != nil {
						prog.Send(agentProgressMsg{text: formatToolCallProgress(event.Tool, event.Content)})
					}
				case agentpkg.EventToolResult:
					if prog != nil {
						prog.Send(agentToolResultMsg{tool: event.Tool, content: event.Content})
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

func formatToolCallProgress(toolName, payload string) string {
	if strings.TrimSpace(payload) == "" {
		return fmt.Sprintf("🔧 Calling tool `%s`…", toolName)
	}
	return fmt.Sprintf("🔧 Calling tool `%s`: %s", toolName, payload)
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
		m.saveConfig()
		return m, tea.Quit
	case "/clear", "/cls":
		m.entries = nil
		m.syncViewport()
		return m, nil
	case "/help", "?":
		add(m.renderMD(renderSlashHelp(slashCommands())))
		return m, nil
	case "/session":
		add(m.renderMD(fmt.Sprintf("**Session:** `%s`\n**Mode:** `%s`\n**API Shape:** `%s`\n**Model:** `%s`", m.sessionID, m.mode, formatAPIShape(m.apiShape), m.model)))
		return m, nil
	case "/mode":
		result := agentpkg.ParseCommand(text, m.mode)
		if result.NewMode != nil {
			m.mode = *result.NewMode
		}
		add(m.renderMD(result.Response))
		m.saveConfig()
		return m, nil
	case "/permissions":
		m.autopilot = !m.autopilot
		status := "manual approval"
		if m.autopilot {
			status = "autopilot (tools auto-approved)"
		}
		add(m.renderMD(fmt.Sprintf("Permissions: **%s**", status)))
		m.saveConfig()
		m.syncViewport()
		return m, nil
	case "/max-turns":
		if len(fields) == 1 {
			add(m.renderMD(fmt.Sprintf("Max turns: **%d**", m.maxTurns)))
			return m, nil
		}
		if n, err := strconv.Atoi(fields[1]); err != nil || n < 1 || n > 100 {
			add(tuiErrorStyle.Render("Max turns must be between 1 and 100"))
			return m, nil
		} else {
			m.maxTurns = n
			add(m.renderMD(fmt.Sprintf("Max turns set to **%d**", n)))
			m.saveConfig()
		}
		return m, nil
	case "/apishape", "/api-shape":
		if len(fields) == 1 {
			add(m.renderMD(fmt.Sprintf("Current API shape: `%s`\n\nSupported shapes: `%s`", formatAPIShape(m.apiShape), supportedAPIShapesText())))
			return m, nil
		}
		newShape, ok := normalizeAPIShape(fields[1])
		if !ok || newShape == "responses" {
			add(tuiErrorStyle.Render(fmt.Sprintf("Error: unknown API shape %q — supported shapes: %s", fields[1], supportedAPIShapesText())))
			return m, nil
		}
		return m, func() tea.Msg {
			if err := UpdateSessionAPIShape(m.client, m.sessionID, newShape); err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Error: " + err.Error()))
			}
			return apiShapeChangedMsg(newShape)
		}
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
	case "/sessions":
		return m, func() tea.Msg {
			sessions, err := ListSessions(m.client)
			if err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Error: " + err.Error()))
			}
			if len(sessions) == 0 {
				return appendLineMsg("No sessions found.")
			}
			return openSessionPickerMsg{sessions: sessions}
		}
	case "/new":
		title := ""
		if len(fields) > 1 {
			title = strings.Join(fields[1:], " ")
		}
		currentModel := m.model
		currentAPIShape := m.apiShape
		currentBackend := m.agentBackend
		return m, func() tea.Msg {
			sid, err := CreateSession(m.client, title, currentModel, currentAPIShape, currentBackend)
			if err != nil {
				return appendLineMsg(tuiErrorStyle.Render("Error: " + err.Error()))
			}
			return sessionCreatedMsg{sessionID: sid, apiShape: currentAPIShape}
		}
	default:
		add(tuiErrorStyle.Render(fmt.Sprintf("Unknown command: %s — use /help", fields[0])))
		return m, nil
	case "/spec":
		sub := ""
		if len(fields) > 1 {
			sub = strings.ToLower(fields[1])
		}
		if sub == "" || sub == "help" {
			add(m.renderMD(specHelpMarkdown()))
		} else if sub == "mode" {
			if len(fields) < 3 {
				current := m.specMode
				if current == "" {
					current = "off"
				}
				add(m.renderMD(fmt.Sprintf("Current spec mode: **%s**\n\nUsage: `/spec mode <spec-kit|openspec|off>`", current)))
			} else {
				newMode := strings.ToLower(fields[2])
				if newMode == "off" {
					m.specMode = ""
					add(m.renderMD("Spec mode **disabled**."))
					m.saveConfig()
				} else if isValidSpecMode(newMode) {
					m.specMode = newMode
					m.mode = "agent"
					var summary string
					if newMode == "spec-kit" {
						summary = specKitWorkflowSummary()
					} else {
						summary = openSpecWorkflowSummary()
					}
					add(m.renderMD(fmt.Sprintf("Spec mode set to **%s**. Switched to **agent** mode.\n\n%s", newMode, summary)))
					m.saveConfig()
				} else {
					add(tuiErrorStyle.Render(fmt.Sprintf("Unknown spec mode %q. Valid modes: spec-kit, openspec, off", newMode)))
				}
			}
		} else {
			session := &SessionState{SpecMode: m.specMode}
			var sb strings.Builder
			handleSpecCommand(&sb, fields, session)
			add(m.renderMD(sb.String()))
		}
		return m, nil
	}
}

func RunTUI(c Client, sessionID, model, mode, apiShape, agentBackend string, history []Message) error {
	m := newChatTUIModel(c, sessionID, model, mode, apiShape, agentBackend, history, ConfigSaveCallback)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	go func() { p.Send(progReadyMsg{p: p}) }()
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}
