package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/asim/malten/server"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waproto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var (
	mtx     sync.Mutex
	streams = map[string]bool{}

	Client *whatsmeow.Client
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 4096
)

func Key(id string) string {
	hasher := fnv.New128()
	hasher.Write([]byte(id))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func observe(jid types.JID) {
	// can't do anything without a client
	if Client == nil {
		fmt.Println("No client available")
		return
	}

	// stream key
	stream := Key(jid.String())

	mtx.Lock()
	_, ok := streams[stream]
	if ok {
		fmt.Println("Stream already observed")
		mtx.Unlock()
		return
	}
	streams[stream] = true
	mtx.Unlock()

	fmt.Println("Observing stream", stream)

	// observe the stream
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:9090/events?stream="+stream, http.Header{})
	if err != nil {
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
				fmt.Println("Stream exited")
				return
			case <-t.C:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					fmt.Println("Stream write error", err)
					return
				}
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				return
			}
			fmt.Println("websocket error", err)
			return
		}

		if len(message) == 0 {
			continue
		}

		// decode message
		var msg *server.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			fmt.Println(err)
			return
		}

		// do not forward that
		if msg.Type == "command" {
			continue
		}

		if msg.Text == "connect" || msg.Text == "close" {
			continue
		}

		// send message to whatsapp
		fmt.Printf("Sending message to: %v %v\n", stream, msg.Text)
		rsp, err := Client.SendMessage(context.Background(), jid, &waproto.Message{
			Conversation: proto.String("malten: " + msg.Text),
		})
		if err != nil {
			fmt.Printf("Error sending message to %v\n", err)
			continue
		}

		fmt.Printf("Message response: %+v\n", rsp)
	}
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		// turn the jid into a stream hash
		stream := Key(v.Info.Chat.String())
		message := v.Message.GetConversation()

		// try get extended message
		if len(message) == 0 {
			text := v.Message.GetExtendedTextMessage()
			if text != nil {
				message = *text.Text
			}
		}

		fmt.Println("JID to Stream", v.Info.Chat.String(), stream)
		fmt.Printf("Info: %+v\n", v.Info)
		fmt.Printf("Message: %+v\n", v.Message)

		// nothing to do
		if len(message) == 0 {
			fmt.Println("No message text, returning")
			return
		}

		// observe the stream
		mtx.Lock()
		_, ok := streams[stream]
		mtx.Unlock()

		// no observer
		if !ok {
			go observe(v.Info.Chat)
			time.Sleep(time.Second)
		}

		// send the command
		if strings.Contains(strings.ToLower(message), "malten") {
			// send to the server
			http.PostForm("http://127.0.0.1:9090/commands", url.Values{
				"prompt": []string{message},
				"stream": []string{stream},
			})
			return
		}

		// do not post anything else yet
	}
}

func Run(ctx context.Context) {
	if os.Getenv("WHATSAPP_CLIENT") != "true" {
		return
	}

	dbLog := waLog.Stdout("Database", "ERROR", true)
	// Make sure you add appropriate DB connector imports, e.g. github.com/mattn/go-sqlite3 for SQLite
	container, err := sqlstore.New("sqlite3", "file:malten.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "ERROR", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				//fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// set the client
	Client = client

	<-ctx.Done()

	client.Disconnect()
}
