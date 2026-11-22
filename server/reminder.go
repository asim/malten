package server

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
)

// RunReminders publishes the dated Qur'an passage into the home stream.
// Retry a missing daily entry until Reminder has archived it.
func (s *Server) RunReminders(ctx context.Context) {
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
				case s.Events <- message:
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

func fetchReminder(ctx context.Context, client *http.Client, base, date string) (*Message, error) {
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
		Verse string
		Links struct{ Verse string }
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&daily); err != nil {
		return nil, err
	}
	if strings.TrimSpace(daily.Verse) == "" {
		return nil, fmt.Errorf("empty verse")
	}
	message := NewMessage(daily.Verse, defaultStream)
	message.Id = "reminder-" + date
	message.ReminderURL = "https://reminder.dev"
	if link, err := url.Parse(daily.Links.Verse); err == nil && !link.IsAbs() && link.Host == "" && strings.HasPrefix(link.Path, "/quran/") {
		message.ReminderURL += link.String()
	}
	return message, nil
}
