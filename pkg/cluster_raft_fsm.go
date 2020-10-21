package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

const (
	opTypeSessionCreate = "session_create"
	opTypeSessionRemove = "session_remove"
	opTypeNodeDown      = "node_down"
)

type op struct {
	Type      string
	SessionID string
	NodeID    string
}

type fsm struct {
	mutex sync.Mutex

	// [session-id]nodeid
	sessions map[string]string
}

// Apply applies a Raft log entry to the key-value store.
func (fsm *fsm) Apply(logEntry *raft.Log) interface{} {
	var op op
	if err := json.Unmarshal(logEntry.Data, &op); err != nil {
		panic("Failed unmarshaling Raft log entry. This is a bug.")
	}

	switch op.Type {
	case opTypeSessionCreate:
		fsm.mutex.Lock()
		defer fsm.mutex.Unlock()

		fsm.sessions[op.SessionID] = op.NodeID

		return nil
	case opTypeSessionRemove:
		fsm.mutex.Lock()
		defer fsm.mutex.Unlock()

		return nil

	case opTypeNodeDown:
		fsm.mutex.Lock()
		defer fsm.mutex.Unlock()

		// Clean up all sessions from a down node
		// in the future we can migrate sessions / be smarter about this, especially if a node is draining
		for sessionID, nodeID := range fsm.sessions {
			if nodeID == op.SessionID {
				delete(fsm.sessions, sessionID)
			}
		}

		return nil
	default:
		panic(fmt.Sprintf("Unrecognized event type in Raft log entry: %s. This is a bug.", op.Type))
	}
}

func (fsm *fsm) Snapshot() (raft.FSMSnapshot, error) {
	fsm.mutex.Lock()
	defer fsm.mutex.Unlock()

	return &fsmSnapshot{sessions: fsm.sessions}, nil
}

// Restore stores the key-value store to a previous state.
func (fsm *fsm) Restore(serialized io.ReadCloser) error {
	var snapshot fsmSnapshot
	if err := json.NewDecoder(serialized).Decode(&snapshot); err != nil {
		return err
	}

	fsm.sessions = snapshot.sessions
	return nil
}
