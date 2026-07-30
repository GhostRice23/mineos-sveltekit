package tui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// errStreamClosed stands in for the transient failures the log stream reports.
var errStreamClosed = errors.New("log stream closed")

// waitMsg runs a command and returns its message, failing rather than hanging
// if the command never produces one.
func waitMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("command did not produce a message")
		return nil
	}
}

func TestListenLogsBatchesQueuedLines(t *testing.T) {
	logs := make(chan string, 8)
	for _, line := range []string{"a", "b", "c", "d"} {
		logs <- line
	}

	m := TuiModel{LogsChan: logs, LogErrsChan: make(chan error)}
	msg, ok := waitMsg(t, m.ListenLogsCmd()).(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}

	// One message for the whole burst is the point: Bubble Tea re-renders per
	// message, so four messages would be four full redraws.
	want := []string{"a", "b", "c", "d"}
	if len(msg.Lines) != len(want) {
		t.Fatalf("got %d lines %v, want %d", len(msg.Lines), msg.Lines, len(want))
	}
	for i := range want {
		if msg.Lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, msg.Lines[i], want[i])
		}
	}
}

func TestListenLogsCapsBatchSize(t *testing.T) {
	logs := make(chan string, LogBatchMax*2)
	for i := 0; i < LogBatchMax*2; i++ {
		logs <- "line"
	}

	m := TuiModel{LogsChan: logs, LogErrsChan: make(chan error)}
	msg := waitMsg(t, m.ListenLogsCmd()).(LogLinesMsg)

	if len(msg.Lines) != LogBatchMax {
		t.Errorf("batch = %d lines, want the cap of %d", len(msg.Lines), LogBatchMax)
	}
}

func TestListenLogsReportsCleanCloseSeparatelyFromRetry(t *testing.T) {
	logs := make(chan string)
	close(logs)

	m := TuiModel{LogsChan: logs, LogErrsChan: make(chan error)}
	msg := waitMsg(t, m.ListenLogsCmd())

	// A bare LogRetryMsg here is what caused the tight reconnect loop: it goes
	// straight back to StartLogStreamCmd with no delay.
	if _, ok := msg.(LogStreamClosedMsg); !ok {
		t.Fatalf("expected LogStreamClosedMsg, got %T", msg)
	}
}

func TestListenLogsSurfacesStreamErrors(t *testing.T) {
	errs := make(chan error, 1)
	errs <- errStreamClosed

	m := TuiModel{LogsChan: make(chan string), LogErrsChan: errs}
	msg, ok := waitMsg(t, m.ListenLogsCmd()).(LogErrorMsg)
	if !ok {
		t.Fatalf("expected LogErrorMsg, got %T", msg)
	}
	if msg.Err != errStreamClosed {
		t.Errorf("err = %v, want %v", msg.Err, errStreamClosed)
	}
}

func TestListenLogsWithoutChannelsDoesNothing(t *testing.T) {
	m := TuiModel{}
	if cmd := m.ListenLogsCmd(); cmd != nil {
		t.Error("expected nil command when there is no stream")
	}
}

func TestCleanCloseReconnectIsDelayedAndCounted(t *testing.T) {
	m := TuiModel{LogsActive: true}

	updated, cmd := m.handleLogStreamClosed()
	next := updated.(TuiModel)

	if next.LogRetries != 1 {
		t.Errorf("LogRetries = %d, want 1", next.LogRetries)
	}
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}

	// tea.Tick only fires after the delay; a command that returns immediately
	// would be the old tight-loop behaviour.
	done := make(chan tea.Msg, 1)
	start := time.Now()
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if elapsed := time.Since(start); elapsed < LogRetryDelay/2 {
			t.Errorf("reconnected after %v, want a delay of about %v", elapsed, LogRetryDelay)
		}
		if _, ok := msg.(LogRetryMsg); !ok {
			t.Errorf("expected LogRetryMsg, got %T", msg)
		}
	case <-time.After(LogRetryDelay * 3):
		t.Fatal("reconnect never fired")
	}
}

