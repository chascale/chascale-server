package server

// Hub maintains the set of active conns and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	conns map[string]*WSConnection

	// Inbound messages from the clients.
	broadcast chan toClientMsg

	// Register requests from the clients.
	register chan *WSConnection

	// Unregister requests from clients.
	unregister chan *WSConnection
}

type toClientMsg struct {
	toIDs []string
	msg   []byte
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan toClientMsg),
		register:   make(chan *WSConnection),
		unregister: make(chan *WSConnection),
		conns:      make(map[string]*WSConnection),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.conns[conn.ID] = conn
		case conn := <-h.unregister:
			if _, ok := h.conns[conn.ID]; ok {
				delete(h.conns, conn.ID)
				close(conn.send)
			}
		case message := <-h.broadcast:
			conns := []*WSConnection{}
			switch {
			case len(message.toIDs) > 0:
				for _, id := range message.toIDs {
					if c, ok := h.conns[id]; ok {
						conns = append(conns, c)
					}
				}
			default:
				for _, c := range h.conns {
					conns = append(conns, c)
				}
			}
			for _, conn := range conns {
				select {
				case conn.send <- message.msg:
				default:
					close(conn.send)
					delete(h.conns, conn.ID)
				}
			}
		}
	}
}
