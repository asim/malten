package server

import (
	"errors"
	"regexp"
	"strings"
)

// Match a whole handle. A slash command is only valid at the start.
// A dot can end a sentence, but can also be part of a domain name.
var maltenMention = regexp.MustCompile(`(?i)(?:^|[\s\p{Z}\p{Ps}\p{Pi},"'])(@malten|^/malten)(?:$|[\s\p{Z}\p{Pe}\p{Pf},:;!?"'…،؛؟]|\.(?:$|[\s\p{Z}]))`)

// MentionPrompt returns the question for Mu and whether the post addresses Malten.
func MentionPrompt(text string) (string, bool) {
	text = strings.TrimSpace(text)
	match := maltenMention.FindStringSubmatchIndex(text)
	if match == nil {
		return text, false
	}
	before, after := text[:match[2]], text[match[3]:]
	if before == "" {
		return strings.TrimSpace(strings.TrimLeft(after, ",:،")), true
	}
	before = strings.TrimRight(before, " \t")
	if strings.Trim(after, ".!?…؟") == "" {
		before = strings.TrimRight(before, ",،")
	}
	return strings.TrimSpace(before + after), true
}

var (
	errNoQuestion       = errors.New("Write a question for Malten.")
	errAgentUnavailable = errors.New("Malten is unavailable. Please try again.")
)

// postMessage publishes the original text and sends an addressed question to Mu once.
func (s *Server) postMessage(message *Message) error {
	prompt, mentioned := MentionPrompt(message.Text)
	if !mentioned {
		s.Publish(message)
		return nil
	}
	if prompt == "" {
		return errNoQuestion
	}
	if !s.queueCommand(message, prompt) {
		return errAgentUnavailable
	}
	return nil
}
