package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time
type dataMsg []NodeInfo

type column struct {
	name  string
	width int
}

var columns = []column{
	{name: "URL", width: 30},
	{name: "ChainID", width: 9},
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

type model struct {
	nodes      []NodeInfo
	sortCol    int
	sortAsc    bool
	interval   time.Duration
	rpcs       []string
	lastUpdate time.Time
	width      int
	height     int
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
)

func newModel(rpcs []string, interval time.Duration) model {
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
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchData(m.rpcs, m.interval),
		tick(m.interval),
	)
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
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "left", "h":
			if m.sortCol > 0 {
				m.sortCol--
			}
		case "right", "l":
			if m.sortCol < len(columns)-1 {
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
			if idx < len(columns) {
				if m.sortCol == idx {
					m.sortAsc = !m.sortAsc
				} else {
					m.sortCol = idx
				}
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
		return m, tea.Batch(
			fetchData(m.rpcs, m.interval),
			tick(m.interval),
		)
	}
	return m, nil
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

	title := titleStyle.Render("eth-watch") + helpStyle.Render("  ←/→ select column  space/enter toggle asc/desc  a=asc d=desc  1-9 quick select  q quit")
	sb.WriteString(title + "\n")

	sortDir := "↓"
	if m.sortAsc {
		sortDir = "↑"
	}
	if !m.lastUpdate.IsZero() {
		sb.WriteString(helpStyle.Render(fmt.Sprintf("Last update: %s  Interval: %s  Sorting by: %s %s",
			m.lastUpdate.Format("15:04:05"),
			m.interval.String(),
			columns[m.sortCol].name,
			sortDir,
		)) + "\n")
	}
	sb.WriteString("\n")

	var headerCells []string
	for i, col := range columns {
		label := col.name
		if i == m.sortCol {
			arrow := " ↓"
			if m.sortAsc {
				arrow = " ↑"
			}
			label = label + arrow
		}
		if i == m.sortCol {
			headerCells = append(headerCells, selectedColStyle.Width(col.width).Render(label))
		} else {
			headerCells = append(headerCells, headerStyle.Width(col.width).Render(label))
		}
	}
	sb.WriteString(strings.Join(headerCells, " ") + "\n")

	divider := strings.Repeat("─", totalWidth())
	sb.WriteString(helpStyle.Render(divider) + "\n")

	for _, node := range m.nodes {
		cells := []string{
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
		row := strings.Join(cells, " ")
		if node.Error != "" {
			sb.WriteString(errorStyle.Render(row) + "\n")
		} else {
			sb.WriteString(row + "\n")
		}
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
		// 截断 val 以留出差值显示空间
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
