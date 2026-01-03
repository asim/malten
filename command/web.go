package command

import (
	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "web",
		Description: "Check or toggle web search status",
		Usage:       "/web [on|off|status]",
		Handler:     handleWeb,
	})
}

func handleWeb(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "🔍 Web search: " + spatial.WebSearchStatus(), nil
	}
	
	switch args[0] {
	case "on":
		spatial.EnableWebSearch()
		return "🔍 Web search enabled", nil
	case "off":
		spatial.DisableWebSearch()
		return "🔍 Web search disabled", nil
	case "status":
		return "🔍 Web search: " + spatial.WebSearchStatus(), nil
	default:
		return "Usage: /websearch [on|off|status]", nil
	}
}