func TestCleanCloseStopsAfterMaxRetries(t *testing.T) {
	m := TuiModel{LogsActive: true, LogRetries: MaxLogRetries}

	updated, cmd := m.handleLogStreamClosed()
	next := updated.(TuiModel)

	if cmd != nil {
		t.Error("expected no further reconnect once the budget is spent")
	}
	if next.ErrMsg == "" {
		t.Error("expected an explanation once reconnecting is given up on")
	}
	if next.LogRetries != MaxLogRetries {
		t.Errorf("LogRetries = %d, want it to stay at %d", next.LogRetries, MaxLogRetries)
	}
}

func TestCleanCloseDoesNotReconnectWhenInactive(t *testing.T) {
	cases := map[string]TuiModel{
		"logs off":           {LogsActive: false},
		"quitting":           {LogsActive: true, Quitting: true},
		"containers stopped": {LogsActive: true, ContainersStopped: true},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			updated, cmd := m.handleLogStreamClosed()
			if cmd != nil {
				t.Error("expected no reconnect")
			}
			if got := updated.(TuiModel).LogRetries; got != 0 {
				t.Errorf("LogRetries = %d, want 0", got)
			}
		})
	}
}

func TestReceivingLinesRestoresTheRetryBudget(t *testing.T) {
	m := TuiModel{LogsActive: true, LogRetries: MaxLogRetries, ErrMsg: logRetriesExhaustedMsg}

	updated, _ := m.Update(LogLinesMsg{Lines: []string{"server started"}})
	next := updated.(TuiModel)

	if next.LogRetries != 0 {
		t.Errorf("LogRetries = %d, want 0 after lines arrived", next.LogRetries)
	}
	if next.ErrMsg != "" {
		t.Errorf("ErrMsg = %q, want it cleared", next.ErrMsg)
	}
	if len(next.Logs) != 1 || next.Logs[0] != "server started" {
		t.Errorf("Logs = %v", next.Logs)
	}
}

// These go through dispatch rather than Update: Update additionally arms the
// notice-expiry timer, so its command is never nil once ErrMsg is set.
func TestErrorRetryIsCappedToo(t *testing.T) {
	m := TuiModel{LogsActive: true, LogRetries: MaxLogRetries}

	updated, cmd := m.dispatch(LogErrorMsg{Err: errStreamClosed})
	if cmd != nil {
		t.Error("expected no reconnect once the budget is spent")
	}
	if got := updated.(TuiModel).ErrMsg; got != logRetriesExhaustedMsg {
		t.Errorf("ErrMsg = %q, want %q", got, logRetriesExhaustedMsg)
	}
}

func TestErrorRetryCountsAttempts(t *testing.T) {
	m := TuiModel{LogsActive: true}

	updated, cmd := m.dispatch(LogErrorMsg{Err: errStreamClosed})
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	if got := updated.(TuiModel).LogRetries; got != 1 {
		t.Errorf("LogRetries = %d, want 1", got)
	}
}

func TestAppendLogsTrimsToTheBufferLimit(t *testing.T) {
	m := &TuiModel{}
	batch := make([]string, MaxLogLines+50)
	for i := range batch {
		batch[i] = "x"
	}
	batch[len(batch)-1] = "last"

	m.AppendLogs(batch)

	if len(m.Logs) != MaxLogLines {
		t.Errorf("buffered %d lines, want %d", len(m.Logs), MaxLogLines)
	}
	if m.Logs[len(m.Logs)-1] != "last" {
		t.Error("the newest line was trimmed instead of the oldest")
	}
}

func TestAppendLogsIgnoresEmptyBatch(t *testing.T) {
	m := &TuiModel{Logs: []string{"a"}}
	m.AppendLogs(nil)
	if len(m.Logs) != 1 {
		t.Errorf("Logs = %v, want unchanged", m.Logs)
	}
}

func TestResetLogStreamClearsBufferAndBudget(t *testing.T) {
	m := &TuiModel{Logs: []string{"a", "b"}, LogRetries: MaxLogRetries}
	m.resetLogStream()

	if m.Logs != nil {
		t.Errorf("Logs = %v, want nil", m.Logs)
	}
	if m.LogRetries != 0 {
		t.Errorf("LogRetries = %d, want 0", m.LogRetries)
	}
}
