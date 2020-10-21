package cluster

import "time"

type opCreateSession struct {
	ClientID  string
	Timestamp time.Time
	SessionID string
}

type opAssignSession struct {
	Timestamp time.Time
	SessionID string
	NodeID    string
}

type opRemoveSession struct {
	Timestamp time.Time
	SessionID string
}
