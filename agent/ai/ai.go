package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/asim/malten/server"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

var (
	Key = os.Getenv("OPENAI_API_KEY")

	Client *openai.Client
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

type AI struct {
	context map[string][]Context
	persona string
}

var (
	DefaultPersona = `Listen. Be patient. Respond kindly. Limit responses to 1024 characters or less.`
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

type Context struct {
	Prompt string
	Reply  string
}

func complete(prompt, user, persona string, ctx ...Context) openai.ChatCompletionRequest {
	message := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: persona,
	}}

	for _, c := range ctx {
		// set the user message
		message = append(message, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: c.Prompt,
		})
		// set the assistant response
		message = append(message, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: c.Reply,
		})
	}

	// append the actual next prompt
	message = append(message, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})

	return openai.ChatCompletionRequest{
		Model:    openai.GPT3Dot5Turbo,
		Messages: message,
		User:     user,
	}
}

func (ai *AI) Listen() error {
	t := time.NewTicker(time.Second * 5)
	defer t.Stop()

	var mtx sync.RWMutex
	agents := map[string]bool{}

	// get streams
	for _ = range t.C {
		streams := server.Default.List()

		mtx.Lock()
		listeners := agents
		mtx.Unlock()

		for name, _ := range streams {
			// we are listening
			if _, ok := listeners[name]; ok {
				continue
			}

			mtx.Lock()
			agents[name] = true
			mtx.Unlock()

			// create a new listener
			go func(stream string) {
				defer func() {
					// stopped listening
					mtx.Lock()
					delete(agents, stream)
					mtx.Unlock()
				}()

				// start listening
				if err := ai.listen(stream); err != nil {
					fmt.Println("stopped listening to", stream, err)
				}
			}(name)

		}
	}

	return nil
}

func (ai *AI) listen(stream string) error {
	if ai.context == nil {
		ai.context = make(map[string][]Context)
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:9090/events?stream="+stream, http.Header{})
	if err != nil {
		return err
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
				return err
			}
		}

		// decode message
		var msg *server.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			fmt.Println(err)
			return err
		}

		// think and respond
		ai.think(msg.Stream, msg.Text)
	}

	return nil
}

func (ai *AI) think(stream, text string) {
	// if seen before ignore it
	if ignore[text] {
		fmt.Println("ignoring:", text)
		return
	}

	// ask openai
	resp, err := Client.CreateChatCompletion(
		context.Background(),
		complete(text, stream, ai.persona, ai.context[stream]...),
	)

	var reply string
	if err != nil {
		reply = err.Error()
	} else {
		reply = resp.Choices[0].Message.Content
	}

	fmt.Println("you:", text)
	fmt.Println("ai:", reply)

	// ignore self
	ignore[reply] = true

	http.PostForm("http://127.0.0.1:9090/messages", url.Values{
		"text":   []string{reply},
		"stream": []string{stream},
	})

	// append context
	ctx := append(ai.context[stream], Context{
		Prompt: text,
		Reply:  reply,
	})

	// cap number of messages we send
	for len(ctx) > 1024 {
		ctx = ctx[1:]
	}

	// save context
	ai.context[stream] = ctx
}

func New(persona string) (*AI, error) {
	if len(Key) == 0 {
		return nil, errors.New("missing OPENAI_API_KEY")
	}

	// set the client
	Client = openai.NewClient(Key)

	ai := new(AI)
	ai.persona = persona

	return ai, nil
}

func Run() {
	ai, err := New(DefaultPersona)
	if err != nil {
		fmt.Println("AI not running:", err)
		return
	}

	for {
		if err := ai.Listen(); err != nil {
			fmt.Println(err)
			time.Sleep(time.Second)
		}
	}
}
