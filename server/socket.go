package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 60 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = 15 * time.Second

	// Maximum message size allowed from client.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// check if the request is for websockets
func IsWebSocket(r *http.Request) bool {
	contains := func(key, val string) bool {
		vv := strings.Split(r.Header.Get(key), ",")
		for _, v := range vv {
			if val == strings.ToLower(strings.TrimSpace(v)) {
				return true
			}
		}
		return false
	}

	if contains("Connection", "upgrade") && contains("Upgrade", "websocket") {
		return true
	}

	return false
}

// serve an actual websocket
func ServeWebSocket(w http.ResponseWriter, r *http.Request, o *Observer) {
	var rspHdr http.Header
	// we use Sec-Websocket-Protocol to pass auth headers so just accept anything here
	if prots := r.Header.Values("Sec-WebSocket-Protocol"); len(prots) > 0 {
		rspHdr = http.Header{}
		for _, p := range prots {
			rspHdr.Add("Sec-WebSocket-Protocol", p)
		}
	}

	// upgrade the connection
	conn, err := upgrader.Upgrade(w, r, rspHdr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// assume text types
	msgType := websocket.TextMessage

	// create a stream
	s := stream{
		ctx:         r.Context(),
		conn:        conn,
		events:      o.Events,
		messageType: msgType,
		killed:      o.Kill,
		observer:    o,
	}

	// start processing the stream
	s.run()
}

type stream struct {
	// message type requested (binary or text)
	messageType int
	// request context
	ctx context.Context
	// the websocket connection.
	conn *websocket.Conn
	// the downstream connection.
	events chan *Message
	// if observer is killed
	killed chan bool
	// the observer
	observer *Observer
}

func (s *stream) run() {
	defer func() {
		s.conn.Close()
	}()

	// to cancel everything
	stopCtx, cancel := context.WithCancel(context.Background())

	// wait for things to exist
	wg := sync.WaitGroup{}
	wg.Add(2)

	// establish the loops
	go s.bufToClientLoop(cancel, &wg, stopCtx)
	go s.clientToServerLoop(cancel, &wg, stopCtx)
	wg.Wait()
}

func (s *stream) clientToServerLoop(cancel context.CancelFunc, wg *sync.WaitGroup, stopCtx context.Context) {
	defer func() {
		cancel()
		wg.Done()
	}()

	s.conn.SetReadLimit(maxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error { s.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		select {
		case <-stopCtx.Done():
			return
		default:
		}

		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Println(err)
			}
			return
		}

		// TODO: pass on client to server messages
		fmt.Println("Got msg", msg)
	}

}

func (s *stream) bufToClientLoop(cancel context.CancelFunc, wg *sync.WaitGroup, stopCtx context.Context) {
	defer func() {
		s.conn.Close()
		cancel()
		wg.Done()

	}()
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-stopCtx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-s.killed:
			s.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case <-ticker.C:
			s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case msg := <-s.observer.Events:
			if msg.Stream != s.observer.Stream {
				fmt.Println("ignoring", msg.Stream, s.observer.Stream)
				continue
			}

			// read response body
			s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			w, err := s.conn.NextWriter(s.messageType)
			if err != nil {
				return
			}
			b, _ := json.Marshal(msg)
			if _, err := w.Write(b); err != nil {
				return
			}
			if err := w.Close(); err != nil {
				return
			}
		}
	}

}
