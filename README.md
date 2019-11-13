# chascale-server


## Running in kubenetes
1. Start a statefulset

```
kubectl apply -f config/chascale-server-statefulset.yaml
```
1. Expose the statfulset app through a loadbalancer

```
kubectl apply -f config/chascale-server-service.yaml
```

1. Test with client 

```
bazel run examples/go-client -- --addr=localhost:8080
```
