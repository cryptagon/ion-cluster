package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/ion-sfu/pkg/log"
	"github.com/pion/webrtc/v3"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
)

type jsonPeer struct {
	raft Raft
	sfu.Peer
}

// Handle incoming RPC call events like join, answer, offer and trickle
func (p *jsonPeer) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	replyError := func(err error) {
		_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{
			Code:    500,
			Message: fmt.Sprintf("%s", err),
		})
	}

	switch req.Method {
	case "join":
		var join Join
		err := json.Unmarshal(*req.Params, &join)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		answer, err := p.Join(join.Sid, join.Offer)
		if err != nil {
			replyError(err)
			break
		}

		p.OnOffer = func(offer *webrtc.SessionDescription) {
			if err := conn.Notify(ctx, "offer", offer); err != nil {
				log.Errorf("error sending offer %s", err)
			}

		}
		p.OnIceCandidate = func(candidate *webrtc.ICECandidateInit) {
			if err := conn.Notify(ctx, "trickle", candidate); err != nil {
				log.Errorf("error sending ice candidate %s", err)
			}
		}

		_ = conn.Reply(ctx, req.ID, answer)

	case "offer":
		var negotiation Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		answer, err := p.Answer(negotiation.Desc)
		if err != nil {
			replyError(err)
			break
		}
		_ = conn.Reply(ctx, req.ID, answer)

	case "answer":
		var negotiation Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		err = p.SetRemoteDescription(negotiation.Desc)
		if err != nil {
			replyError(err)
		}

	case "trickle":
		var trickle Trickle
		err := json.Unmarshal(*req.Params, &trickle)
		if err != nil {
			log.Errorf("connect: error parsing candidate: %v", err)
			replyError(err)
			break
		}

		err = p.Trickle(trickle.Candidate)
		if err != nil {
			replyError(err)
		}
	}
}

// WebsocketConfig params for http listener / websocket server
type WebsocketConfig struct {
	Key  string
	Cert string
	Addr string
}

// WebsocketServer hosts an embedded websocket signaling server
type WebsocketServer struct {
	sfu     *sfu.SFU
	errChan chan error

	config WebsocketConfig
}

// NewWebsocketServer creates a websocket signaling server
func NewWebsocketServer(s *sfu.SFU, conf WebsocketConfig) (*WebsocketServer, chan error) {
	e := make(chan error)
	w := &WebsocketServer{
		sfu:     s,
		errChan: e,
		config:  conf,
	}
	return w, e
}

// Run listens for incoming websocket signaling requests
func (s *WebsocketServer) Run() {
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

		p := jsonPeer{
			sfu.NewPeer(s.sfu),
		}
		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), &p)
		<-jc.DisconnectNotify()
	}))

	var err error
	if s.config.Key != "" && s.config.Cert != "" {
		log.Infof("Listening at https://[%s]", s.config.Addr)
		err = http.ListenAndServeTLS(s.config.Addr, s.config.Cert, s.config.Key, nil)
	} else {
		log.Infof("Listening at http://[%s]", s.config.Addr)
		err = http.ListenAndServe(s.config.Addr, nil)
	}

	if err != nil {
		s.errChan <- err
	}
}
