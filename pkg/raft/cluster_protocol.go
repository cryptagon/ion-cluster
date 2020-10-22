package cluster

import "github.com/pion/webrtc/v3"

const (
	messageTypeJoin    = "join"
	messageTypeOffer   = "offer"
	messageTypeAnswer  = "answer"
	messageTypeTrickle = "trickle"
)

// message is a struct that is sent from peer to peer
type message struct {
	Type    string
	Payload []byte
}

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
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}
