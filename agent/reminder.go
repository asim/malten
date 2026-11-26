package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/asim/malten/server"
)

// runReminders publishes the day's reflection from Reminder.dev in the home stream.
// Retry a missing daily entry until Reminder has archived it.
func runReminders(ctx context.Context, events chan<- *server.Message) {
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	var published string
	for {
		date := time.Now().UTC().Format("2006-01-02")
		if date != published {
			message, err := fetchReminder(ctx, client, "https://reminder.dev", date)
			if err != nil {
				log.Printf("Daily reminder %s: %v", date, err)
			} else {
				select {
				case events <- message:
					published = date
				case <-ctx.Done():
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func fetchReminder(ctx context.Context, client *http.Client, base, date string) (*server.Message, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/daily/"+date, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var daily struct {
		Message string
		Verse   string
		Links   struct{ Verse string }
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&daily); err != nil {
		return nil, err
	}
	const origin = "https://reminder.dev"
	verseURL := ""
	if link, err := url.Parse(daily.Links.Verse); err == nil && len(daily.Links.Verse) <= 512 && !link.IsAbs() && link.Host == "" && strings.HasPrefix(link.Path, "/quran/") {
		verseURL = origin + link.String()
	}

	text, verse := strings.TrimSpace(daily.Message), strings.TrimSpace(daily.Verse)
	source := origin + "/daily/" + date
	var passage *server.Metadata
	if text == "" {
		if verse == "" {
			return nil, fmt.Errorf("daily entry has no reflection or verse")
		}
		text, source = verse, origin
		if verseURL != "" {
			source = verseURL
		}
	} else if verse != "" || verseURL != "" {
		passage = &server.Metadata{Type: "article", Title: "Qur’an passage", Url: verseURL, Description: excerpt(verse, server.MaxMessageSize)}
	}

	message := server.NewMessage(text, server.DefaultStream)
	message.Id = "reminder-" + date
	message.Source = source
	message.Metadata = passage
	return message, nil
}

// The reflection and the scripture are distinct sources when both are present.
func reminderPassage(m *server.Message) string {
	if m.Stream != server.DefaultStream || !strings.HasPrefix(m.Id, "reminder-") || m.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(excerpt(m.Metadata.Description, 1400) + "\n" + m.Metadata.Url)
}

func reflectionWithPassage(m *server.Message) string {
	if passage := reminderPassage(m); passage != "" {
		return excerpt(m.Text, 2400) + "\n\nQur’an passage\n" + passage
	}
	return m.Text
}
