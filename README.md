# ION Cluster

ION Cluster is a clusterable and horizontally scalable SFU build on [ion-sfu](https://github.com/pion/ion-sfu).  It coordinates sessions between nodes, and provides a signal interface over a jsonrpc2 websocket.

It supports operating as a single node with no dependencies, or in clustered mode using etcd.

## Dependencies
#### OSX
```
brew install pkg-config gstreamer gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly
# optionally
brew install gst-libav
```


#### Ubuntu 

```
apt install -y pkg-config gstreamer1.0 gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly
# optionally
apt install -y gstreamer1.0-libav #supports more decoders for cli
```

## Build
```
➜  ion-cluster git:(master) ✗ go build 
➜  ion-cluster git:(master) ✗ ./ion-cluster
A batteries included and scalable implementation of ion-sfu

Usage:
  ion-cluster [command]

Available Commands:
  client      Connect to an ion-cluster server as a client
  help        Help about any command
  server      start an ion-cluster server node

Flags:
      --config string   config file (default is $HOME/.cobra.yaml)
  -h, --help            help for ion-cluster
      --viper           use Viper for configuration (default true)

Use "ion-cluster [command] --help" for more information about a command.
```

## Server
### Run in local mode

```
./ion-cluster server -c cfgs/local.toml
```


### Run in etcd mode

```
docker-compose up -d etcd

./ion-cluster server -c config.toml       # Listens on :7000 
./ion-cluster server -c config2.toml      # Listens on :7001
```


## Client 
IonCluster can act as a client and publish streams to a remote cluster

```
➜  ion-cluster git:(client) ✗ ./ion-cluster client --help
Connect to an ion-cluster server as a client

Usage:
  ion-cluster client [flags] <MEDIA FILE TO UPLOAD>

Flags:
  -h, --help           help for client
  -s, --sid string     session id to join (default "test-session")
  -t, --token string   jwt access token
  -u, --url string     sfu host to connect to (default "ws://localhost:7000")
```
