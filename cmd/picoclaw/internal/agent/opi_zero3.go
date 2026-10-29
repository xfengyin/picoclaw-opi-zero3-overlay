//go:build opi_zero3

package agent

import (
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/board/opi_zero3"
)

// attachBoardSupport wires Orange Pi Zero3 board features (status LED
// signaling, thermal watchdog) to the CLI agent runtime events when the
// process runs on that board. On other hardware the call is a no-op.
func attachBoardSupport(loop *agent.AgentLoop) {
	if loop == nil {
		return
	}
	opi_zero3.Attach(loop.RuntimeEvents())
}
