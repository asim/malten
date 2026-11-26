package server

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultDailyAI = 100

type quotaError struct {
	message string
	after   time.Duration
}

func (e *quotaError) Error() string { return e.message }

// WriteQuotaError returns true when the request hit a local usage limit.
func WriteQuotaError(w http.ResponseWriter, err error) bool {
	var q *quotaError
	if !errors.As(err, &q) {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int((q.after+time.Second-1)/time.Second)))
	http.Error(w, q.message, http.StatusTooManyRequests)
	return true
}

// ClientNetwork trusts only the address appended by the local reverse proxy.
// IPv6 clients sharing a /64 use the same allowance.
func ClientNetwork(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	ip = ip.Unmap()
	if ip.IsLoopback() {
		hops := r.Header.Values("X-Forwarded-For")
		if len(hops) > 0 {
			last := hops[len(hops)-1]
			last = strings.TrimSpace(last[strings.LastIndexByte(last, ',')+1:])
			if forwarded, err := netip.ParseAddr(last); err == nil {
				ip = forwarded.Unmap()
			}
		}
	}
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}

type usageWindow struct {
	count int
	until time.Time
}

type aiTotal struct {
	Date  string
	Count int
}

type limits struct {
	mu         sync.Mutex
	windows    map[string]usageWindow
	nextPrune  time.Time
	activeAI   int
	dailyLimit int
	total      aiTotal
	path       string
}

// take is called with mu held. Expired client identifiers are only kept in memory.
func (l *limits) take(key string, max int, period time.Duration, now time.Time) error {
	if !now.Before(l.nextPrune) {
		for k, v := range l.windows {
			if !v.until.After(now) {
				delete(l.windows, k)
			}
		}
		l.nextPrune = now.Add(time.Minute)
	}
	v, ok := l.windows[key]
	if !ok && len(l.windows) >= 4096 {
		return &quotaError{"Too many requests. Please try again in a minute.", time.Minute}
	}
	if !now.Before(v.until) {
		v = usageWindow{until: now.Add(period)}
	}
	if v.count >= max {
		return &quotaError{"Too many requests from this network. Please try again later.", v.until.Sub(now)}
	}
	v.count++
	l.windows[key] = v
	return nil
}

// ConfigureAI restores the aggregate count before the server accepts requests.
func (s *Server) ConfigureAI(path, setting string) error {
	l := &s.limits
	if setting != "" {
		n, err := strconv.Atoi(setting)
		if err != nil || n < 0 {
			return errors.New("MALTEN_AI_DAILY_LIMIT must be a nonnegative number")
		}
		l.dailyLimit = n
	}
	l.path = path
	if err := os.Remove(path + ".tmp"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &l.total); err != nil {
		return err
	}
	if _, err := time.Parse(time.DateOnly, l.total.Date); err != nil || l.total.Count < 0 {
		return errors.New("invalid saved AI usage")
	}
	return nil
}

// BeginAI reserves one free question before queuing or sending it to Mu.
// A successful reservation is persisted before any upstream work begins.
func (s *Server) BeginAI(network string) (func(), error) {
	l := &s.limits
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().UTC()
	next := l.total
	if date := now.Format(time.DateOnly); next.Date != date {
		next = aiTotal{Date: date}
	}
	if next.Count >= l.dailyLimit {
		return nil, &quotaError{"Malten’s free AI allowance has been reached for today. You can still post reflections.", now.Truncate(24 * time.Hour).Add(24 * time.Hour).Sub(now)}
	}
	if l.activeAI >= 4 {
		return nil, &quotaError{"Malten is busy. Please try again shortly.", 5 * time.Second}
	}
	if err := l.take("ai:"+network, 5, 10*time.Minute, now); err != nil {
		return nil, err
	}
	next.Count++
	if l.path != "" {
		if err := saveJSON(l.path, next); err != nil {
			log.Printf("save AI usage: %v", err)
			return nil, errors.New("Malten is temporarily unavailable.")
		}
	}
	l.total = next
	l.activeAI++
	return sync.OnceFunc(func() {
		l.mu.Lock()
		l.activeAI--
		l.mu.Unlock()
	}), nil
}

func (s *Server) limitWrites(kind, network string, max int) error {
	l := &s.limits
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.take(kind+":"+network, max, time.Minute, time.Now())
}

// FinishCommand releases the free AI slot held by an admitted stream question.
func (m *Message) FinishCommand() {
	if m.finish != nil {
		m.finish()
	}
}
