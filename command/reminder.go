package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const reminderCacheTTL = 5 * time.Minute

func init() {
	Register(&Command{
		Name:        "reminder",
		Description: "Get daily Islamic reminder",
		Usage:       "/reminder",
		Handler:     reminderHandler,
	})
	Register(&Command{
		Name:        "quran",
		Description: "Search the Quran",
		Usage:       "/quran <query>",
		Handler:     quranSearchHandler,
	})
}

type ReminderResponse struct {
	Verse   string `json:"verse"`
	Hadith  string `json:"hadith"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type SearchResponse struct {
	Answer     string `json:"answer"`
	References []struct {
		Text     string `json:"text"`
		Metadata struct {
			Chapter string `json:"chapter"`
			Verse   string `json:"verse"`
			Name    string `json:"name"`
			Source  string `json:"source"`
		} `json:"metadata"`
	} `json:"references"`
}

func reminderHandler(args []string) (string, error) {
	// Check cache
	if cached, ok := GlobalCache.Get("reminder:latest"); ok {
		return cached, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://reminder.dev/api/latest")
	if err != nil {
		return "Error fetching reminder", err
	}
	defer resp.Body.Close()

	var data ReminderResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "Error parsing response", err
	}

	// Format output
	var parts []string
	if data.Verse != "" {
		parts = append(parts, data.Verse)
	}
	if data.Name != "" {
		// Just the name title, not the full description
		lines := strings.Split(data.Name, "\n")
		if len(lines) > 0 {
			parts = append(parts, lines[0])
		}
	}

	result := strings.Join(parts, "\n\n")
	GlobalCache.Set("reminder:latest", result, reminderCacheTTL)

	return result, nil
}

func quranSearchHandler(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /quran <query> (e.g. /quran patience)", nil
	}

	query := strings.Join(args, " ")
	
	// Check cache
	cacheKey := "quran:" + query
	if cached, ok := GlobalCache.Get(cacheKey); ok {
		return cached, nil
	}

	// Call reminder.dev search API
	reqBody, _ := json.Marshal(map[string]string{"q": query})
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post("https://reminder.dev/api/search", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "Error searching", err
	}
	defer resp.Body.Close()

	var data SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "Error parsing response", err
	}

	if data.Answer == "" && len(data.References) == 0 {
		return "No results found", nil
	}

	// Format: answer + top 2 references
	var result strings.Builder
	
	if data.Answer != "" {
		// Strip HTML tags from answer
		answer := stripHTML(data.Answer)
		result.WriteString(answer)
	}

	if len(data.References) > 0 {
		result.WriteString("\n")
		max := 2
		if len(data.References) < max {
			max = len(data.References)
		}
		for i := 0; i < max; i++ {
			ref := data.References[i]
			if ref.Metadata.Source == "quran" {
				result.WriteString(fmt.Sprintf("\n[%s %s:%s]", ref.Metadata.Name, ref.Metadata.Chapter, ref.Metadata.Verse))
			}
		}
	}

	finalResult := result.String()
	GlobalCache.Set(cacheKey, finalResult, reminderCacheTTL)

	return finalResult, nil
}

func stripHTML(s string) string {
	// Simple HTML tag stripper
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	// Also handle HTML entities
	s = result.String()
	s = strings.ReplaceAll(s, "&ldquo;", "\"")
	s = strings.ReplaceAll(s, "&rdquo;", "\"")
	s = strings.ReplaceAll(s, "&lsquo;", "'")
	s = strings.ReplaceAll(s, "&rsquo;", "'")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}
