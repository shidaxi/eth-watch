package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time
type dataMsg []NodeInfo

// k8sDiscoveringMsg is sent when K8s discovery starts.
type k8sDiscoveringMsg struct{}

// k8sDiscoveredMsg carries the live RPC URLs found by K8s discovery.
type k8sDiscoveredMsg struct {
	rpcs []string
	err  error
}

type column struct {
	name  string
	width int
}

var columns = []column{
	{name: "URL", width: 30},
	{name: "ChainID", width: 13},
	{name: "Latest", width: 14},
	{name: "Hash", width: 13},
	{name: "Safe", width: 14},
	{name: "Finalized", width: 14},
	{name: "Syncing", width: 9},
	{name: "Peers", width: 7},
	{name: "Version", width: 28},
	{name: "Updated", width: 12},
}

const (
	colURL       = 0
	colChainID   = 1
	colLatest    = 2
	colHash      = 3
	colSafe      = 4
	colFinalized = 5
	colSyncing   = 6
	colPeers     = 7
	colVersion   = 8
	colUpdated   = 9
)

type displayMode int

const (
	modeWide   displayMode = iota
	modeSimple
)

// simpleColMap maps simple-mode column positions to the global column constants.
var simpleColMap = []int{colURL, colLatest, colHash}

type model struct {
	nodes      []NodeInfo
	sortCol    int
	sortAsc    bool
	interval   time.Duration
	rpcs       []string
	lastUpdate time.Time
	width      int
	height     int

	// Display mode
	dispMode displayMode

	// K8s mode
	inK8s       bool
	discovering bool
	discoverErr string

	// Filter
	filterMode bool
	filterStr  string
	filterRe   *regexp.Regexp

	// Pagination
	page     int
	pageSize int

	// Quit confirmation
	escPending bool
}

var (
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("237"))
	selectedColStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	normalStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	diffStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	syncedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	syncingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	k8sBadgeStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("27")).Padding(0, 1)
	filterActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	filterLabelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func newModel(rpcs []string, interval time.Duration, inK8s bool) model {
	nodes := make([]NodeInfo, len(rpcs))
	for i, url := range rpcs {
		nodes[i] = NodeInfo{URL: url, LatestBlock: "...", LatestHash: "...", SafeBlock: "...", FinalBlock: "...", Syncing: "...", Version: "...", PeerCount: "...", ChainID: "..."}
	}
	return model{
		nodes:    nodes,
		sortCol:  colLatest,
		sortAsc:  false,
		interval: interval,
		rpcs:     rpcs,
		width:    120,
		inK8s:    inK8s,
		page:     0,
		pageSize: 20,
		dispMode: modeWide,
	}
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.inK8s {
		cmds = append(cmds, discoverK8sCmd())
	}
	if len(m.rpcs) > 0 {
		cmds = append(cmds, fetchData(m.rpcs, m.interval), tick(m.interval))
	}
	return tea.Batch(cmds...)
}

func discoverK8sCmd() tea.Cmd {
	return func() tea.Msg {
		candidates, err := discoverK8sRPCCandidates()
		if err != nil {
			return k8sDiscoveredMsg{err: err}
		}
		if len(candidates) == 0 {
			return k8sDiscoveredMsg{rpcs: nil}
		}
		live := probeRPCCandidates(candidates, 5*time.Second)
		return k8sDiscoveredMsg{rpcs: live}
	}
}

