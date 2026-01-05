package command

import (
	"fmt"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "agentic",
		Description: "Toggle agentic mode for agents (LLM-based processing)",
		Usage:       "/agentic [on|off|status]",
		Handler:     handleAgentic,
	})
}

func handleAgentic(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		// Status
		status := "off"
		if spatial.AgenticMode {
			status = "on"
		}
		return fmt.Sprintf("🤖 Agentic mode: %s\n\nUse `/agentic on` or `/agentic off` to toggle.", status), nil
	}

	switch strings.ToLower(args[0]) {
	case "on", "enable", "true", "1":
		spatial.EnableAgenticMode()
		return "🤖 Agentic mode enabled. Agents will now use LLM processing.", nil
	case "off", "disable", "false", "0":
		spatial.DisableAgenticMode()
		return "🤖 Agentic mode disabled. Agents using simple polling.", nil
	case "status":
		status := "off"
		if spatial.AgenticMode {
			status = "on"
		}
		return fmt.Sprintf("🤖 Agentic mode: %s", status), nil
	default:
		return "", fmt.Errorf("unknown option: %s. Use on/off/status", args[0])
	}
}
