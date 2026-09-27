package derived

import (
	"context"
	"os"
	"strings"

	"github.com/akofink/akagent-cli/internal/store"
)

func checkTmux(ctx context.Context, runner Runner, execution store.Execution) Finding {
	surface := "execution:" + execution.ID
	if execution.Host == "" || execution.TmuxPane == "" {
		return finding(surface, StateUnknown, "pane_unbound", "Bind a pane on a new execution or verify the owning agent externally")
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return finding(surface, StateUnknown, "host_unavailable", "Verify the owning host externally")
	}
	if execution.Host != host {
		return finding(surface, StateUnknown, "remote_host", "Check the execution on its owning host")
	}
	output, err := runner.Run(ctx, "tmux", "list-panes", "-a", "-F", "#{pane_id}|#{window_id}|#{@agent_state}")
	if err != nil || ctx.Err() != nil {
		return finding(surface, StateUnknown, "tmux_unavailable", "Check tmux on the owning host")
	}
	seen := false
	state, window := "", ""
	windows := map[string]int{}
	for _, line := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		if line == "" && len(output) == 0 {
			break
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 || !paneID(parts[0]) || len(parts[1]) < 2 || parts[1][0] != '@' || strings.ContainsAny(parts[1]+parts[2], "\r\n\x00") {
			return finding(surface, StateUnknown, "tmux_unavailable", "Check tmux on the owning host")
		}
		windows[parts[1]]++
		if parts[0] == execution.TmuxPane {
			if seen {
				return finding(surface, StateUnknown, "tmux_unavailable", "Check tmux on the owning host")
			}
			seen, window, state = true, parts[1], parts[2]
		}
	}
	if !seen {
		return finding(surface, StateMissing, "no_live_pane", "Verify the attempt independently before closing its record")
	}
	if windows[window] != 1 {
		return finding(surface, StateUnknown, "shared_window", "Verify the agent state on its pane independently")
	}
	switch state {
	case "", "working", "idle", "blocked", "waiting":
		return finding(surface, StateCurrent, "live", "No execution action needed")
	case "done":
		return finding(surface, StateStale, "agent_done", "Verify the delivery contract and close the execution explicitly")
	default:
		return finding(surface, StateUnknown, "state_unavailable", "Verify the agent state on the owning pane")
	}
}

func paneID(value string) bool {
	if len(value) < 2 || value[0] != '%' {
		return false
	}
	for _, c := range value[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
