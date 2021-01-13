package cluster

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/koding/websocketproxy"

	log "github.com/pion/ion-log"
	"github.com/pion/ion-sfu/pkg/sfu"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"

	// pprof
	_ "net/http/pprof"
)

// NodeState represents the current state of the node
type NodeState string

var (
	// NodeStateInit the node is initializing
	NodeStateInit NodeState = "init"
	// NodeStateReady the node is ready for clients
	NodeStateReady NodeState = "online"
	// NodeStateTerminating the node is draining and preparing to shutdown
	NodeStateTerminating NodeState = "terminating"
)

// Signal is the grpc/http/websocket signaling server
type Signal struct {
	state   NodeState
	c       coordinator
	errChan chan error

	config SignalConfig
}

// NewSignal creates a signaling server
func NewSignal(s *sfu.SFU, c coordinator, conf SignalConfig) (*Signal, chan error) {
	e := make(chan error)
	w := &Signal{
		state:   NodeStateInit,
		c:       c,
		errChan: e,
		config:  conf,
	}
	go w.monitor()
	return w, e
}

func (s *Signal) monitor() {
	t := time.NewTicker(1000 * time.Millisecond)
	for {
		select {
		case <-t.C:
			err := s.c.updateNodeState(s.state, MetricsGetActiveSessions(), MetricsGetActiveClientsCount())
			if err != nil {
				s.errChan <- err
				return
			}
		}
	}
}

// NodeState Updates the state  node
func (s *Signal) NodeState(n NodeState) {
	s.state = n
}

// ServeWebsocket listens for incoming websocket signaling requests
func (s *Signal) ServeWebsocket() {
	r := mux.NewRouter()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	r.Handle("/session/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		sid := vars["id"]

		if s.config.Auth.Enabled {
			token, err := authGetAndValidateToken(s.config.Auth, r)
			if err != nil {
				log.Errorf("error authenticating token => %s", err)
				http.Error(w, "Invalid Token", http.StatusForbidden)
				return
			}

			log.Debugf("valid token with claims %#v", token)
			if token.SID != sid {
				log.Errorf("invalid claims for session %s => %s", sid, err)
				http.Error(w, "Invalid Token", http.StatusForbidden)
				return
			}
		}

		meta, err := s.c.getOrCreateSession(vars["id"])
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if meta.Redirect {
			endpoint := fmt.Sprintf("%v/session/%v", meta.NodeEndpoint, meta.SessionID)
			url, err := url.Parse(endpoint)
			if err != nil {
				log.Errorf("error parsing backend url to proxy websocket")
			}
			proxy := websocketproxy.NewProxy(url)
			proxy.Upgrader = &upgrader
			log.Debugf("starting proxy for session %v -> node %v @ %v", meta.SessionID, meta.NodeID, endpoint)
			prometheusGaugeProxyClients.Inc()
			proxy.ServeHTTP(w, r)
			prometheusGaugeProxyClients.Dec()
			log.Debugf("closed proxy for session %v -> node %v @ %v", meta.SessionID, meta.NodeID, endpoint)
			return
		}

		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer c.Close()

		prometheusGaugeClients.Inc()
		p := JSONSignal{
			sync.Mutex{},
			s.c,
			sfu.NewPeer(s.c),
		}
		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), &p)
		<-jc.DisconnectNotify()
		prometheusGaugeClients.Dec()
	}))

	r.Handle("/metrics", metricsHandler())
	r.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	http.Handle("/", r)
	s.NodeState(NodeStateReady)

	var err error
	if s.config.Key != "" && s.config.Cert != "" {
		log.Infof("Listening at https://[%s]", s.config.HTTPAddr)
		err = http.ListenAndServeTLS(s.config.HTTPAddr, s.config.Cert, s.config.Key, nil)
	} else {
		log.Infof("Listening at http://[%s]", s.config.HTTPAddr)
		err = http.ListenAndServe(s.config.HTTPAddr, nil)
	}

	if err != nil {
		s.errChan <- err
	}
}

// // ServeGRPC serve grpc
// func (s *Signal) ServeGRPC() {
// 	l, err := net.Listen("tcp", s.config.GRPCAddr)
// 	if err != nil {
// 		s.errChan <- err
// 		return
// 	}

// 	gs := grpc.NewServer()
// 	inst := grpcServer.GRPCSignal{SFU: s.sfu}
// 	pb.RegisterSFUService(gs, &pb.SFUService{
// 		Signal: inst.Signal,
// 	})

// 	log.Infof("GRPC Listening at %s", s.config.GRPCAddr)
// 	if err := gs.Serve(l); err != nil {
// 		log.Errorf("err=%v", err)
// 		s.errChan <- err
// 		return
// 	}
// }
