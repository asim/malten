package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/asim/malten/server"
	"github.com/sashabaranov/go-openai"
)

var (
	Key = os.Getenv("OPENAI_API_KEY")

	Client *openai.Client
)

const DefaultModel = "gpt-6-astra-light"

var stateMu sync.Mutex

type AI struct {
	context map[string][]Context
	persona string
}

var (
	DefaultPersona = `Listen. Be patient. Respond kindly. Limit responses to 1024 characters or less.`

	MaxTokens = 1024
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

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
		Model:     DefaultModel,
		Messages:  message,
		User:      user,
		MaxTokens: MaxTokens,
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
		listeners := make(map[string]bool, len(agents))
		for name, active := range agents {
			listeners[name] = active
		}
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
	o := server.NewObserver(stream)
	o.Internal = true
	server.Default.Observe(o)
	defer close(o.Kill)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg := <-o.Events:
			if msg.Text != "" {
				ai.execute(msg.Stream, msg.Text)
			}
		case <-ticker.C:
			if _, exists := server.Default.List()[stream]; !exists {
				return nil
			}
		}
	}
}

func (ai *AI) observe(stream, prompt string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	// if seen before ignore it
	if ignore[prompt] {
		fmt.Println("ignoring:", prompt)
		return
	}

	// append context
	ctx := append(ai.context[stream], Context{
		Prompt: prompt,
	})

	// cap number of messages
	for len(ctx) > 1024 {
		ctx = ctx[1:]
	}

	// save context
	ai.context[stream] = ctx
}

func (ai *AI) execute(stream, prompt string) {
	stateMu.Lock()
	// if seen before ignore it
	if ignore[prompt] {
		stateMu.Unlock()
		fmt.Println("ignoring:", prompt)
		return
	}

	history := append([]Context(nil), ai.context[stream]...)
	stateMu.Unlock()
	fmt.Printf("you#%s: %v\n", stream, prompt)

	// ask openai
	resp, err := Client.CreateChatCompletion(
		context.Background(),
		complete(prompt, stream, ai.persona, history...),
	)

	var reply string
	if err != nil {
		reply = err.Error()
	} else if len(resp.Choices) > 0 {
		reply = resp.Choices[0].Message.Content
	}

	fmt.Printf("ai#%s: %v\n", stream, reply)

	// ignore self
	stateMu.Lock()
	ignore[reply] = true
	stateMu.Unlock()

	http.PostForm("http://127.0.0.1:9090/messages", url.Values{
		"message": []string{reply},
		"stream":  []string{stream},
	})

	// append context
	stateMu.Lock()
	defer stateMu.Unlock()
	ctx := append(ai.context[stream], Context{
		Prompt: prompt,
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
	ai.context = make(map[string][]Context)

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
