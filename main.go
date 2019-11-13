//
//                +----------+     +----------+
// client1 -------|          |     |          |------- client3
//                |   Srv1   |-----|   Srv2   |
//                |          |     |          |
// client2 -------|          |     |          |------- client4
//                +----------+     +----------+
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/chascale/chascale-server/server"
)

var addr = flag.String("addr", ":8080", "http service address")

func main() {
	flag.Parse()
	hub := server.NewHub()
	go hub.Run()
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		server.ServeWS(hub, w, r)
	})
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
