package main

import (
	"context"
	"github.com/lxbot/lxlib"
	"github.com/mattn/go-mastodon"
	"github.com/mohemohe/temple"
	"log"
	"net/url"
	"os"
	"plugin"
	"strings"
	"time"
)

type (
	M = map[string]interface{}
)

var store *lxlib.Store
var ch *chan M
var reconnectCh chan int
var connected bool
var client *mastodon.Client

func Boot(s *plugin.Plugin, c *chan M) {
	var err error
	store, err = lxlib.NewStore(s)
	if err != nil {
		panic(err)
	}
	ch = c

	initStore()

	u := os.Getenv("LXBOT_MASTODON_BASE_URL")
	if u == "" {
		log.Fatalln("invalid url:", "'LXBOT_MASTODON_BASE_URL' にAPI URLを設定してください")
	}
	token := os.Getenv("LXBOT_MASTODON_ACCESS_TOKEN")
	if token == "" {
		log.Fatalln("invalid token:", "'LXBOT_MASTODON_ACCESS_TOKEN' にアクセストークンを設定してください")
	}
	client = mastodon.NewClient(&mastodon.Config{
		Server:      os.Getenv("LXBOT_MASTODON_BASE_URL"),
		AccessToken: os.Getenv("LXBOT_MASTODON_ACCESS_TOKEN"),
	})
	ws := client.NewWSClient()
	connected = false
	reconnectCh = make(chan int)
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
			m, err := lxlib.NewLXMessage(msg)
			if err != nil {
				log.Println("LXMessage error:", err)
				return nil
			}
			if strings.HasPrefix(m.Message.Text, os.Getenv("LXBOT_COMMAND_PREFIX")+"mstdn") {
				handleInternal(m)
				r, err := m.Reply().ToMap()
				if err != nil {
					log.Println("LXMessage error:", err)
					return nil
				}
				return r
			}
			return nil
		},
	}
}

func handleInternal(msg *lxlib.LXMessage) {
	p := os.Getenv("LXBOT_COMMAND_PREFIX")

	args := strings.Fields(msg.Message.Text)
	l := len(args)
	if l == 2 {
		switch args[1] {
		case "status":
			status(msg)
			return
		case "reconnect":
			reconnect(msg)
			return
		case "list":
			list(msg)
			return
		case "add":
			msg.SetText(p + "mstdn " + args[1] + " @screen_name@domain [all|image]")
			return
		case "remove":
			fallthrough
		case "find":
			msg.SetText(p + "mstdn " + args[1] + " @screen_name@domain")
			return
		}
	}
	if l == 3 {
		acct := strings.TrimPrefix(args[2], "@")

		switch args[1] {
		case "add":
			msg.SetText(p + "mstdn " + args[1] + " @screen_name@domain [all|image]")
			return
		case "remove":
			remove(msg, acct)
			return
		case "find":
			msg.SetText("//TODO:")
			return
		}
	}
	if l == 4 {
		log.Println("l: 4")

		switch args[1] {
		case "add":
			t := args[2]
			acct := t
			if strings.HasPrefix(t, "https://") {
				if u, err := url.Parse(t); err == nil {
					user := strings.Split(u.Path, "/")[1]
					acct = user + "@" + u.Host
				}
			}
			acct = strings.TrimPrefix(acct, "@")

			mode := strings.ToLower(args[3])

			log.Println("add")
			log.Println("acct:", acct)
			log.Println("mode:", mode)

			switch mode {
			case "all":
				fallthrough
			case "image":
				add(msg, acct, mode)
				return
			}
		}
	}
	msg.SetText(p + "mstdn [status|add|remove|list|find|reconnect]")
}

func status(msg *lxlib.LXMessage) {
	url := os.Getenv("LXBOT_MASTODON_BASE_URL")
	if connected {
		msg.SetText("`" + url + "/api/v1/streaming/?stream=user` に接続しています")
	} else {
		msg.SetText("`" + url + "/api/v1/streaming/?stream=user` から切断しています")
	}
}

func reconnect(msg *lxlib.LXMessage) {
	reconnectCh <- 1
	url := os.Getenv("LXBOT_MASTODON_BASE_URL")
	msg.SetText("`" + url + "/api/v1/streaming/?stream=user` に再接続しました")
}

func connect(client *mastodon.WSClient) {
	event, err := client.StreamingWSUser(context.Background())
	if err != nil {
		log.Println(err)
		time.Sleep(10 * time.Second)
		go connect(client)
		return
	}

	log.Println("start streaming loop")
	connected = true
LOOP:
	for {
		select {
		case e := <-event:
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
		case <-reconnectCh:
			break LOOP
		}
	}
	connected = false
	log.Println("exits streaming loop")

	go connect(client)
}

