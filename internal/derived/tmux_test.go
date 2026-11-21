package derived

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

type tmuxFixture struct {
	output string
	err    error
	calls  *int
}

func (f tmuxFixture) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	*f.calls++
	if name != "tmux" || strings.Join(args, " ") != "list-panes -a -F #{pane_id}|#{window_id}|#{pid}:#{start_time}|#{@agent_state}" {
		return nil, errors.New("unexpected command")
	}
	return []byte(f.output), f.err
}

func TestTmuxCheck(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, host, pane, output, state, code string
		err                                   error
		calls                                 int
	}{
		{"working", host, "%11", "%11|@1|10:20|\n", StateCurrent, "live", nil, 1},
		{"waiting", host, "%11", "%11|@1|10:20|waiting\n", StateCurrent, "live", nil, 1},
		{"done", host, "%11", "%11|@1|10:20|done\n", StateStale, "agent_done", nil, 1},
		{"missing", host, "%11", "%12|@1|10:20|\n", StateMissing, "no_live_pane", nil, 1},
		{"empty", host, "%11", "", StateMissing, "no_live_pane", nil, 1},
		{"unknown state", host, "%11", "%11|@1|10:20|invalid\n", StateUnknown, "state_unavailable", nil, 1},
		{"malformed", host, "%11", "%11|@1|10:20|x|secret\n", StateUnknown, "tmux_unavailable", nil, 1},
		{"duplicate", host, "%11", "%11|@1|10:20|\n%11|@1|10:20|\n", StateUnknown, "tmux_unavailable", nil, 1},
		{"shared", host, "%11", "%11|@1|10:20|done\n%12|@1|10:20|done\n", StateUnknown, "shared_window", nil, 1},
		{"failed", host, "%11", "private", StateUnknown, "tmux_unavailable", errors.New("private"), 1},
		{"reused", host, "%11", "%11|@1|11:21|\n", StateMissing, "pane_reused", nil, 1},
		{"bad server", host, "%11", "%11|@1|bad|\n", StateUnknown, "tmux_unavailable", nil, 1},
		{"remote", host + "-other", "%11", "", StateUnknown, "remote_host", nil, 0},
		{"unbound", host, "", "", StateUnknown, "pane_unbound", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			exec := store.Execution{ID: "attempt", Host: tc.host, TmuxPane: tc.pane, TmuxServer: "10:20"}
			got := checkTmux(context.Background(), tmuxFixture{tc.output, tc.err, &calls}, exec)
			expect(t, got, tc.state, tc.code)
			if calls != tc.calls || got.Detail != "" {
				t.Fatalf("calls=%d finding=%+v", calls, got)
			}
		})
	}
}

func TestTmuxOfflineAndTerminalSkipAdapter(t *testing.T) {
	host, _ := os.Hostname()
	calls := 0
	snapshot := Snapshot{TaskID: "task", Task: store.Manifest{Lifecycle: "created"}, Executions: []store.Execution{{ID: "attempt", Host: host, TmuxPane: "%11"}}}
	runner := tmuxFixture{output: "%11|@1|10:20|done\n", calls: &calls}
	expect(t, Check(snapshot, runner, Options{Offline: true}).Executions[0], StateUnknown, "adapter_unavailable")
	snapshot.Task.Lifecycle = "finished"
	expect(t, Check(snapshot, runner, Options{}).Executions[0], StateStale, "task_terminal")
	if calls != 0 {
		t.Fatalf("offline or terminal check contacted tmux %d times", calls)
	}
}

func TestTmuxLegacyBindingWithoutServerIdentity(t *testing.T) {
	host, _ := os.Hostname()
	calls := 0
	exec := store.Execution{ID: "attempt", Host: host, TmuxPane: "%11"}
	expect(t, checkTmux(context.Background(), tmuxFixture{"%11|@1|99:98|\n", nil, &calls}, exec), StateCurrent, "live")
}

func TestParseTmuxServer(t *testing.T) {
	for in, want := range map[string]string{"12:34\n": "12:34", "": "", "12": "", "a:1": "", "1:2:3": "", " 5:6 ": "5:6"} {
		if got := ParseTmuxServer(in); got != want {
			t.Fatalf("%q: %q", in, got)
		}
	}
}
