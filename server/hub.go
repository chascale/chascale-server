package server

import (
	"fmt"
	"log"

	"github.com/chascale/chascale-server/data"
	"github.com/hashicorp/memberlist"
)

// Hub maintains the set of active conns and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	conns map[string]*WSConnection

	// Inbound messages from the clients.
	broadcast chan data.Message

	// Inbound message from the peers
	peercast chan data.Message

	// Register requests from the clients.
	register chan *WSConnection

	// Unregister requests from clients.
	unregister chan *WSConnection

	// Seed node IPs
	seeds []string

	// memberlist
	peers *memberlist.Memberlist

	// port to listen for peer updates
	peerPort int
}

// NewHub creates a new hub with seeds and peerPort.
func NewHub(seeds []string, peerPort int) *Hub {
	return &Hub{
		broadcast:  make(chan data.Message),
		peercast:   make(chan data.Message, 256),
		register:   make(chan *WSConnection),
		unregister: make(chan *WSConnection),
		conns:      make(map[string]*WSConnection),
		seeds:      seeds,
		peerPort:   peerPort,
	}
}

// Run starts the hub.
func (h *Hub) Run() {
	if err := h.joinCluster(); err != nil {
		panic(fmt.Sprintf("node failed to join the cluster: %v", err))
	}
	for {
		select {
		case conn := <-h.register:
			h.conns[conn.ID] = conn
		case conn := <-h.unregister:
			if _, ok := h.conns[conn.ID]; ok {
				delete(h.conns, conn.ID)
				close(conn.send)
			}
		case m := <-h.broadcast:
			idsInOtherNodes := []string{}
			for _, id := range m.To {
				conn, ok := h.conns[id]
				if !ok {
					idsInOtherNodes = append(idsInOtherNodes, id)
					continue
				}
				select {
				case conn.send <- m:
				default:
					h.unregister <- conn
				}
			}
			// If there are any more ids remaining.
			if len(idsInOtherNodes) > 0 {
				m.To = idsInOtherNodes
				b, err := m.JSON()
				if err != nil {
					log.Printf("[DEBUG] chascale: failed to marshal %v to json: %v", m, err)
				}
				for _, node := range h.peers.Members() {
					h.peers.SendReliable(node, b)
				}
			}
		case m := <-h.peercast:
			for _, id := range m.To {
				conn, ok := h.conns[id]
				if !ok {
					continue
				}
				select {
				case conn.send <- m:
				default:
					h.unregister <- conn
				}
			}
		}
	}
}

func (h *Hub) joinCluster() error {
	if len(h.seeds) == 0 {
		return fmt.Errorf("no seeds to join the cluster")
	}
	d := &clusterDelegate{
		h:  h,
		bc: new(memberlist.TransmitLimitedQueue),
	}
	cfg := memberlist.DefaultLocalConfig()
	cfg.BindPort = h.peerPort
	cfg.Delegate = d

	list, err := memberlist.Create(cfg)
	if err != nil {
		return fmt.Errorf("creating memberlist failed: %v", err)
	}
	if _, err := list.Join(h.seeds); err != nil {
		return fmt.Errorf("failed to join memberlist: %v", err)
	}
	h.peers = list
	return nil
}

type clusterDelegate struct {
	h  *Hub
	bc *memberlist.TransmitLimitedQueue
}

func (d *clusterDelegate) NotifyMsg(b []byte) {
	if m, err := data.JSONToMessage(b); err == nil {
		d.h.peercast <- *m
	}
}

func (d *clusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	return d.bc.GetBroadcasts(overhead, limit)
}

func (d *clusterDelegate) NodeMeta(limit int) []byte {
	// not use, noop
	return []byte("")
}
func (d *clusterDelegate) LocalState(join bool) []byte {
	// not use, noop
	return []byte("")
}
func (d *clusterDelegate) MergeRemoteState(buf []byte, join bool) {
	// not use
}
