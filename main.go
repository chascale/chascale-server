//
//                +----------+     +----------+
// client1 -------|          |     |          |------- client3
//                |   Srv1   |-----|   Srv2   |
//                |          |     |          |
// client2 -------|          |     |          |------- client4
//                +----------+     +----------+
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/chascale/chascale-server/k8s"
	"github.com/chascale/chascale-server/server"
)

var (
	addr          = flag.String("addr", ":8080", "http service address")
	interNodePort = flag.Int("inter_node_port", 8081, "inter node port to communicate between nodes")
	k8sNamespace  = flag.String("k8s_namespace", "default", "k8s namespace")
	k8sSvcName    = flag.String("k8s_service_name", "chascale-server", "k8s service name")
)

func main() {
	ctx := context.Background()
	flag.Parse()
	start(ctx, 0)
}

func start(ctx context.Context, c int) {
	var (
		clusterSeeds []string
		err          error
	)

	for i := 0; i < 10 && len(clusterSeeds) < 3 && err == nil; i++ {
		clusterSeeds, err = k8s.GetEndpoints(ctx, *k8sNamespace, *k8sSvcName)
		if err != nil {
			log.Fatalf("[DEBUG] chascale: failed to find cluster seed nodes: %v", err)
		}
		log.Printf("[DEBUG] chascale: nodes in the cluster: %v", clusterSeeds)
		if len(clusterSeeds) < 3 {
			log.Printf("[DEBUG] chascale: sleeping 10s...")
			time.Sleep(10 * time.Second)
		}
	}
	if err != nil {
		log.Fatalf("[DEBUG] chascale: falied to find cluester seed nodes.")
	}
	hub := server.NewHub(clusterSeeds, *interNodePort)
	go hub.Run()
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		server.ServeWS(hub, w, r)
	})

	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal("[DEBUG] chascale: ListenAndServe: ", err)
	}
}
