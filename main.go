package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

var (
	max = 1000
	latest chan string
	reqCh  chan chan []string
)

func main() {
	latest = make(chan string, 100)
	reqCh = make(chan chan []string)

        go func() {
                var thoughts []string

                for {
                        select {
                        case thought := <-latest:
				if len(thought) > 512 {
					thought = thought[:512]
				}

                                thoughts = append(thoughts, thought)
                                if len(thoughts) > max {
                                        thoughts = thoughts[1:]
                                }
                        case ch := <-reqCh:
                                ch <- thoughts
                        }
                }
        }()

	http.Handle("/", http.FileServer(http.Dir("html")))

	http.HandleFunc("/thoughts", func(w http.ResponseWriter, r *http.Request) {
		ch := make(chan []string)
		reqCh <- ch
		thoughts := <-ch
		b, _ := json.Marshal(thoughts)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(b))
		return
	})

	http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		select {
		case latest <- r.Form.Get("text"):
		default:
		}
		http.Redirect(w, r, "/", 302)
	})

	http.ListenAndServe(":8080", nil)
}
