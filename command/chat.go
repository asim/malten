package command



func init() {
	Register(&Command{
		Name:        "chat",
		Description: "Web-enhanced chat - searches the web for current info",
		Usage:       "/chat <question>",
		Handler:     handleChat,
	})
}

func handleChat(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Please ask a question", nil
	}
	
	// Foursquare is POI-only, general chat goes to AI
	return "Use the main chat for questions. /chat is for web-enhanced queries (not yet implemented).", nil
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
