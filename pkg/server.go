package cluster

import (
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	pb "github.com/pion/ion-sfu/cmd/signal/grpc/proto"

	grpcServer "github.com/pion/ion-sfu/cmd/signal/grpc/server"
	jsonrpcServer "github.com/pion/ion-sfu/cmd/signal/json-rpc/server"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/ion-sfu/pkg/log"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
	"google.golang.org/grpc"

	// pprof
	_ "net/http/pprof"
)

// SignalConfig params for http listener / grpc / websocket server
type SignalConfig struct {
	Key      string
	Cert     string
	HTTPAddr string
	GRPCAddr string
}

// Signal is the grpc/http/websocket signaling server
type Signal struct {
	sfu     *sfu.SFU
	errChan chan error

	config SignalConfig
}

// NewSignal creates a signaling server
func NewSignal(s *sfu.SFU, conf SignalConfig) (*Signal, chan error) {
	e := make(chan error)
	w := &Signal{
		sfu:     s,
		errChan: e,
		config:  conf,
	}
	return w, e
}

// ServeWebsocket listens for incoming websocket signaling requests
func (s *Signal) ServeWebsocket() {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	http.Handle("/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer c.Close()

		p := jsonrpcServer.NewJSONSignal(sfu.NewPeer(s.sfu))
		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), p)
		<-jc.DisconnectNotify()
	}))

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

// ServeGRPC serve grpc
func (s *Signal) ServeGRPC() {
	l, err := net.Listen("tcp", s.config.GRPCAddr)
	if err != nil {
		s.errChan <- err
		return
	}

	gs := grpc.NewServer()
	inst := grpcServer.GRPCSignal{SFU: s.sfu}
	pb.RegisterSFUService(gs, &pb.SFUService{
		Signal: inst.Signal,
	})

	log.Infof("GRPC Listening at %s", s.config.GRPCAddr)
	if err := gs.Serve(l); err != nil {
		log.Errorf("err=%v", err)
		s.errChan <- err
		return
	}
}
