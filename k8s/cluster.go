package k8s

import (
	"context"

	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
)

type endpoints struct {
	IPs []string `json:"ips"`
}

// GetEndpoints searches the endpoints of a service returning a list of IP addresses.
//export GetEndpoints
func GetEndpoints(ctx context.Context, ns, svc string) ([]string, error) {
	client, err := k8s.NewInClusterClient()
	if err != nil {
		return nil, err
	}

	ips := make([]string, 0)

	var endpoints corev1.Endpoints
	err = client.Get(ctx, ns, svc, &endpoints)
	if err != nil {
		return nil, err
	}

	for _, endpoint := range endpoints.Subsets {
		for _, address := range endpoint.Addresses {
			ips = append(ips, *address.Ip)
		}
	}

	return ips, nil
}
