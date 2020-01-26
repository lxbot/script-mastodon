package main

import (
	"context"
	"github.com/mattn/go-mastodon"
	"github.com/mohemohe/temple"
	"log"
	"os"
	"plugin"
	"strings"
	"time"
)

type (
	M = map[string]interface{}
)

var store *plugin.Plugin
var ch *chan M

func Boot(s *plugin.Plugin, c *chan M) {
	store = s
	ch = c

	initStore()

	url := os.Getenv("LXBOT_MASTODON_BASE_URL")
	if url == "" {
		log.Fatalln("invalid url:", "'LXBOT_MASTODON_BASE_URL' にAPI URLを設定してください")
	}
	token := os.Getenv("LXBOT_MASTODON_ACCESS_TOKEN")
	if token == "" {
		log.Fatalln("invalid token:", "'LXBOT_MASTODON_ACCESS_TOKEN' にアクセストークンを設定してください")
	}
	client := mastodon.NewClient(&mastodon.Config{
		Server: os.Getenv("LXBOT_MASTODON_BASE_URL"),
		AccessToken: os.Getenv("LXBOT_MASTODON_ACCESS_TOKEN"),
	})
	ws := client.NewWSClient()
	go connect(ws)
}

func Help() string {
	t := `{{.p}}mstdn: mastodon
`
	m := M{
		"p": os.Getenv("LXBOT_COMMAND_PREFIX"),
	}
	r, _ := temple.Execute(t, m)
	return r
}

func OnMessage() []func(M) M {
	return []func(M) M{
		func(msg M) M {
			text := msg["message"].(M)["text"].(string)
			if strings.HasPrefix(text, os.Getenv("LXBOT_COMMAND_PREFIX")+"mstdn") {
				msg["mode"] = "reply"
				msg["message"].(M)["text"] = "text"
				return msg
			}
			return nil
		},
	}
}

func connect(client *mastodon.WSClient) {
	event, err := client.StreamingWSUser(context.Background())
	if err != nil {
		log.Println(err)
		time.Sleep(10 * time.Second)
		go connect(client)
		return
	}

LOOP:
	for {
		e := <- event
		switch e.(type) {
		case *mastodon.UpdateEvent:
			ue := e.(*mastodon.UpdateEvent)
			onUpdate(ue.Status)
			break
		case *mastodon.NotificationEvent:
			log.Println("NotificationEvent:", e)
			break
		case *mastodon.DeleteEvent:
			log.Println("DeleteEvent:", e)
			break
		case *mastodon.ErrorEvent:
			log.Println("ErrorEvent:", e)
			break LOOP
		}
	}

	go connect(client)
}

func onUpdate(status *mastodon.Status) {
	log.Println(status)
}

func initStore() {
	var key string

	key = "lxbot_mastodon_rooms"
	rooms := get(key)
	if rooms == nil {
		set(key, []interface{}{})
	}
}

func getRooms() []string {
	rooms := get("lxbot_mastodon_rooms")
	l := len(rooms.([]interface{}))
	r := make([]string, l, l)
	for i, v := range rooms.([]interface{}) {
		r[i] = v.(string)
	}
	return r
}

func get(key string) interface{} {
	fn, err := store.Lookup("Get")
	if err != nil {
		log.Println(err)
		return nil
	}
	result := fn.(func(string) interface{})(key)
	return result
}

func set(key string, value interface{}) {
	fn, err := store.Lookup("Set")
	if err != nil {
		log.Println(err)
		return
	}
	fn.(func(string, interface{}))(key, value)
}
