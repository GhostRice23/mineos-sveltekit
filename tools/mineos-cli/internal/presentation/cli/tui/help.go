package tui

import (
	"fmt"
	"strings"
)

// HelpEntry is one keybinding in the help overlay.
type HelpEntry struct {
	Keys string
	What string
}

// HelpSection groups related keybindings.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

// HelpSections is the keybinding reference shown by '?'.
//
// These bindings previously existed only in input.go's switch statement, so the
// vim-style navigation was undiscoverable unless you read the source.
func HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{"j / down", "Move down (scroll down in logs)"},
				{"k / up", "Move up (scroll up in logs)"},
				{"h / left", "Previous log source"},
				{"l / right", "Next log source"},
				{"enter", "Select"},
				{"esc", "Back"},
				{"pgup / pgdn", "Page through logs"},
			},
		},
		{
			Title: "Logs",
			Entries: []HelpEntry{
				{"/", "Search logs"},
				{"n / N", "Next / previous match"},
				{"g / G", "Jump to top / bottom"},
				{"r", "Reconnect the log stream"},
			},
		},
		{
			Title: "Other",
			Entries: []HelpEntry{
				{"p", "Toggle pre-release updates (Settings)"},
				{"?", "Show or hide this help"},
				{"q", "Quit"},
			},
		},
	}
}

// RenderHelpOverlay renders the keybinding reference.
func (m TuiModel) RenderHelpOverlay(width, height int) []string {
	lines := make([]string, 0, height)
	lines = append(lines, StyleHeader.Render(" Keyboard shortcuts"))
	lines = append(lines, StyleSubtle.Render(strings.Repeat("─", max(0, width))))

	for _, section := range HelpSections() {
		lines = append(lines, "")
		lines = append(lines, TrimToWidth(StyleStatus.Render(" "+section.Title), width))
		for _, entry := range section.Entries {
			row := fmt.Sprintf("   %-14s %s", entry.Keys, entry.What)
			lines = append(lines, TrimToWidth(row, width))
		}
	}

	lines = append(lines, "")
	lines = append(lines, TrimToWidth(StyleSubtle.Render(" Press ? or esc to close"), width))

	return PadLines(lines, height)
}
