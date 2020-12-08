package client

import (
	"context"
	"encoding/json"
	"fmt"

	cluster "github.com/pion/ion-cluster/pkg"
	log "github.com/pion/ion-log"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
)

var (
	errNotConnected = fmt.Errorf("error no connection established")
)

// Signal is the RPC Interface for ion-cluster
type Signal interface {
	Open(url string) (closed <-chan struct{}, err error)
	Close() error

	Join(sid string, offer *webrtc.SessionDescription) (*webrtc.SessionDescription, error)
	Offer(offer *webrtc.SessionDescription) (*webrtc.SessionDescription, error)
	Answer(answer *webrtc.SessionDescription) error
	Trickle(target int, trickle *webrtc.ICECandidateInit) error

	OnNegotiate(func(offer *webrtc.SessionDescription))
	OnTrickle(func(target int, trickle *webrtc.ICECandidateInit))
}

// JSONRPCSignalClient is a websocket jsonrpc2 client for ion-cluster
type JSONRPCSignalClient struct {
	context context.Context
	jc      *jsonrpc2.Conn

	onNegotiate func(jsep *webrtc.SessionDescription)
	onTrickle   func(target int, trickle *webrtc.ICECandidateInit)
}

// NewJSONRPCSignalClient constructor
func NewJSONRPCSignalClient(ctx context.Context) Signal {
	return &JSONRPCSignalClient{context: ctx}
}

// Open connects to the given url
func (c *JSONRPCSignalClient) Open(url string) (<-chan struct{}, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	c.jc = jsonrpc2.NewConn(c.context, websocketjsonrpc2.NewObjectStream(conn), c)
	return c.jc.DisconnectNotify(), nil
}

// Close disconnects the websocket
func (c *JSONRPCSignalClient) Close() error {
	return c.jc.Close()
}

// Join a session id with an sdp offer (returns an sdp answer or error)
func (c *JSONRPCSignalClient) Join(sid string, offer *webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if c.jc == nil {
		return nil, errNotConnected
	}

	log.Debugf("signal client sending join: %v ", sid)
	var answer *webrtc.SessionDescription

	err := c.jc.Call(c.context, "join", &cluster.Join{Sid: sid, Offer: *offer}, &answer)
	if err != nil {
		return nil, err
	}

	return answer, nil
}

// Offer a new sdp to the server (returns an sdp answer)
func (c *JSONRPCSignalClient) Offer(offer *webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if c.jc == nil {
		return nil, errNotConnected
	}

	log.Debugf("signal client sending offer")
	var answer *webrtc.SessionDescription
	err := c.jc.Call(c.context, "offer", &cluster.Negotiation{Desc: *offer}, &answer)
	if err != nil {
		return nil, err
	}

	return answer, nil
}

// Answer an sdp offer that originated from the server
func (c *JSONRPCSignalClient) Answer(answer *webrtc.SessionDescription) error {
	if c.jc == nil {
		return errNotConnected
	}

	log.Debugf("signal client sending answer")
	return c.jc.Notify(c.context, "answer", &cluster.Negotiation{Desc: *answer})
}

// Trickle send ice candiates to the server
func (c *JSONRPCSignalClient) Trickle(target int, trickle *webrtc.ICECandidateInit) error {
	if c.jc == nil {
		return errNotConnected
	}

	log.Debugf("signal client sending trickle ice")
	return c.jc.Notify(c.context, "trickle", &cluster.Trickle{Target: target, Candidate: *trickle})
}

// Handle handles incoming jsonrpc2 messages
func (c *JSONRPCSignalClient) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	switch req.Method {
	case "offer":
		log.Debugf("signal client got offer")
		var offer webrtc.SessionDescription
		err := json.Unmarshal(*req.Params, &offer)
		if err != nil {
			log.Errorf("error parsing offer from server")
			break
		}

		if c.onNegotiate != nil {
			c.onNegotiate(&offer)
		}

	case "trickle":
		log.Debugf("signal client got trickle ice")

		var trickle cluster.Trickle
		err := json.Unmarshal(*req.Params, &trickle)
		if err != nil {
			log.Errorf("error parsing trickle ice from server")
			break
		}

		if c.onTrickle != nil {
			c.onTrickle(trickle.Target, &trickle.Candidate)
		}
	}
}

//OnNegotiate hook a negotiation handler
func (c *JSONRPCSignalClient) OnNegotiate(cb func(offer *webrtc.SessionDescription)) {
	c.onNegotiate = cb
}

//OnTrickle hook a trickle handler
func (c *JSONRPCSignalClient) OnTrickle(cb func(target int, trickle *webrtc.ICECandidateInit)) {
	c.onTrickle = cb
}
