package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/api"
)

// ServerAction identifies a per-server action.
//
// A defined string type rather than a bare string: comparisons stay typed and
// the compiler catches a typo, while the CLI-verb actions still convert
// straight into argv.
type ServerAction string

const (
	ServerActionStart   ServerAction = "start"
	ServerActionStop    ServerAction = "stop"
	ServerActionRestart ServerAction = "restart"
	ServerActionKill    ServerAction = "kill"
	// ServerActionConsole and ServerActionBack are UI-only — they are handled
	// in the TUI and never reach the CLI as a verb.
	ServerActionConsole ServerAction = "console"
	ServerActionBack    ServerAction = "back"
)

// ServerActionItem represents an action available for a server
type ServerActionItem struct {
	Label       string
	Action      ServerAction
	Destructive bool
}

// GetServerActions returns the list of actions available for a server
func GetServerActions() []ServerActionItem {
	return []ServerActionItem{
		{Label: "Start Server", Action: ServerActionStart},
		{Label: "Stop Server", Action: ServerActionStop},
		{Label: "Restart Server", Action: ServerActionRestart},
		{Label: "Kill Server", Action: ServerActionKill, Destructive: true},
		{Label: "Send Console Command", Action: ServerActionConsole},
		{Label: "← Back to Server List", Action: ServerActionBack},
	}
}

func (m TuiModel) RenderServersMain(width, height int) []string {
	// If in server actions mode, show actions for selected server
	if m.ServerActions && len(m.Servers) > 0 {
		return m.RenderServerActionsMain(width, height)
	}

	tableHeight := height / 2
	logHeight := height - tableHeight - 1

	tableLines := m.RenderServersTable(width, tableHeight)
	logLines := m.RenderMinecraftLogs(width, logHeight)

	lines := append(tableLines, StyleSubtle.Render(strings.Repeat("─", width)))
	lines = append(lines, logLines...)
	return lines
}

// ServersTableState is why the server table has no rows. "No servers found."
// used to be shown for all three, so a still-loading TUI and an unreachable API
// looked exactly like a working install with nothing on it.
type ServersTableState int

const (
	// ServersStateLoading — the first list request has not come back yet.
	ServersStateLoading ServersTableState = iota
	// ServersStateUnavailable — the API could not be reached.
	ServersStateUnavailable
	// ServersStateEmpty — the API answered, with no servers.
	ServersStateEmpty
)

// ServersTableState reports which of the three empty states applies.
func (m TuiModel) ServersTableState() ServersTableState {
	switch {
	case m.ServersLoaded:
		return ServersStateEmpty
	case m.ConfigReady && !m.Healthy:
		return ServersStateUnavailable
	default:
		return ServersStateLoading
	}
}

// ServersPlaceholder is the line shown in place of the table rows.
func ServersPlaceholder(state ServersTableState) string {
	switch state {
	case ServersStateUnavailable:
		return "Can't reach the MineOS API - retrying..."
	case ServersStateEmpty:
		return "No servers yet. Create one in the web UI."
	default:
		return "Loading servers..."
	}
}

func (m TuiModel) RenderServersTable(width, height int) []string {
	lines := make([]string, 0, height)

	// Table Header
	header := fmt.Sprintf("  %-20s %-10s %-8s %-7s", "SERVER NAME", "STATUS", "PLAYERS", "MEM")
	lines = append(lines, StyleHeader.Render(header))
	lines = append(lines, StyleSubtle.Render(strings.Repeat("─", width)))

	if m.ErrMsg != "" {
		lines = append(lines, TrimToWidth(StyleError.Render(" Error: "+m.ErrMsg), width))
	}

	if len(m.Servers) == 0 {
		state := m.ServersTableState()
		placeholder := " " + ServersPlaceholder(state)
		if state == ServersStateLoading {
			placeholder = " " + m.Spinner.View() + ServersPlaceholder(state)
		}
		lines = append(lines, TrimToWidth(StyleSubtle.Render(placeholder), width))
		return PadLines(lines, height)
	}

	for i, server := range m.Servers {
		prefix := "  "
		nameStyle := StyleHeader // Default
		if i == m.Selected {
			prefix = StyleSelected.Render("▶ ")
			nameStyle = StyleSelected
		}

		// Pad plain text before styling so column widths stay honest.
		name := nameStyle.Render(fmt.Sprintf("%-20s", server.Name))
		status := FormatStatus(fmt.Sprintf("%-10s", server.Status))
		players := fmt.Sprintf("%-8s", formatPlayers(server.PlayersOnline, server.PlayersMax))
		mem := fmt.Sprintf("%-7s", formatMemory(server.MemoryBytes))
		restart := ""
		if server.NeedsRestart {
			restart = " " + StyleError.Render("⟳ restart")
		}

		line := fmt.Sprintf("%s%s %s %s %s%s", prefix, name, status, players, mem, restart)
		lines = append(lines, TrimToWidth(line, width))
	}

	return PadLines(lines, height)
}

