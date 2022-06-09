package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	postQuote(getQOD)
	postQuote(getRandom)
}

func think(stream, text string) {
	http.PostForm("http://127.0.0.1:9090/messages", url.Values{
		"text":   []string{text},
		"stream": []string{stream},
	})
}

func getQOD() (string, []string, error) {
	r, err := http.Get("http://api.theysaidso.com/qod.json")
	if err != nil {
		return "", nil, err
	}
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return "", nil, err
	}

	if data["contents"] == nil {
		return "", nil, fmt.Errorf("No data")
	}

	content := data["contents"].(map[string]interface{})
	hashtags := []string{}
	for _, tag := range content["tags"].([]interface{}) {
		hashtags = append(hashtags, "#"+tag.(string))
	}

	return fmt.Sprintf("\"%s\" - %s %s", content["quote"], content["author"], strings.Join(hashtags, " ")), hashtags, nil
}

func getRandom() (string, []string, error) {
	r, err := http.Get("http://www.iheartquotes.com/api/v1/random?format=json&max_characters=400&max_lines=1")
	if err != nil {
		return "", nil, err
	}
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return "", nil, err
	}

	if data["quote"] == nil {
		return "", nil, fmt.Errorf("No data")
	}

	hashtags := []string{}
	for _, tag := range data["tags"].([]interface{}) {
		hashtags = append(hashtags, "#"+tag.(string))
	}

	return fmt.Sprintf("\"%s\" %s", data["quote"].(string), strings.Join(hashtags, " ")), hashtags, nil
}

func postQuote(fn func() (string, []string, error)) {
	quote, hashtags, err := fn()
	if err != nil {
		return
	}
	think("_", quote)
	for _, tag := range hashtags {
		think(tag[1:], quote)
	}
}

func main() {
	t1 := time.NewTicker(time.Hour * 24)
	t2 := time.NewTicker(time.Hour)

	for {
		select {
		case <-t1.C:
			postQuote(getQOD)
		case <-t2.C:
			postQuote(getRandom)
		}
	}
}
