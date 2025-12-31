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
			Book    string `json:"book"`
			Volume  string `json:"volume"`
			Meaning string `json:"meaning"`
			English string `json:"english"`
		} `json:"metadata"`
	} `json:"references"`
}

func reminderHandler(args []string) (string, error) {
	// If args provided, do search
	if len(args) > 0 {
		return searchHandler(args)
	}

	// Otherwise return daily reminder
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

func searchHandler(args []string) (string, error) {
	query := strings.Join(args, " ")
	
	// Check cache
	cacheKey := "reminder:search:" + query
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

	// Format: answer + top 3 references with links
	var result strings.Builder
	
	if data.Answer != "" {
		answer := stripHTML(data.Answer)
		result.WriteString(answer)
	}

	if len(data.References) > 0 {
		max := 3
		if len(data.References) < max {
			max = len(data.References)
		}
		for i := 0; i < max; i++ {
			ref := data.References[i]
			switch ref.Metadata.Source {
			case "quran":
				// Use path format to avoid # fragment parsing issues
				result.WriteString(fmt.Sprintf("\n%s %s:%s - https://reminder.dev/quran/%s/%s", 
					ref.Metadata.Name, ref.Metadata.Chapter, ref.Metadata.Verse,
					ref.Metadata.Chapter, ref.Metadata.Verse))
			case "bukhari":
				result.WriteString(fmt.Sprintf("\nBukhari %s - https://reminder.dev/hadith", ref.Metadata.Book))
			case "names":
				name := ref.Metadata.Name
				if name == "" {
					name = ref.Metadata.Meaning + " (" + ref.Metadata.English + ")"
				}
				result.WriteString(fmt.Sprintf("\n%s - https://reminder.dev/names", name))
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
