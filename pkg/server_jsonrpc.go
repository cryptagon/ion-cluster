package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/webrtc/v3"
	"github.com/sourcegraph/jsonrpc2"
)

// Join message sent when initializing a peer connection
type Join struct {
	Sid   string                    `json:"sid"`
	Offer webrtc.SessionDescription `json:"offer"`
}

// Negotiation message sent when renegotiating the peer connection
type Negotiation struct {
	Desc webrtc.SessionDescription `json:"desc"`
}

// Trickle message sent when renegotiating the peer connection
type Trickle struct {
	Target    int                     `json:"target"`
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

type JSONSignal struct {
	mu sync.Mutex
	c  coordinator
	*sfu.Peer
}

// Handle incoming RPC call events like join, answer, offer and trickle
func (p *JSONSignal) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()

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

		meta, err := p.c.getOrCreateSession(join.Sid)
		if err != nil {
			replyError(err)
			break
		}

		if meta.Redirect {
			payload, _ := json.Marshal(meta)
			// session exists on other node, let client know
			_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{
				Code:    302,
				Message: string(payload),
			})
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
		p.OnIceCandidate = func(candidate *webrtc.ICECandidateInit, target int) {
			if err := conn.Notify(ctx, "trickle", Trickle{
				Candidate: *candidate,
				Target:    target,
			}); err != nil {
				log.Errorf("error sending ice candidate %s", err)
			}
		}
		p.OnICEConnectionStateChange = func(s webrtc.ICEConnectionState) {
			if s == webrtc.ICEConnectionStateFailed {
				log.Infof("peer ice failed, closing socket")
				conn.Close()
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

		err = p.Trickle(trickle.Candidate, trickle.Target)
		if err != nil {
			replyError(err)
		}

	case "ping":
		_ = conn.Reply(ctx, req.ID, "pong")
		break
	}
}
