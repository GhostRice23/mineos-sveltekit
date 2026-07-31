package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/domain/ports"
	"github.com/freemancraft/mineos-sveltekit/tools/mineos-cli/internal/infrastructure/api"
)

// modelWithServer builds a model that looks connected, with one server selected.
func modelWithServer(t *testing.T, name string) TuiModel {
	t.Helper()
	return TuiModel{
		Ctx:         context.Background(),
		Client:      api.NewClient("http://127.0.0.1:1", "test-key"),
		ConfigReady: true,
		CurrentView: ViewServers,
		Servers:     []ports.Server{{Name: name}},
		Selected:    0,
	}
}

func TestServerActionRunsInProcess(t *testing.T) {
	m := modelWithServer(t, "lobby")
	m.ServerActions = true
	m.ActionIndex = indexOfAction(t, ServerActionStart)

	updated, cmd := m.navSelect()
	next := updated.(TuiModel)

	if cmd == nil {
		t.Fatal("expected a command for the server action")
	}
	// The old path switched to the output pane to show subprocess stdout.
	// In-process execution reports through the footer instead.
	if next.CurrentView == ViewOutput {
		t.Error("a server action should no longer hijack the view")
	}

	// The command talks to the API (which is not listening here), so it
	// resolves to a result message rather than spawning anything.
	msg := waitMsg(t, cmd)
	if _, ok := msg.(ActionResultMsg); !ok {
		t.Fatalf("expected ActionResultMsg, got %T", msg)
	}
}

func TestServerActionReportsAMissingSelection(t *testing.T) {
	m := TuiModel{Ctx: context.Background(), ConfigReady: true}

	msg := waitMsg(t, m.ServerActionCmd("", ServerActionStart, "Start Server")).(ActionResultMsg)

	if msg.Err == nil {
		t.Fatal("expected an error when no server is selected")
	}
}

func TestServerActionReportsADisconnectedApi(t *testing.T) {
	m := TuiModel{Ctx: context.Background()} // ConfigReady false, Client nil

	msg := waitMsg(t, m.ServerActionCmd("lobby", ServerActionStart, "Start Server")).(ActionResultMsg)

	if msg.Err == nil {
		t.Fatal("expected an error when the API is not connected")
	}
	if !strings.Contains(msg.Err.Error(), "API") {
		t.Errorf("err = %v, want it to mention the API", msg.Err)
	}
}

func TestDestructiveServerActionIsConfirmedAsATypedAction(t *testing.T) {
	m := modelWithServer(t, "lobby")
	m.ServerActions = true
	m.ActionIndex = indexOfAction(t, ServerActionKill)

	updated, _ := m.navSelect()
	next := updated.(TuiModel)

	if next.Mode != ModeConfirm {
		t.Fatalf("Mode = %v, want ModeConfirm", next.Mode)
	}
	if next.ConfirmAction == nil {
		t.Fatal("no action stored for confirmation")
	}
	// Carrying the target explicitly means the confirmation does not have to
	// re-derive it from argv.
	if next.ConfirmAction.Kind != MenuKindServerAction {
		t.Errorf("Kind = %v, want MenuKindServerAction", next.ConfirmAction.Kind)
	}
	if next.ConfirmAction.Server != "lobby" || next.ConfirmAction.ServerAct != ServerActionKill {
		t.Errorf("confirmation targets %q/%q", next.ConfirmAction.Server, next.ConfirmAction.ServerAct)
	}
}

func TestConfirmingAServerActionRunsItInProcess(t *testing.T) {
	m := modelWithServer(t, "lobby")
	m.Mode = ModeConfirm
	m.ConfirmAction = &MenuItem{
		Label:       "Kill Server",
		Kind:        MenuKindServerAction,
		Server:      "lobby",
		ServerAct:   ServerActionKill,
		Destructive: true,
	}

	updated, cmd := m.HandleConfirmInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	next := updated.(TuiModel)

	if next.Mode != ModeNormal {
		t.Errorf("Mode = %v, want the dialog dismissed", next.Mode)
	}
	if cmd == nil {
		t.Fatal("expected the confirmed action to run")
	}
	if _, ok := waitMsg(t, cmd).(ActionResultMsg); !ok {
		t.Error("a confirmed server action should run in-process")
	}
}

func TestStackAndSystemActionsStayOnTheSubprocessPath(t *testing.T) {
	// docker compose orchestration and the interactive commands genuinely need
	// a subprocess; only API operations moved in-process. Asserting on the
	// routing kind rather than running the command, which would exec the test
	// binary.
	for _, item := range BuildNavItems() {
		if item.Action == nil {
			continue
		}
		if item.Action.Kind == MenuKindServerAction {
			t.Errorf("nav item %q must not be routed as a server action", item.Action.Label)
		}
		if len(item.Action.Args) == 0 {
			t.Errorf("nav item %q has no argv to run", item.Action.Label)
		}
	}
}

func TestEveryServerActionIsRoutedInProcess(t *testing.T) {
	for _, action := range GetServerActions() {
		// console and back are handled in the TUI itself, never dispatched.
		if action.Action == ServerActionConsole || action.Action == ServerActionBack {
			continue
		}

		m := modelWithServer(t, "lobby")
		m.ServerActions = true
		m.ActionIndex = indexOfAction(t, action.Action)

		updated, cmd := m.navSelect()
		next := updated.(TuiModel)

		if next.Mode == ModeConfirm {
			// Destructive actions confirm first; the stored item must still be
			// typed as a server action.
			if next.ConfirmAction.Kind != MenuKindServerAction {
				t.Errorf("%q confirms as kind %v", action.Label, next.ConfirmAction.Kind)
			}
			continue
		}

		if cmd == nil {
			t.Errorf("%q produced no command", action.Label)
			continue
		}
		if _, ok := waitMsg(t, cmd).(ActionResultMsg); !ok {
			t.Errorf("%q did not run in-process", action.Label)
		}
	}
}

func TestSuccessfulActionRefreshesTheServerList(t *testing.T) {
	m := TuiModel{ConfigReady: true, Client: api.NewClient("http://127.0.0.1:1", "k")}

	_, cmd := m.Update(ActionResultMsg{Message: "Start Server: lobby"})

	if cmd == nil {
		t.Error("a successful action should refresh the table rather than wait for the poll")
	}
}

func TestFailedActionDoesNotRefresh(t *testing.T) {
	m := TuiModel{ConfigReady: true}

	updated, _ := m.Update(ActionResultMsg{Err: errStreamClosed})

	if updated.(TuiModel).ErrMsg == "" {
		t.Error("the failure should be reported")
	}
}

func indexOfAction(t *testing.T, want ServerAction) int {
	t.Helper()
	for i, a := range GetServerActions() {
		if a.Action == want {
			return i
		}
	}
	t.Fatalf("action %q not found", want)
	return -1
}
