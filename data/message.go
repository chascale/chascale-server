package data

import "encoding/json"

// Message is the type between the clients, server nodes.
type Message struct {
	// To is a list of keys/IDs.
	To []string `json:"to"`
	// From is the key/ID the message from.
	From string `json:"from"`
	// Payload is the actual message send between clients, server nodes.
	Payload []byte `json:"payload"`
}

func (m Message) JSON() ([]byte, error) {
	return json.Marshal(m)
}

func JSONToMessage(b []byte) (*Message, error) {
	m := &Message{}
	if err := json.Unmarshal(b, m); err != nil {
		return nil, err
	}
	return m, nil
}
