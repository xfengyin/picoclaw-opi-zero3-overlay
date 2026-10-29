//go:build !opi_zero3

package agent

import "github.com/sipeed/picoclaw/pkg/agent"

// attachBoardSupport is a no-op when the opi_zero3 build tag is not set.
func attachBoardSupport(loop *agent.AgentLoop) {}