// formatPlayers renders "online/max" (or "online", or "—" when unknown).
func formatPlayers(online, max *int) string {
	if online == nil {
		return "—"
	}
	if max == nil {
		return fmt.Sprintf("%d", *online)
	}
	return fmt.Sprintf("%d/%d", *online, *max)
}

// formatMemory renders a byte count as a compact MiB/GiB value ("—" when unknown).
func formatMemory(b *int64) string {
	if b == nil || *b <= 0 {
		return "—"
	}
	mib := float64(*b) / (1024 * 1024)
	if mib >= 1024 {
		return fmt.Sprintf("%.1fG", mib/1024)
	}
	return fmt.Sprintf("%.0fM", mib)
}

// renderMetricsLines renders the live per-server metrics panel (streamed via SSE)
// plus sparklines over the recent history.
func (m TuiModel) renderMetricsLines() []string {
	lines := []string{StyleHeader.Render(" LIVE METRICS ")}
	p := m.PerfSample
	if p == nil && len(m.PerfHistory) == 0 {
		return append(lines, StyleSubtle.Render("  waiting for data…"), "")
	}

	if p != nil {
		tps := "—"
		tpsStyle := StyleRunning
		if p.Tps != nil {
			tps = fmt.Sprintf("%.1f", *p.Tps)
			if *p.Tps < LowTpsThreshold { // matches the server-side alert threshold
				tpsStyle = StyleError
			}
		}
		lines = append(lines, fmt.Sprintf("  TPS: %s   CPU: %.0f%%   RAM: %d/%d MB   Players: %d",
			tpsStyle.Render(tps), p.CpuPercent, p.RamUsedMb, p.RamTotalMb, p.PlayerCount))
	}

	lines = append(lines, m.renderSparklines()...)
	return append(lines, "")
}

// renderSparklines renders one history row per tracked metric.
func (m TuiModel) renderSparklines() []string {
	if len(m.PerfHistory) < 2 {
		// A single point is a dot, not a trend; say so rather than draw one.
		if len(m.PerfHistory) > 0 {
			return []string{StyleSubtle.Render("  history: collecting…")}
		}
		return nil
	}

	tps, cpu, players := perfSeries(m.PerfHistory)

	rows := []string{
		sparkRow("TPS ", tps, 0, 20, 1, ""),
		sparkRow("CPU ", cpu, 0, 100, 0, "%"),
		sparkRow("Plyr", players, 0, math.NaN(), 0, ""),
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row != "" {
			out = append(out, row)
		}
	}
	return out
}

// sparkRow renders one labelled sparkline with its min/avg/max summary.
func sparkRow(label string, values []float64, scaleMin, scaleMax float64, decimals int, suffix string) string {
	stats := Stats(values)
	if stats.Count == 0 {
		return ""
	}
	spark := Sparkline(values, SparklineWidth, scaleMin, scaleMax)
	return fmt.Sprintf("  %s %s  %s",
		StyleSubtle.Render(label),
		spark,
		StyleSubtle.Render(FormatSeriesStats(stats, decimals, suffix)))
}

