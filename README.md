# ION Cluster

ION Cluster is a clusterable and horizontally scalable SFU build on [ion-sfu](https://github.com/pion/ion-sfu).  It coordinates sessions between nodes, and provides a signal interface over a jsonrpc2 websocket.

It supports operating as a single node with no dependencies, or in clustered mode using etcd.

## Run in local mode

```
go run cmd/main.go -c cfgs/local.toml
```


## Run in etcd mode

```
docker-compose up -d etcd

go run cmd/main.go -c config.toml       # Listens on :7000 
go run cmd/main.go -c config2.toml      # Listens on :7001
```