func fetchData(rpcs []string, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		results := make([]NodeInfo, len(rpcs))
		type result struct {
			idx  int
			info NodeInfo
		}
		ch := make(chan result, len(rpcs))
		for i, url := range rpcs {
			go func(idx int, u string) {
				ch <- result{idx: idx, info: queryNode(u, timeout)}
			}(i, url)
		}
		for range rpcs {
			r := <-ch
			results[r.idx] = r.info
		}
		return dataMsg(results)
	}
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.filterMode {
			return m.updateFilterMode(msg)
		}

		// Reset esc-pending on any key other than esc
		if msg.String() != "esc" {
			m.escPending = false
		}

		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			if m.filterStr != "" || m.filterRe != nil {
				m.filterStr = ""
				m.filterRe = nil
				m.page = 0
				m.escPending = true
			} else if m.escPending {
				return m, tea.Quit
			} else {
				m.escPending = true
			}
		case "left", "h":
			if m.dispMode == modeSimple {
				m.prevSimpleCol()
			} else if m.sortCol > 0 {
				m.sortCol--
			}
		case "right", "l":
			if m.dispMode == modeSimple {
				m.nextSimpleCol()
			} else if m.sortCol < len(columns)-1 {
				m.sortCol++
			}
		case "a":
			m.sortAsc = true
		case "d":
			m.sortAsc = false
		case " ", "enter":
			m.sortAsc = !m.sortAsc
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			if m.dispMode == modeSimple {
				if idx < len(simpleColMap) {
					col := simpleColMap[idx]
					if m.sortCol == col {
						m.sortAsc = !m.sortAsc
					} else {
						m.sortCol = col
					}
				}
			} else {
				if idx < len(columns) {
					if m.sortCol == idx {
						m.sortAsc = !m.sortAsc
					} else {
						m.sortCol = idx
					}
				}
			}
		case "/":
			m.filterMode = true
			return m, nil
		case "pgdown":
			m.nextPage()
		case "pgup":
			m.prevPage()
		case ",":
			if m.dispMode == modeWide {
				m.dispMode = modeSimple
				// Clamp sortCol to simple columns
				if !isSimpleCol(m.sortCol) {
					m.sortCol = colLatest
				}
			} else {
				m.dispMode = modeWide
			}
		}
		m.sortNodes()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case dataMsg:
		m.nodes = []NodeInfo(msg)
		m.lastUpdate = time.Now()
		m.sortNodes()

	case tickMsg:
		if len(m.rpcs) > 0 {
			return m, tea.Batch(fetchData(m.rpcs, m.interval), tick(m.interval))
		}
		return m, tick(m.interval)

	case k8sDiscoveredMsg:
		m.discovering = false
		if msg.err != nil {
			m.discoverErr = msg.err.Error()
			return m, nil
		}
		if len(msg.rpcs) > 0 {
			// Merge with existing rpcs (deduplicate)
			existing := make(map[string]bool, len(m.rpcs))
			for _, r := range m.rpcs {
				existing[r] = true
			}
			for _, r := range msg.rpcs {
				if !existing[r] {
					m.rpcs = append(m.rpcs, r)
					m.nodes = append(m.nodes, NodeInfo{
						URL: r, LatestBlock: "...", LatestHash: "...",
						SafeBlock: "...", FinalBlock: "...", Syncing: "...",
						Version: "...", PeerCount: "...", ChainID: "...",
					})
				}
			}
		}
		if len(m.rpcs) > 0 {
			return m, tea.Batch(fetchData(m.rpcs, m.interval), tick(m.interval))
		}
	}
	return m, nil
}

func (m model) updateFilterMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.filterMode = false
		m.escPending = false
		m.page = 0
		if m.filterStr == "" {
			m.filterRe = nil
		} else {
			re, err := regexp.Compile(m.filterStr)
			if err == nil {
				m.filterRe = re
			}
		}
	case "esc":
		if m.escPending {
			return m, tea.Quit
		}
		m.filterMode = false
		m.filterStr = ""
		m.filterRe = nil
		m.page = 0
		m.escPending = true
	case "backspace", "ctrl+h":
		m.escPending = false
		if len(m.filterStr) > 0 {
			m.filterStr = m.filterStr[:len(m.filterStr)-1]
		}
		m.page = 0
		m.updateLiveFilter()
	default:
		m.escPending = false
		if len(msg.String()) == 1 {
			m.filterStr += msg.String()
			m.page = 0
			m.updateLiveFilter()
		}
	}
	return m, nil
}

// updateLiveFilter compiles the current filter string for live preview.
func (m *model) updateLiveFilter() {
	if m.filterStr == "" {
		m.filterRe = nil
		return
	}
	re, err := regexp.Compile(m.filterStr)
	if err == nil {
		m.filterRe = re
	}
}

func (m *model) sortNodes() {
	sort.SliceStable(m.nodes, func(i, j int) bool {
		a, b := m.nodes[i], m.nodes[j]
		var less bool
		switch m.sortCol {
		case colURL:
			less = a.URL < b.URL
		case colChainID:
			less = numStr(a.ChainID) < numStr(b.ChainID)
		case colLatest:
			less = numStr(a.LatestBlock) < numStr(b.LatestBlock)
		case colHash:
			less = a.LatestHash < b.LatestHash
		case colSafe:
			less = numStr(a.SafeBlock) < numStr(b.SafeBlock)
		case colFinalized:
			less = numStr(a.FinalBlock) < numStr(b.FinalBlock)
		case colSyncing:
			less = a.Syncing < b.Syncing
		case colPeers:
			less = numStr(a.PeerCount) < numStr(b.PeerCount)
		case colVersion:
			less = a.Version < b.Version
		case colUpdated:
			less = a.UpdatedAt.Before(b.UpdatedAt)
		default:
			less = a.URL < b.URL
		}
		if m.sortAsc {
			return less
		}
		return !less
	})
}

// visibleNodes returns the filtered nodes for the current page.
func (m *model) visibleNodes() []NodeInfo {
	all := m.filteredNodes()
	total := len(all)
	if total == 0 {
		return nil
	}
	start := m.page * m.pageSize
	if start >= total {
		start = 0
		m.page = 0
	}
	end := start + m.pageSize
	if end > total {
		end = total
	}
	return all[start:end]
}

