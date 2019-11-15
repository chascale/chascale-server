package server

import (
	"log"
	"net/http"

	"github.com/chascale/chascale-server/data"
)

// ServeWS handles websocket requests from the peer.
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] chascale: %+v\n", r)
	id, ok := r.URL.Query()["id"]
	if !ok || len(id) == 0 {
		w.WriteHeader(400)
		w.Write([]byte(`{"error": "missing id"}`))
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	wsConn := &WSConnection{ID: id[0], hub: hub, conn: conn, send: make(chan data.Message, 256)}
	hub.register <- wsConn

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go wsConn.writePump()
	go wsConn.readPump()
}
