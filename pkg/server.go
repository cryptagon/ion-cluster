package cluster

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"

	// pprof
	_ "net/http/pprof"
)

// Signal is the grpc/http/websocket signaling server
type Signal struct {
	c       coordinator
	sfu     *sfu.SFU
	errChan chan error

	config SignalConfig
}

// NewSignal creates a signaling server
func NewSignal(s *sfu.SFU, c coordinator, conf SignalConfig) (*Signal, chan error) {
	e := make(chan error)
	w := &Signal{
		sfu:     s,
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

	r.Handle("/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer c.Close()

		p := JSONSignal{
			s.c,
			sfu.NewPeer(s.sfu),
		}
		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), &p)
		<-jc.DisconnectNotify()
	}))

	r.Handle("/session/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		meta, err := s.c.getOrCreateSession(vars["id"])
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		payload, err := json.Marshal(meta)
		if err != nil {
			log.Debugf("error marshaling nodeMeta: %v", err)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	}))

	http.Handle("/", r)

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