// filteredNodes returns all nodes after applying the regex filter (no paging).
func (m *model) filteredNodes() []NodeInfo {
	if m.filterRe == nil {
		return m.nodes
	}
	var out []NodeInfo
	for _, n := range m.nodes {
		if m.filterRe.MatchString(n.URL) {
			out = append(out, n)
		}
	}
	return out
}

func (m *model) totalPages() int {
	total := len(m.filteredNodes())
	if total == 0 {
		return 1
	}
	pages := total / m.pageSize
	if total%m.pageSize != 0 {
		pages++
	}
	return pages
}

func (m *model) nextPage() {
	if m.page < m.totalPages()-1 {
		m.page++
	}
}

func (m *model) prevPage() {
	if m.page > 0 {
		m.page--
	}
}

func isSimpleCol(col int) bool {
	for _, c := range simpleColMap {
		if c == col {
			return true
		}
	}
	return false
}

func (m *model) simpleColIndex() int {
	for i, c := range simpleColMap {
		if c == m.sortCol {
			return i
		}
	}
	return 0
}

func (m *model) nextSimpleCol() {
	idx := m.simpleColIndex()
	if idx < len(simpleColMap)-1 {
		m.sortCol = simpleColMap[idx+1]
	}
}

func (m *model) prevSimpleCol() {
	idx := m.simpleColIndex()
	if idx > 0 {
		m.sortCol = simpleColMap[idx-1]
	}
}

// simpleColumns returns the 3-column layout for simple mode with a
// dynamically sized URL column wide enough for the longest URL.
func (m *model) simpleColumns() []column {
	urlWidth := 20
	for _, n := range m.nodes {
		if len(n.URL) > urlWidth {
			urlWidth = len(n.URL)
		}
	}
	if urlWidth > 100 {
		urlWidth = 100
	}
	return []column{
		{name: "URL", width: urlWidth},
		{name: "Latest", width: 14},
		{name: "Hash", width: 13},
	}
}

