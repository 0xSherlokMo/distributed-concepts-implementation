package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type State int

const (
	StateInitialized State = iota
	StatePreparedToCommit
	StateAborted
	StateCommited
)

func (s State) String() string {
	var stringState string
	switch s {
	case StateInitialized:
		stringState = "Initialized"
	case StatePreparedToCommit:
		stringState = "prepared-to-commit"
	case StateAborted:
		stringState = "aborted"
	case StateCommited:
		stringState = "commited"
	default:
		stringState = "unknown"
	}
	return stringState
}

type SessionMetadata struct {
	State    State
	Reciever chan<- State
}

var sessions = make(map[string]SessionMetadata)

func readableSessions() map[string]string {
	readableSession := make(map[string]string)
	for sessionId, metadata := range sessions {
		readableSession[sessionId] = metadata.State.String()
	}
	return readableSession
}

const (
	TIMEOUT = time.Second * 15
)

func SessionManager(id string, events <-chan State) {
	log.Printf("Started Session %s", id)
	for {
		select {
		case event := <-events:
			metadata := sessions[id]
			log.Printf("Session %s changed from %s, to %s", id, metadata.State, event)
			metadata.State = event
			sessions[id] = metadata
			if event == StateCommited {
				log.Printf("Session %s Finished.", id)
				return
			}
		case <-time.After(TIMEOUT):
			metadata := sessions[id]
			log.Printf("Session %s timedout at state %s", id, metadata.State)
			metadata.State = StateAborted
			sessions[id] = metadata
			return
		}
	}
}

func main() {
	http.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			response, _ := json.Marshal(map[string]any{"error": "wrong status code"})
			w.WriteHeader(http.StatusNotFound)
			w.Write(response)
			return
		}
		id, _ := uuid.NewRandom()
		sessionId := id.String()
		response, _ := json.Marshal(map[string]any{"sessionId": sessionId})
		eventBus := make(chan State, 1)
		sessions[sessionId] = SessionMetadata{
			State:    StateInitialized,
			Reciever: eventBus,
		}
		go SessionManager(sessionId, eventBus)
		w.Write(response)
	})

	http.HandleFunc("/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte{})
			return
		}
		response, _ := json.Marshal(readableSessions())
		w.Write(response)
	})

	http.HandleFunc("/prepare", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte{})
			return
		}
		id := r.Header.Get("x-session-id")
		sessionMetadata, ok := sessions[id]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			response, _ := json.Marshal(map[string]any{"error": "no session id exists with the provided ID"})
			w.Write(response)
			return
		}

		if sessionMetadata.State != StateInitialized {
			w.WriteHeader(http.StatusBadRequest)
			response, _ := json.Marshal(map[string]any{"error": "session cannot be prepared."})
			w.Write(response)
			return
		}

		sessionMetadata.Reciever <- StatePreparedToCommit
		response, _ := json.Marshal(map[string]any{"sessionId": id, "status": StatePreparedToCommit.String()})
		w.Write(response)
	})

	http.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte{})
			return
		}
		id := r.Header.Get("x-session-id")
		sessionMetadata, ok := sessions[id]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			response, _ := json.Marshal(map[string]any{"error": "no session id exists with the provided ID"})
			w.Write(response)
			return
		}

		if sessionMetadata.State != StatePreparedToCommit {
			w.WriteHeader(http.StatusBadRequest)
			response, _ := json.Marshal(map[string]any{"error": "session cannot be commited."})
			w.Write(response)
			return
		}

		sessionMetadata.Reciever <- StateCommited
		response, _ := json.Marshal(map[string]any{"sessionId": id, "status": StateCommited.String()})
		w.Write(response)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
