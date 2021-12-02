package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/webrtc/v3"
	"github.com/sourcegraph/jsonrpc2"
)

// Join message sent when initializing a peer connection
type Join struct {
	SID   string                    `json:"sid"`
	UID   string                    `json:"uid"`
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

// Presence contains metadata for every peerID (can be used for mapping userIDs to streams, stream types, etc)
type Presence struct {
	Revision uint64                 `json:"revision"`
	Meta     map[string]interface{} `json:"meta"`
}

type JSONSignal struct {
	mu sync.Mutex
	c  coordinator
	*sfu.PeerLocal

	sid string
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
			log.Error(err, "connect: error parsing offer")
			replyError(err)
			break
		}

		meta, err := p.c.getOrCreateSession(join.SID)
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

		err = p.Join(join.SID, join.UID)
		if err != nil {
			replyError(err)
			break
		}

		answer, err := p.Answer(join.Offer)
		if err != nil {
			replyError(err)
			break
		}

		p.OnOffer = func(offer *webrtc.SessionDescription) {
			if err := conn.Notify(ctx, "offer", offer); err != nil {
				log.Error(err, "error sending offer")
			}
		}
		p.OnIceCandidate = func(candidate *webrtc.ICECandidateInit, target int) {
			if err := conn.Notify(ctx, "trickle", Trickle{
				Candidate: *candidate,
				Target:    target,
			}); err != nil {
				log.Error(err, "error sending ice candidate")
			}
		}
		p.OnICEConnectionStateChange = func(s webrtc.ICEConnectionState) {
			if s == webrtc.ICEConnectionStateFailed || s == webrtc.ICEConnectionStateClosed {
				log.Info("peer ice failed/closed, closing peer and websocket")
				p.Close()
				conn.Close()
			}
		}

		s, _ := p.c.GetSession(join.SID)
		session := s.(*Session)

		listen := make(chan Broadcast, 32)
		session.BroadcastAddListener(p.ID(), listen)

		p.sid = join.SID

		stop := conn.DisconnectNotify()
		go func() {
			log.Info("peer starting broadcast listener")
			for {
				select {
				case msg := <-listen:
					log.Info("peer got broadcast", "id", p.ID(), "msg", msg)
					conn.Notify(ctx, msg.method, msg.params)
				case <-stop:
					session.BroadcastRemoveListener(p.ID())
					session.UpdatePresenceMetaForPeer(p.ID(), nil)
					log.Info("peer broadcast listener closed", "id", p.ID())
					return
				}
			}
		}()

		_ = conn.Reply(ctx, req.ID, answer)

	case "offer":
		var negotiation Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		if err != nil {
			log.Error(err, "connect: error parsing offer")
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
			log.Error(err, "connect: error parsing offer")
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
			log.Error(err, "connect: error parsing candidate")
			replyError(err)
			break
		}

		err = p.Trickle(trickle.Candidate, trickle.Target)
		if err != nil {
			replyError(err)
		}

	case "presence_set":
		if p.sid == "" {
			replyError(fmt.Errorf("cannot update presence for peer not in any session"))
			break
		}
		var meta map[string]interface{}
		err := json.Unmarshal(*req.Params, &meta)
		if err != nil {
			log.Error(err, "presence: error parsing metadata")
			replyError(err)
			break
		}

		s, _ := p.c.GetSession(p.sid)
		session := s.(*Session)
		session.UpdatePresenceMetaForPeer(p.ID(), meta)

	case "ping":
		_ = conn.Reply(ctx, req.ID, "pong")
		break
	}
}
