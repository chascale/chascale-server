package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/hashicorp/memberlist"
)

type membersFlag []string

func (m *membersFlag) String() string {
	return fmt.Sprintf("%v", *m)
}
func (m *membersFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

var (
	knownMembers membersFlag

	bindAddr = flag.String("addr", "", "Binding adodress for this process")
	bindPort = flag.Int("port", 0, "Binding port for this process")
)

func main() {
	flag.Var(&knownMembers, "known_members", "list of known members")
	flag.Parse()

	if len(knownMembers) == 0 {
		knownMembers = append(knownMembers, "127.0.0.1")
	}

	cfg := memberlist.DefaultLocalConfig()
	cfg.Name = fmt.Sprintf("%s:%d", *bindAddr, *bindPort)
	cfg.BindAddr = *bindAddr
	cfg.BindPort = *bindPort

	list, err := memberlist.Create(cfg)

	if err != nil {
		panic("Failed to create memberlist: " + err.Error())
	}

	// Join an existing cluster by specifying at least one known member.
	n, err := list.Join([]string(knownMembers))
	if err != nil {
		panic("Failed to join cluster: " + err.Error())
	}

	fmt.Printf("nodes in the list: %d", n)

	for {
		// Ask for members of the cluster
		for _, member := range list.Members() {
			fmt.Printf("Member: %s %s\n", member.Name, member.Addr)
		}
		fmt.Println("----------------------------")
		time.Sleep(10 * time.Second)
	}
}
