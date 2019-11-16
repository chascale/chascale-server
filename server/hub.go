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
		broadcast:      make(chan data.Message),
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
	if err := h.joinCluster(); err != nil {
		panic(fmt.Sprintf("node failed to join the cluster: %v", err))
	}
	add := make(chan string, 256)
	remove := make(chan string, 256)
	go h.syncWithPeers(add, remove)
	curName := h.memberlistCfg.Name
	for {
		select {
		case conn := <-h.register:
			log.Printf("%s chascale: register %v", curName, conn)
			// TODO: How the new peers will be notified about the new client add.
			h.conns.Store(conn.ID, conn)
			add <- conn.ID
		case conn := <-h.unregister:
			log.Printf("%s chascale: unregister %v", curName, conn)
			if _, ok := h.conns.Load(conn.ID); ok {
				h.conns.Delete(conn.ID)
				close(conn.send)
				remove <- conn.ID
			}
		case m := <-h.broadcast:
			log.Printf("%s chascale: broadcast message %v", curName, m)
			for _, id := range m.To {
				v, _ := h.conns.Load(id)
				switch conn := v.(type) {
				case *WSConnection:
					log.Printf("%s chascale: broadcasting message %v to %v", curName, m, conn)
					select {
					case conn.send <- m:
					default:
						h.unregister <- conn
					}
				case string:
					nodeName := conn
					log.Printf("%s chascale: forwarding message %v to %v", curName, m, nodeName)
					node := h.nodeByName(nodeName)
					if node != nil {
						// Override to list node to flood the meesages.
						m.To = []string{id}
						m.OrigNodeName = h.memberlistCfg.Name
						b, err := json.Marshal(m)
						if err != nil {
							log.Printf("[ERROR] %s chascale: failed to marshal %v to json: %v", curName, m, err)
						}
						if err := h.peers.SendReliable(node, b); err != nil {
							log.Printf("[ERROR] %s chascale:node: failed to send to node %v; message %v", curName, nodeName, m)
						}
					}
				default:
					log.Printf("%s chascale: unable forward/broadcast message %v", curName, m)
				}
			}
		case m := <-h.peercast:
			log.Printf("%s chascale: peer message %v", curName, m)
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
	curName := h.memberlistCfg.Name
	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case id := <-add:
			log.Printf("%s chascale: boradcast add msg of id %v", curName, id)
			m := clientChangeMessage{ClientIDs: []string{id}, NodeName: h.memberlistCfg.Name, Op: "Add"}
			if b, err := json.Marshal(&m); err == nil {
				for _, node := range h.peers.Members() {
					h.peers.SendReliable(node, b)
				}
			}
		case id := <-remove:
			log.Printf("%s chascale: boradcast remove msg of id %v", curName, id)
			m := clientChangeMessage{ClientIDs: []string{id}, NodeName: h.memberlistCfg.Name, Op: "Remove"}
			if b, err := json.Marshal(&m); err == nil {
				for _, node := range h.peers.Members() {
					h.peers.SendReliable(node, b)
				}
			}
		case <-ticker.C:
			log.Printf("%s chascale: boradcast current state %v", curName)
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
			log.Printf("%s chascale: received msg m1 %v", h.memberlistCfg.Name, m1)
			h.peercast <- m1
			return
		}
		m2 := data.Message{}
		if err := json.Unmarshal(b, &m2); err == nil && !reflect.DeepEqual(m2, data.Message{}) {
			log.Printf("%s chascale: received msg m2 %v", h.memberlistCfg.Name, m2)
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