func numStr(s string) int64 {
	if s == "" || s == "N/A" || s == "..." {
		return -1
	}
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func (m model) View() string {
	var sb strings.Builder

	// ── Title bar ──────────────────────────────────────────────────────────
	title := titleStyle.Render("eth-watch")
	if m.inK8s {
		title += " " + k8sBadgeStyle.Render("⎈ K8S")
	}
	modeName := "wide"
	if m.dispMode == modeSimple {
		modeName = "simple"
	}
	title += helpStyle.Render(fmt.Sprintf("  ←/→ col  space asc/desc  a/d  1-9 col  pgup/pgdn page  / filter  , mode(%s)  esc×2/q quit", modeName))
	sb.WriteString(title + "\n")

	// ── Status line ────────────────────────────────────────────────────────
	sortDir := "↓"
	if m.sortAsc {
		sortDir = "↑"
	}
	status := ""
	if m.discovering {
		status = "  " + syncingStyle.Render("● discovering K8s services…")
	} else if m.discoverErr != "" {
		status = "  " + errorStyle.Render("K8s discovery error: "+m.discoverErr)
	}
	filterInfo := ""
	if m.filterRe != nil {
		total := len(m.nodes)
		shown := len(m.filteredNodes())
		filterInfo = fmt.Sprintf("  Filter: /%s/ (%d/%d)", m.filterStr, shown, total)
	}
	totalPages := m.totalPages()
	pageInfo := ""
	if totalPages > 1 {
		pageInfo = fmt.Sprintf("  Page %d/%d  [/] prev/next", m.page+1, totalPages)
	}
	if !m.lastUpdate.IsZero() {
		sb.WriteString(helpStyle.Render(fmt.Sprintf("Last update: %s  Interval: %s  Sort: %s %s",
			m.lastUpdate.Format("15:04:05"),
			m.interval.String(),
			columns[m.sortCol].name,
			sortDir,
		)) + filterActiveStyle.Render(filterInfo) + helpStyle.Render(pageInfo) + status + "\n")
	} else {
		sb.WriteString(status + "\n")
	}
	if m.escPending {
		sb.WriteString(syncingStyle.Render("Press esc again to quit") + "\n")
	}
	sb.WriteString("\n")

	// ── Header row ─────────────────────────────────────────────────────────
	var activeCols []column
	var colIDs []int // global column IDs for active columns
	if m.dispMode == modeSimple {
		activeCols = m.simpleColumns()
		colIDs = simpleColMap
	} else {
		activeCols = columns
		colIDs = make([]int, len(columns))
		for i := range columns {
			colIDs[i] = i
		}
	}

	var headerCells []string
	for i, col := range activeCols {
		globalID := colIDs[i]
		label := col.name
		if globalID == m.sortCol {
			arrow := " ↓"
			if m.sortAsc {
				arrow = " ↑"
			}
			label = label + arrow
		}
		if globalID == m.sortCol {
			headerCells = append(headerCells, selectedColStyle.Width(col.width).Render(label))
		} else {
			headerCells = append(headerCells, headerStyle.Width(col.width).Render(label))
		}
	}
	sb.WriteString(strings.Join(headerCells, " ") + "\n")

	tw := 0
	for i, col := range activeCols {
		tw += col.width
		if i < len(activeCols)-1 {
			tw++
		}
	}
	sb.WriteString(helpStyle.Render(strings.Repeat("─", tw)) + "\n")

	// ── Data rows ──────────────────────────────────────────────────────────
	visible := m.visibleNodes()
	for _, node := range visible {
		var cells []string
		if m.dispMode == modeSimple {
			scols := m.simpleColumns()
			cells = []string{
				renderCell(truncate(node.URL, scols[0].width), scols[0].width, false, node.Error != ""),
				renderCell(node.LatestBlock, scols[1].width, false, node.Error != ""),
				renderCell(node.LatestHash, scols[2].width, false, node.Error != ""),
			}
		} else {
			cells = []string{
				renderCell(truncate(node.URL, columns[colURL].width), columns[colURL].width, false, node.Error != ""),
				renderCell(node.ChainID, columns[colChainID].width, false, node.Error != ""),
				renderCell(node.LatestBlock, columns[colLatest].width, false, node.Error != ""),
				renderCell(node.LatestHash, columns[colHash].width, false, node.Error != ""),
				renderBlockWithDiff(node.SafeBlock, node.LatestBlock, columns[colSafe].width),
				renderBlockWithDiff(node.FinalBlock, node.LatestBlock, columns[colFinalized].width),
				renderSyncing(node.Syncing, columns[colSyncing].width),
				renderCell(node.PeerCount, columns[colPeers].width, false, node.Error != ""),
				renderCell(truncate(node.Version, columns[colVersion].width), columns[colVersion].width, false, false),
				renderCell(formatUpdated(node.UpdatedAt), columns[colUpdated].width, false, false),
			}
		}
		row := strings.Join(cells, " ")
		if node.Error != "" {
			sb.WriteString(errorStyle.Render(row) + "\n")
		} else {
			sb.WriteString(row + "\n")
		}
	}

	// ── Filter input bar ───────────────────────────────────────────────────
	if m.filterMode {
		cursor := "█"
		re, err := regexp.Compile(m.filterStr)
		var hint string
		if m.filterStr != "" && err != nil {
			hint = errorStyle.Render(" (invalid regex)")
		} else if re != nil {
			total := len(m.nodes)
			shown := len(m.filteredNodes())
			hint = helpStyle.Render(fmt.Sprintf(" (%d/%d)", shown, total))
		}
		sb.WriteString("\n" + filterLabelStyle.Render("Filter /") +
			filterActiveStyle.Render(m.filterStr) +
			filterActiveStyle.Render(cursor) +
			filterLabelStyle.Render("/") +
			hint +
			helpStyle.Render("  enter=confirm  esc=clear") + "\n")
	}

	return sb.String()
}

func totalWidth() int {
	total := 0
	for i, col := range columns {
		total += col.width
		if i < len(columns)-1 {
			total++
		}
	}
	return total
}

func renderBlockWithDiff(val, latest string, width int) string {
	latestN := numStr(latest)
	valN := numStr(val)
	if latestN > 0 && valN >= 0 && latestN > valN {
		diff := latestN - valN
		diffStr := fmt.Sprintf("(-%d)", diff)
		combinedLen := lipgloss.Width(val) + lipgloss.Width(diffStr)
		if combinedLen <= width {
			trailing := strings.Repeat(" ", width-combinedLen)
			return normalStyle.Render(val) + diffStyle.Render(diffStr) + trailing
		}
		available := width - lipgloss.Width(diffStr)
		if available < 1 {
			return normalStyle.Width(width).Render(val)
		}
		return normalStyle.Width(available).Render(val) + diffStyle.Render(diffStr)
	}
	return normalStyle.Width(width).Render(val)
}

func renderCell(val string, width int, bold bool, isError bool) string {
	if isError {
		return errorStyle.Width(width).Render(val)
	}
	return normalStyle.Width(width).Render(val)
}

func renderSyncing(val string, width int) string {
	switch val {
	case "synced":
		return syncedStyle.Width(width).Render(val)
	case "syncing":
		return syncingStyle.Width(width).Render(val)
	default:
		return normalStyle.Width(width).Render(val)
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func formatUpdated(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	ago := time.Since(t)
	if ago < time.Second {
		return "just now"
	}
	if ago < time.Minute {
		return fmt.Sprintf("%ds ago", int(ago.Seconds()))
	}
	return fmt.Sprintf("%dm ago", int(ago.Minutes()))
}
