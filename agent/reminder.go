package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/asim/malten/server"
)

const reminderOrigin = "https://reminder.dev"

const dailyReflectionPrompt = `Write one original daily reflection for Malten, a network for reflection rooted in Islam and open to everyone, including people encountering Islam for the first time.
Choose one idea grounded in the supplied daily readings. The readings can speak to different things. Begin with a small, recognisable moment or choice in everyday life. In two or three short sentences, offer a thought and end with one open question someone can answer from their own experience.
Use your own words in plain English. Honour the meaning of the source and explain any unfamiliar word you need. Leave room for the reader to respond differently or simply sit with the question. Return only the plain-text post. Malten shows the original readings and credits separately.`

// A missing archive is retried on the next tick. Once published, the post lives
// in the same rolling stream and recovery snapshot as any other reflection.
func (a *Agent) runReminders(ctx context.Context) {
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	var published string
	for {
		date := time.Now().UTC().Format(time.DateOnly)
		if date != published {
			if err := a.publishReminder(ctx, client, reminderOrigin, date); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Daily reminder %s: %v", date, err)
			} else {
				published = date
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *Agent) publishReminder(ctx context.Context, client *http.Client, base, date string) error {
	id := "reminder-" + date
	if len(a.server.Retrieve(id, server.DefaultStream, 0, 0, 1)) > 0 {
		return nil
	}
	m, err := fetchReminder(ctx, client, base, date)
	if err != nil {
		return err
	}
	if a.Available() {
		original, err := a.writeDailyReflection(ctx, m)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Daily reflection %s: %v; publishing the source reminder", date, err)
		} else {
			m.Text, m.Source = original, muMaltenURL
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.Created = time.Now().UnixNano()
	if err := a.server.Publish(m); err != nil {
		return err
	}
	return a.server.Checkpoint()
}

// Each daily post starts a fresh Mu request with only the public readings.
// Keep the editorial request separate from reader conversations.
func (a *Agent) writeDailyReflection(ctx context.Context, reminder *server.Message) (string, error) {
	req := muRequest{Prompt: dailyReflectionPrompt}
	// Keep all four readings within Mu's 16 KB of context, including labels.
	includeSummary := true
	for _, ref := range reminder.References {
		if ref.Text == "" {
			continue
		}
		req.Context.Messages = append(req.Context.Messages, contextMessage{Author: ref.Title, Text: excerpt(ref.Text, 3800)})
		if ref.Text == reminder.Text {
			includeSummary = false
		}
	}
	if includeSummary {
		req.Context.Messages = append(req.Context.Messages, contextMessage{Author: "Reminder.dev reflection", Text: excerpt(reminder.Text, 3800)})
	}
	if wire, _ := json.Marshal(req); len(wire) > muRequestBytes {
		// Four 1,200-byte excerpts fit on the wire even with sixfold JSON escaping.
		for i := range req.Context.Messages {
			m := &req.Context.Messages[i]
			m.Text = excerpt(m.Text, 1200)
		}
	}
	reply, err := a.ask(ctx, req)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(reply.Text)
	if len(text) > server.MaxMessageSize {
		return "", errors.New("Mu returned a reflection too long for a stream post")
	}
	// A quotation from the archive must keep its original attribution.
	if text == reminder.Text {
		return "", errors.New("Mu repeated the source reminder")
	}
	for _, ref := range reminder.References {
		if text == ref.Text {
			return "", errors.New("Mu repeated a source reading")
		}
	}
	return text, nil
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
		Message, Verse, Hadith, Name string
		Links                        struct{ Verse, Hadith, Name string }
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&daily); err != nil {
		return nil, err
	}
	archive := reminderOrigin + "/daily/" + date
	source, text := archive, strings.TrimSpace(daily.Message)
	var references []server.Reference
	for _, reading := range []struct{ title, text, link, path string }{
		{"Qur’an passage", daily.Verse, daily.Links.Verse, "/quran/"},
		{"Hadith", daily.Hadith, daily.Links.Hadith, "/hadith/"},
		{"Name of Allah", daily.Name, daily.Links.Name, "/names/"},
	} {
		ref := server.Reference{Title: reading.title, Text: excerpt(strings.TrimSpace(reading.text), server.MaxMessageSize)}
		link, err := url.Parse(reading.link)
		if err == nil && len(reading.link) <= 512 && !link.IsAbs() && link.Host == "" && strings.HasPrefix(link.Path, reading.path) {
			ref.URL = reminderOrigin + link.String()
		}
		if ref.Text == "" && ref.URL == "" {
			continue
		}
		references = append(references, ref)
		if text == "" && ref.Text != "" {
			text = ref.Text
			if ref.URL != "" {
				source = ref.URL
			}
		}
	}
	if text == "" {
		return nil, errors.New("daily entry has no reflection or readings")
	}
	m := server.NewMessage(text, server.DefaultStream)
	m.Id, m.Source = "reminder-"+date, source
	m.References = append(references, server.Reference{Title: "Full reminder on Reminder.dev", URL: archive})
	return m, nil
}

func dailyByline(m *server.Message) string {
	if m.Source == muMaltenURL {
		return "Malten’s daily reflection"
	}
	return "Daily Reminder"
}

// Keep all three source labels and links in context even for a long reading.
func reminderContext(m *server.Message) string {
	if m.Stream != server.DefaultStream || !strings.HasPrefix(m.Id, "reminder-") {
		return ""
	}
	if len(m.References) == 0 && m.Metadata != nil {
		return strings.TrimSpace("Qur’an passage\n" + excerpt(m.Metadata.Description, 640) + "\n" + m.Metadata.Url)
	}
	var parts []string
	for _, ref := range m.References {
		part := ref.Title
		if ref.Text != "" && ref.Text != m.Text {
			part += "\n" + excerpt(ref.Text, 640)
		}
		if ref.URL != "" && ref.URL != m.Source {
			part += "\n" + ref.URL
		}
		if part != ref.Title {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (req *muRequest) addReminderSources(m *server.Message) {
	sources := reminderContext(m)
	if sources == "" {
		return
	}
	original := req.Context.Messages
	req.Context.Messages = append(original, contextMessage{Author: "Readings from Reminder.dev", Text: sources})
	// Leave the reader's question and earlier exchange intact when space is tight.
	if wire, _ := json.Marshal(req); len(wire) > muRequestBytes {
		req.Context.Messages = original
	}
}