// perfSeries projects the sample history onto the three plotted series.
// TPS is omitted for samples that never reported one (a stopped server, or a
// server without the Spark plugin), so a gap is not drawn as a zero.
func perfSeries(history []api.PerfSample) (tps, cpu, players []float64) {
	tps = make([]float64, 0, len(history))
	cpu = make([]float64, 0, len(history))
	players = make([]float64, 0, len(history))
	for _, s := range history {
		if s.Tps != nil {
			tps = append(tps, *s.Tps)
		}
		cpu = append(cpu, s.CpuPercent)
		players = append(players, float64(s.PlayerCount))
	}
	return tps, cpu, players
}

// RenderMinecraftLogs renders Minecraft server logs for the selected server
func (m TuiModel) RenderMinecraftLogs(width, height int) []string {
	if height <= 0 {
		return nil
	}
	lines := make([]string, 0, height)

	serverName := m.SelectedServer()
	if serverName == "" {
		lines = append(lines, StyleHeader.Render(" MINECRAFT LOGS "))
		lines = append(lines, TrimToWidth(StyleSubtle.Render("  Select a server to view logs."), width))
		return PadLines(lines, height)
	}

	title := fmt.Sprintf(" LOGS: %s ", serverName)
	lines = append(lines, StyleHeader.Render(title))

	if !m.ConfigReady {
		lines = append(lines, TrimToWidth(StyleSubtle.Render("  API not connected."), width))
		return PadLines(lines, height)
	}

	if len(m.Logs) == 0 {
		lines = append(lines, TrimToWidth(StyleSubtle.Render("  Waiting for logs..."), width))
		return PadLines(lines, height)
	}

	start := 0
	if len(m.Logs) > height-1 {
		start = len(m.Logs) - (height - 1)
	}
	for _, line := range m.Logs[start:] {
		// Sanitize log line to remove ANSI codes that cause rendering issues on Linux
		sanitized := SanitizeLogLine(line)
		// 4 space indent to prevent overlap with nav menu
		lines = append(lines, TrimToWidth("    "+sanitized, width))
	}

	return PadLines(lines, height)
}

func FormatStatus(status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch value {
	case "running":
		return StyleRunning.Render(status)
	case "stopped", "exited":
		return StyleStopped.Render(status)
	default:
		return StyleSubtle.Render(status)
	}
}

// RenderServerActionsMain renders the server actions view
func (m TuiModel) RenderServerActionsMain(width, height int) []string {
	lines := make([]string, 0, height)

	serverName := m.SelectedServer()
	title := fmt.Sprintf(" SERVER: %s ", serverName)
	lines = append(lines, StyleHeader.Render(title))
	lines = append(lines, StyleSubtle.Render(strings.Repeat("─", width)))
	lines = append(lines, "")

	// Show server status
	if m.Selected >= 0 && m.Selected < len(m.Servers) {
		server := m.Servers[m.Selected]
		statusLine := "  Status: " + FormatStatus(server.Status)
		lines = append(lines, statusLine)
		lines = append(lines, "")
	}

	// Live metrics panel (streamed via SSE while this view is open)
	lines = append(lines, m.renderMetricsLines()...)

	// Show actions
	lines = append(lines, StyleHeader.Render(" ACTIONS "))
	lines = append(lines, "")

	actions := GetServerActions()
	for i, action := range actions {
		prefix := "  "
		label := action.Label
		if action.Destructive {
			label = label + " !"
		}
		if i == m.ActionIndex {
			prefix = StyleSelected.Render("▶ ")
			label = StyleSelected.Render(label)
		}
		lines = append(lines, prefix+label)
	}

	lines = append(lines, "")
	lines = append(lines, StyleSubtle.Render("  [Enter] Select  [Esc] Back"))

	// Fill remaining space with logs
	usedHeight := len(lines) + 2 // +2 for separator and some padding
	logHeight := height - usedHeight
	if logHeight > 3 {
		lines = append(lines, "")
		lines = append(lines, StyleSubtle.Render(strings.Repeat("─", width)))
		logLines := m.RenderMinecraftLogs(width, logHeight)
		lines = append(lines, logLines...)
	}

	return PadLines(lines, height)
}
