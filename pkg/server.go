package cluster

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/koding/websocketproxy"

	"github.com/pion/ion-cluster/pkg/sfu"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"

	// pprof
	_ "net/http/pprof"
)

// Signal is the grpc/http/websocket signaling server
type Signal struct {
	c       coordinator
	errChan chan error

	config SignalConfig
}

// NewSignal creates a signaling server
func NewSignal(c coordinator, conf SignalConfig) (*Signal, chan error) {
	e := make(chan error)
	w := &Signal{
		c:       c,
		errChan: e,
		config:  conf,
	}
	return w, e
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
				log.Error(err, "error authenticating token")
				http.Error(w, "Invalid Token", http.StatusForbidden)
				return
			}

			log.Info("valid token with claims", "token", token)
			if token.SID != sid {
				log.Error(err, "invalid claims for session", "sessionID", sid)
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
				log.Error(err, "error parsing backend url to proxy websocket")
				return
			}
			proxy := websocketproxy.NewProxy(url)
			proxy.Upgrader = &upgrader

			log.Info("starting proxy for session", "sessionID", meta.SessionID, "nodeID", meta.NodeID, "endpoint", endpoint)
			prometheusGaugeProxyClients.Inc()
			proxy.ServeHTTP(w, r)
			prometheusGaugeProxyClients.Dec()
			log.Info("closed proxy for session", "sessionID", meta.SessionID, "nodeID", meta.NodeID, "endpoint", endpoint)
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
			"",
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

	var err error
	if s.config.Key != "" && s.config.Cert != "" {
		log.Info("Started JSONRPC Server (https)", "listen", s.config.HTTPAddr)
		err = http.ListenAndServeTLS(s.config.HTTPAddr, s.config.Cert, s.config.Key, nil)
	} else {
		log.Info("Started JSONRPC Server", "listen", s.config.HTTPAddr)
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