func add(msg *lxlib.LXMessage, acct string, mode string) {
	accounts, err := client.AccountsSearch(context.TODO(), "@"+acct, 1)
	if err != nil {
		msg.SetText("mastodon account search API エラーが発生しました")
		return
	}
	if len(accounts) == 0 {
		msg.SetText("@" + acct + "さんはFediverseに存在しません")
		return
	}
	account := accounts[0]
	result, err := client.AccountFollow(context.TODO(), account.ID)
	if err != nil {
		msg.SetText("mastodon account follow API エラーが発生しました")
		return
	}
	switch {
	case result.Following:
		msg.SetText("@" + acct + "さんを追加しました")
		addRoom(msg.Room.ID)
		addAcct(msg.Room.ID, acct, mode)
		return
	case result.Requested:
		msg.SetText("@" + acct + "さんにフォローリスエストを送りました（鍵垢です）")
		return
	case result.Blocking:
		msg.SetText("@" + acct + "さんの追加に失敗しました（ブロックされています）")
		return
	default:
		msg.SetText("@" + acct + "さんの追加に失敗しました（不明なエラー）")
		return
	}
}

func remove(msg *lxlib.LXMessage, acct string) {
	removeAcct(msg.Room.ID, acct)
	msg.SetText("@" + acct + "さんを削除しました")
}

func list(msg *lxlib.LXMessage) {
	text := "\n```\n"
	accts := listAcct(msg.Room.ID)
	for k, v := range accts {
		text += k + ": " + v.(string) + "\n"
	}
	text += "```"
	msg.SetText(text)
}

func dummyMessage() *lxlib.LXMessage {
	msg := M{
		"user": M{
			"id":   "user.id",
			"name": "user.name",
		},
		"room": M{
			"id":          "room.id",
			"name":        "room.name",
			"description": "room.description",
		},
		"message": M{
			"id":          "message.id",
			"text":        "message.text",
			"attachments": []M{},
		},
		"mode": "",
	}
	lxm, _ := lxlib.NewLXMessage(msg)
	return lxm
}

func onUpdate(status *mastodon.Status) {
	log.Println(status)
	rooms := getRooms()
	for _, room := range rooms {
		if mode, ok := acct(room, status.Account.Acct); ok {
			switch mode {
			case "image":
				if len(status.MediaAttachments) == 0 {
					return
				}
			}

			msg := dummyMessage()
			msg.Room.ID = room
			text := "@" + status.Account.Acct + " の"
			if status.Reblog != nil {
				text += "ブースト: "
				if status.Reblog.URL != "" {
					text += status.Reblog.URL
				} else {
					text += status.Reblog.URI
				}
			} else {
				text += "トゥート: "
				if status.URL != "" {
					text += status.URL
				} else {
					text += status.URI
				}
			}

			m, err := msg.SetText(text).Send().ToMap()
			if err == nil {
				*ch <- m
			}
		}
	}
}

func initStore() {
	var key string

	key = "lxbot_mastodon_rooms"
	rooms := store.Get(key)
	if rooms == nil {
		store.Set(key, M{})
	}
}

func getRooms() []string {
	rooms := store.Get("lxbot_mastodon_rooms")
	r := make([]string, 0)
	for k, _ := range rooms.(M) {
		r = append(r, k)
	}
	return r
}

func addRoom(roomID string) {
	key := "lxbot_mastodon_rooms"
	rooms := store.Get(key)
	rooms.(M)[roomID] = 1
	store.Set(key, rooms)
}

func escapeAcct(acct string) string {
	return strings.ReplaceAll(acct, ".", "__DOT__")
}

func unescapeAcct(acct string) string {
	return strings.ReplaceAll(acct, "__DOT__", ".")
}

func addAcct(roomID string, acct string, mode string) {
	key := "lxbot_mastodon_room_" + roomID
	current := store.Get(key)
	if current == nil {
		current = M{}
	}
	current.(M)[escapeAcct(acct)] = mode
	store.Set(key, current)
}

func removeAcct(roomID string, acct string) {
	key := "lxbot_mastodon_room_" + roomID
	current := store.Get(key)
	if current == nil {
		current = M{}
	}
	delete(current.(M), escapeAcct(acct))
	store.Set(key, current)
}

func acct(roomID string, acct string) (string, bool) {
	key := "lxbot_mastodon_room_" + roomID
	current := store.Get(key)
	if current == nil {
		return "", false
	}
	mode, ok := current.(M)[escapeAcct(acct)]
	if !ok {
		return "", false
	}
	return mode.(string), true
}

func listAcct(roomID string) M {
	key := "lxbot_mastodon_room_" + roomID
	current := store.Get(key)
	if current == nil {
		return M{}
	}

	for k, v := range current.(M) {
		current.(M)[unescapeAcct(k)] = v
		if k != unescapeAcct(k) {
			delete(current.(M), k)
		}
	}
	return current.(M)
}
