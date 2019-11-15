# chascale-server


## Build the container

```
blaze run :bundle -- --norun
```

It will publish the container image **chascale-sever:latest** to local docker images

## Running in kubenetes
1. Start a statefulset

```
kubectl apply -f config/chascale-server-statefulset.yaml
```
1. Expose the statfulset app through a loadbalancer

```
kubectl apply -f config/chascale-server-service.yaml
```

1. Test with client (take 3 terminals)

- Terminal-1

```
bazel run examples/go-client:go-client -- --addr=localhost:8080 --client_id=c1 --to_id=c2
```

- Terminal-2

```
bazel run examples/go-client:go-client -- --addr=localhost:8080 --client_id=c2 --to_id=c3
```

- Terminal-3

```
bazel run examples/go-client:go-client -- --addr=localhost:8080 --client_id=c3 --to_id=c1
```

