package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

func TestStackActionsDeclareTheirContainerEffect(t *testing.T) {
	// Container state used to be inferred from the label text, so renaming a
	// menu entry silently broke it. Every stack action must now say what it does.
	want := map[string]StackEffect{
		"Start Containers":   StackEffectStarts,
		"Stop Containers":    StackEffectStops,
		"Restart Containers": StackEffectStarts,
		"Remove Containers":  StackEffectStops,
	}

	seen := map[string]bool{}
	for _, item := range BuildNavItems() {
		if item.Action == nil {
			continue
		}
		if effect, ok := want[item.Action.Label]; ok {
			seen[item.Action.Label] = true
			if item.Action.Effect != effect {
				t.Errorf("%q has effect %v, want %v", item.Action.Label, item.Action.Effect, effect)
			}
		}
	}
	for label := range want {
		if !seen[label] {
			t.Errorf("nav item %q not found", label)
		}
	}
}

func TestStoppingContainersMarksThemStopped(t *testing.T) {
	m := TuiModel{StreamingRunning: true, ConfigReady: true, ServersLoaded: true}

	updated, _ := m.Update(StreamingFinishedMsg{Label: "anything at all", Effect: StackEffectStops})
	next := updated.(TuiModel)

	if !next.ContainersStopped {
		t.Error("ContainersStopped = false, want true")
	}
	if next.ConfigReady {
		t.Error("ConfigReady should be cleared once the API is gone")
	}
	if next.ServersLoaded {
		t.Error("ServersLoaded should be cleared so the table stops claiming the list is current")
	}
}

func TestStartingContainersClearsTheStoppedFlag(t *testing.T) {
	m := TuiModel{StreamingRunning: true, ContainersStopped: true}

	updated, _ := m.Update(StreamingFinishedMsg{Label: "anything at all", Effect: StackEffectStarts})

	if updated.(TuiModel).ContainersStopped {
		t.Error("ContainersStopped = true, want false")
	}
}

func TestNonStackActionLeavesContainerStateAlone(t *testing.T) {
	// "Update Images" contains neither Stop nor Start, but "Upgrade CLI" would
	// have matched neither either — the point is that only a declared effect
	// changes the flag.
	m := TuiModel{StreamingRunning: true, ContainersStopped: true}

	updated, _ := m.Update(StreamingFinishedMsg{Label: "Update Images", Effect: StackEffectNone})

	if !updated.(TuiModel).ContainersStopped {
		t.Error("an effect-less action must not change container state")
	}
}

func TestAFailedStackActionDoesNotChangeContainerState(t *testing.T) {
	m := TuiModel{StreamingRunning: true}

	updated, _ := m.Update(StreamingFinishedMsg{
		Label:  "Stop Containers",
		Effect: StackEffectStops,
		Err:    errStreamClosed,
	})

	if updated.(TuiModel).ContainersStopped {
		t.Error("a failed stop must not be recorded as containers stopped")
	}
}

func TestServerActionsAreTyped(t *testing.T) {
	actions := GetServerActions()
	if len(actions) == 0 {
		t.Fatal("no server actions")
	}

	// The CLI-verb actions have to survive conversion into argv.
	byAction := map[ServerAction]bool{}
	for _, a := range actions {
		byAction[a.Action] = true
	}
	for _, want := range []ServerAction{ServerActionStart, ServerActionStop, ServerActionRestart, ServerActionKill} {
		if !byAction[want] {
			t.Errorf("missing action %q", want)
		}
		if string(want) == "" {
			t.Errorf("action %q has no CLI verb", want)
		}
	}
	if !byAction[ServerActionConsole] || !byAction[ServerActionBack] {
		t.Error("console/back actions missing")
	}
}

func TestSpinnerRunsDuringStartupAndStops(t *testing.T) {
	m := NewTuiModel(nil, nil, "test")

	if !m.Busy() {
		t.Error("the TUI should be busy before the first load answers")
	}

	m.FirstLoadDone = true
	if m.Busy() {
		t.Error("an idle TUI must not keep the spinner ticking")
	}
}

func TestSpinnerRunsWhileACommandStreams(t *testing.T) {
	m := TuiModel{FirstLoadDone: true}
	if m.Busy() {
		t.Fatal("precondition: not busy")
	}

	m.StreamingRunning = true
	if !m.Busy() {
		t.Error("a streaming command should keep the spinner running")
	}

	m.StreamingRunning = false
	m.InteractiveRunning = true
	if !m.Busy() {
		t.Error("an interactive command should keep the spinner running")
	}
}

func TestSpinnerDoesNotSpinForeverOnAnUnreachableApi(t *testing.T) {
	// An API that never comes up is a steady state the servers table reports in
	// words; animating it would re-render ten times a second indefinitely.
	m := TuiModel{}
	updated, _ := m.Update(ConfigLoadedMsg{Err: errStreamClosed})

	if !updated.(TuiModel).FirstLoadDone {
		t.Error("a failed first load must still end the startup spinner")
	}
}

func TestSpinnerTickStopsWhenIdle(t *testing.T) {
	m := TuiModel{FirstLoadDone: true}

	_, cmd := m.dispatch(spinner.TickMsg{})

	if cmd != nil {
		t.Error("an idle spinner should not schedule another tick")
	}
}

func TestSpinnerTickContinuesWhileBusy(t *testing.T) {
	m := NewTuiModel(nil, nil, "test")

	_, cmd := m.dispatch(spinner.TickMsg{ID: m.Spinner.ID()})

	if cmd == nil {
		t.Error("a busy spinner should schedule the next tick")
	}
}

func TestSpinnerIsRearmedWhenWorkStarts(t *testing.T) {
	m := TuiModel{FirstLoadDone: true}
	if m.Busy() {
		t.Fatal("precondition: idle")
	}

	// Starting a streaming command has to restart the tick loop, since it stops
	// itself while idle.
	_, cmd := m.Update(StreamingStartedMsg{Output: make(chan string), Label: "Start Containers"})
	if cmd == nil {
		t.Error("expected the spinner to be re-armed when a command starts")
	}
}

func TestLoadingPlaceholderShowsTheSpinner(t *testing.T) {
	m := NewTuiModel(nil, nil, "test")

	rendered := strings.Join(m.RenderServersTable(60, 6), "\n")

	if !strings.Contains(rendered, "Loading servers") {
		t.Errorf("loading state missing:\n%s", rendered)
	}
	// The spinner frame sits in front of the text.
	if strings.Contains(rendered, " Loading servers") && !strings.Contains(rendered, m.Spinner.View()) {
		t.Errorf("spinner frame not rendered:\n%s", rendered)
	}
}
