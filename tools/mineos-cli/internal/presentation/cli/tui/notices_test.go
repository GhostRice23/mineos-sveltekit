package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStatusNoticeExpires(t *testing.T) {
	m := TuiModel{}

	updated, cmd := m.Update(ActionResultMsg{Message: "server started"})
	next := updated.(TuiModel)

	if next.StatusMsg != "server started" {
		t.Fatalf("StatusMsg = %q", next.StatusMsg)
	}
	if next.StatusSeq != 1 {
		t.Errorf("StatusSeq = %d, want 1", next.StatusSeq)
	}
	if cmd == nil {
		t.Fatal("expected an expiry to be armed")
	}

	// The expiry the wrapper armed clears the notice it was armed for.
	expired, _ := next.Update(ClearStatusMsg{Seq: next.StatusSeq})
	if got := expired.(TuiModel).StatusMsg; got != "" {
		t.Errorf("StatusMsg = %q, want it cleared", got)
	}
}

func TestStaleExpiryDoesNotClearANewerNotice(t *testing.T) {
	m := TuiModel{}

	first, _ := m.Update(ActionResultMsg{Message: "first"})
	second, _ := first.(TuiModel).Update(ActionResultMsg{Message: "second"})
	next := second.(TuiModel)

	// The timer armed for "first" fires after "second" replaced it.
	after, _ := next.Update(ClearStatusMsg{Seq: 1})
	if got := after.(TuiModel).StatusMsg; got != "second" {
		t.Errorf("StatusMsg = %q, want the newer notice to survive", got)
	}
}

func TestErrorNoticeExpiresIndependently(t *testing.T) {
	m := TuiModel{}

	updated, _ := m.Update(ActionResultMsg{Err: errStreamClosed})
	next := updated.(TuiModel)

	if next.ErrMsg == "" {
		t.Fatal("expected ErrMsg to be set")
	}
	// A status expiry must not clear an error.
	afterStatus, _ := next.Update(ClearStatusMsg{Seq: next.ErrSeq})
	if afterStatus.(TuiModel).ErrMsg == "" {
		t.Error("a status expiry cleared the error notice")
	}

	afterErr, _ := next.Update(ClearErrorMsg{Seq: next.ErrSeq})
	if got := afterErr.(TuiModel).ErrMsg; got != "" {
		t.Errorf("ErrMsg = %q, want it cleared", got)
	}
}

func TestNoExpiryArmedWhenNoticeUnchanged(t *testing.T) {
	m := TuiModel{}

	_, cmd := m.Update(HealthCheckedMsg{Healthy: true})
	if cmd != nil {
		t.Error("expected no expiry for a message that sets no notice")
	}
}

func TestServersTableStates(t *testing.T) {
	cases := []struct {
		name  string
		model TuiModel
		want  ServersTableState
	}{
		{"before the first response", TuiModel{}, ServersStateLoading},
		{"config loaded but API down", TuiModel{ConfigReady: true}, ServersStateUnavailable},
		{"config loaded and API up", TuiModel{ConfigReady: true, Healthy: true}, ServersStateLoading},
		{"answered with no servers", TuiModel{ConfigReady: true, Healthy: true, ServersLoaded: true}, ServersStateEmpty},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.model.ServersTableState(); got != tc.want {
				t.Errorf("state = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestServersPlaceholderDistinguishesTheThreeStates(t *testing.T) {
	seen := map[string]bool{}
	for _, state := range []ServersTableState{ServersStateLoading, ServersStateUnavailable, ServersStateEmpty} {
		text := ServersPlaceholder(state)
		if text == "" {
			t.Fatalf("no placeholder for state %v", state)
		}
		if seen[text] {
			t.Errorf("state %v reuses the message %q", state, text)
		}
		seen[text] = true
	}
}

func TestSuccessfulListMarksServersLoaded(t *testing.T) {
	m := TuiModel{}

	updated, _ := m.Update(ServersLoadedMsg{})
	if !updated.(TuiModel).ServersLoaded {
		t.Error("an empty-but-successful response should still count as loaded")
	}
}

func TestFailedListDoesNotMarkServersLoaded(t *testing.T) {
	m := TuiModel{}

	updated, _ := m.Update(ServersLoadedMsg{Err: errStreamClosed})
	if updated.(TuiModel).ServersLoaded {
		t.Error("a failed list must not be reported as an empty install")
	}
}

func TestHelpOverlayTogglesWithQuestionMark(t *testing.T) {
	m := TuiModel{Width: 80, Height: 24}

	opened, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if got := opened.(TuiModel).Mode; got != ModeHelp {
		t.Fatalf("Mode = %v, want ModeHelp", got)
	}

	closed, _ := opened.(TuiModel).HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if got := closed.(TuiModel).Mode; got != ModeNormal {
		t.Errorf("Mode = %v, want ModeNormal", got)
	}
}

func TestHelpOverlayClosesOnEsc(t *testing.T) {
	m := TuiModel{Mode: ModeHelp}

	closed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if got := closed.(TuiModel).Mode; got != ModeNormal {
		t.Errorf("Mode = %v, want ModeNormal", got)
	}
}

func TestHelpOverlaySwallowsOtherKeys(t *testing.T) {
	// 'q' would quit and 'j' would move the selection if either reached the
	// normal handler.
	for _, key := range []string{"j", "q", "/"} {
		m := TuiModel{Mode: ModeHelp, CurrentView: ViewServers}

		next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd != nil {
			t.Errorf("%q was not swallowed by the help overlay", key)
		}
		if got := next.(TuiModel).Mode; got != ModeHelp {
			t.Errorf("%q left Mode = %v, want the overlay to stay open", key, got)
		}
	}
}

func TestHelpOverlayStillAllowsCtrlC(t *testing.T) {
	m := TuiModel{Mode: ModeHelp}

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C must quit from the help overlay")
	}
	if !next.(TuiModel).Quitting {
		t.Error("Quitting = false, want true")
	}
}

func TestHelpOverlayDocumentsEveryBoundKey(t *testing.T) {
	rendered := strings.Join(TuiModel{Width: 80}.RenderHelpOverlay(80, 40), "\n")

	// Keys handled in input.go that a user has no other way to discover.
	for _, key := range []string{"j", "k", "h", "l", "/", "n", "N", "g", "G", "p", "r", "?", "q"} {
		if !strings.Contains(rendered, key) {
			t.Errorf("help overlay does not mention %q", key)
		}
	}
}
