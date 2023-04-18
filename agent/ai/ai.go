package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/asim/malten/server"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

var (
	Key = os.Getenv("OPENAI_API_KEY")

	AI *openai.Client
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var ignore = map[string]bool{
	"connect": true,
	"close":   true,
}

func listen() {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:9090/events", http.Header{})
	if err != nil {
		fmt.Println(err)
		return
	}

	defer func() {
		conn.Close()
	}()

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	exit := make(chan bool, 1)

	// write loop
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()

		for {
			select {
			case <-exit:
				fmt.Println("ws exited")
				return
			case <-t.C:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					fmt.Println("ws write error", err)
					return
				}
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("error: %v\n", err)
			}
			break
		}

		// decode message
		var msg *server.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			fmt.Println(err)
			continue
		}

		// think and respond
		think(msg.Stream, msg.Text)
	}
}

func think(stream, text string) {

	// if seen before ignore it
	if ignore[text] {
		fmt.Println("ignoring:", text)
		return
	}

	// ask openai
	resp, err := AI.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: text,
				},
			},
		},
	)

	var reply string
	if err != nil {
		reply = "ai: " + err.Error()
	} else {
		reply = "ai: " + resp.Choices[0].Message.Content
	}
	fmt.Println(text)
	fmt.Println(reply)

	// ignore self
	ignore[reply] = true

	http.PostForm("http://127.0.0.1:9090/messages", url.Values{
		"text":   []string{reply},
		"stream": []string{stream},
	})
}

func main() {
	if len(Key) == 0 {
		fmt.Println("missing OPENAI_API_KEY")
	}

	// set the client
	AI = openai.NewClient(Key)

	// start listening
	listen()
}
