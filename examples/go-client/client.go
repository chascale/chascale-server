package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", ":8080", "http service address")
var clientID = flag.String("client_id", "", "client id")
var toID = flag.String("to_id", "", "detination client id")

type clientMsg struct {
	ToIDs   []string `json:"toIds"`
	Message string   `json:"message"`
}

func main() {
	flag.Parse()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws"}
	if len(*clientID) == 0 {
		*clientID = uuid.New().String()
	}
	u.RawQuery = fmt.Sprintf("id=%s", *clientID)

	log.Println(u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			m := clientMsg{}
			err := c.ReadJSON(&m)
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", m)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	toIDs := []string{}
	if len(*toID) > 0 {
		toIDs = append(toIDs, *toID)
	}
	log.Printf("toIDs: %v\n", toIDs)

	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			m := clientMsg{
				ToIDs:   toIDs,
				Message: fmt.Sprintf("From:%s, msg: %s", *clientID, strconv.FormatInt(t.Unix(), 10)),
			}
			//mbytes, err := json.Marshal(&m)
			//if err != nil {
			//	log.Println("json marshal:", err)
			//	return
			//}
			err := c.WriteJSON(&m)
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
