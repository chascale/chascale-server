package server

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/chascale/chascale-server/data"
	//"github.com/google/uuid"
	"github.com/hashicorp/memberlist"
)

// Hub maintains the set of active conns and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	conns *sync.Map

	// Inbound messages from the clients.
	broadcast chan data.Message

	// Inbound message from the peers
	peercast chan clientChangeMessage

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

	toClientMsgBuf map[string][]data.Message

	memberlistCfg *memberlist.Config
}

// NewHub creates a new hub with seeds and peerPort.
func NewHub(seeds []string, peerPort int) *Hub {
	return &Hub{
		broadcast:      make(chan data.Message, 256),
		peercast:       make(chan clientChangeMessage, 256),
		register:       make(chan *WSConnection),
		unregister:     make(chan *WSConnection),
		conns:          new(sync.Map),
		seeds:          seeds,
		peerPort:       peerPort,
		toClientMsgBuf: map[string][]data.Message{},
	}
}

// Run starts the hub.
func (h *Hub) Run() {
	add := make(chan string, 256)
	remove := make(chan string, 256)
	go h.syncWithPeers(add, remove)
	if err := h.joinCluster(); err != nil {
		panic(fmt.Sprintf("node failed to join the cluster: %v", err))
	}
	for {
		select {
		case conn := <-h.register:
			// TODO: How the new peers will be notified about the new client add.
			h.conns.Store(conn.ID, conn)
			add <- conn.ID
		case conn := <-h.unregister:
			if _, ok := h.conns.Load(conn.ID); ok {
				h.conns.Delete(conn.ID)
				close(conn.send)
				remove <- conn.ID
			}
		case m := <-h.broadcast:
			for _, id := range m.To {
				conn, ok := h.conns.Load(id)
				if !ok {
					log.Printf("[ERROR] chascale:notfound failed to find the connection to forward the message: %v", m)
					// Buffer the messages until the clients get connected.
					if _, bok := h.toClientMsgBuf[id]; !bok {
						h.toClientMsgBuf[id] = []data.Message{}
					}
					h.toClientMsgBuf[id] = append(h.toClientMsgBuf[id], m)
					continue
				}
				switch c := conn.(type) {
				case *WSConnection:
					log.Printf("[DEBUG] chascale:wsconnection: message %v, current node %s", m, h.memberlistCfg.Name)
					select {
					case c.send <- m:
					default:
						h.unregister <- c
					}
				case string:
					log.Printf("[DEBUG] chascale:node: message intended for peer %v,  %v", c, m)
					node := h.nodeByName(c)
					if node != nil {
						// Override to list node to flood the meesages.
						m.To = []string{id}
						m.OrigNodeName = h.memberlistCfg.Name
						b, err := json.Marshal(m)
						if err != nil {
							log.Printf("[DEBUG] chascale: failed to marshal %v to json: %v", m, err)
						}
						if err := h.peers.SendReliable(node, b); err != nil {
							log.Printf("[ERROR] chascale:node: failed to send to node %v; message %v", c, m)
						}
					}
				default:
					log.Printf("[ERROR] chascale:c failted to find the connection type")
				}
			}
		case m := <-h.peercast:
			go func(h *Hub, m clientChangeMessage) {
				switch m.Op {
				case "Add":
					if m.NodeName != h.memberlistCfg.Name {
						for _, cid := range m.ClientIDs {
							v, _ := h.conns.Load(cid)
							switch v.(type) {
							case *WSConnection:
								continue
							default:
								h.conns.Store(cid, m.NodeName)
							}
						}
					}
				case "Remove":
					for _, cid := range m.ClientIDs {
						v, _ := h.conns.Load(cid)
						switch v.(type) {
						case string:
							h.conns.Delete(cid)
						}
					}
				}
			}(h, m)
		}
	}
}

func (h *Hub) nodeByName(name string) *memberlist.Node {
	for _, node := range h.peers.Members() {
		if node.Name == name {
			return node
		}
	}
	return nil
}

func (h *Hub) syncWithPeers(add, remove chan string) {
	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case id := <-add:
			m := clientChangeMessage{ClientIDs: []string{id}, NodeName: h.memberlistCfg.Name, Op: "Add"}
			if b, err := json.Marshal(&m); err == nil {
				for _, node := range h.peers.Members() {
					h.peers.SendReliable(node, b)
				}
			}
		case id := <-remove:
			m := clientChangeMessage{ClientIDs: []string{id}, NodeName: h.memberlistCfg.Name, Op: "Remove"}
			if b, err := json.Marshal(&m); err == nil {
				for _, node := range h.peers.Members() {
					h.peers.SendReliable(node, b)
				}
			}
		case <-ticker.C:
			ids := []string{}
			h.conns.Range(func(k, v interface{}) bool {
				switch v.(type) {
				case *WSConnection:
					return false
				case string:
					ids = append(ids, k.(string))
					return true
				}
				return false
			})
			if len(ids) > 0 {
				m := clientChangeMessage{ClientIDs: ids, NodeName: h.memberlistCfg.Name, Op: "Add"}
				if b, err := json.Marshal(&m); err == nil {
					for _, node := range h.peers.Members() {
						h.peers.SendReliable(node, b)
					}
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
	// cfg.Name = uuid.New().String()

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
	h.memberlistCfg = cfg
	return nil
}

type clientChangeMessage struct {
	ClientIDs []string `json:"clientIds"`
	NodeName  string   `json:"nodeName"`
	Op        string   `json:"op"`
}

type clusterDelegate struct {
	h  *Hub
	bc *memberlist.TransmitLimitedQueue
}

func (d *clusterDelegate) NotifyMsg(b []byte) {
	go func(h *Hub, b []byte) {
		m1 := clientChangeMessage{}
		if err := json.Unmarshal(b, &m1); err == nil && !reflect.DeepEqual(m1, clientChangeMessage{}) {
			log.Printf("[DEBUG] chascale: NotifyMsg m1: %v", m1)
			h.peercast <- m1
			return
		}
		m2 := data.Message{}
		if err := json.Unmarshal(b, &m2); err == nil && !reflect.DeepEqual(m2, data.Message{}) {
			log.Printf("[DEBUG] chascale: NotifyMsg m2: %v+", m2)
			h.broadcast <- m2
			return
		}

	}(d.h, b)
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
