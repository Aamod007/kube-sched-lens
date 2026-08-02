// Package api serves the REST + WebSocket interface.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Aamod007/kube-sched-lens/internal/diagnose"
	"github.com/Aamod007/kube-sched-lens/internal/watcher"
)

var upgrader = websocket.Upgrader{
	// Local tool: allow all origins.
	CheckOrigin: func(*http.Request) bool { return true },
}

// Handler returns the HTTP handler serving all endpoints.
func Handler(state *watcher.State) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/pending-pods", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, orEmpty(diagnose.PendingPods(state, unixNow)))
	})

	mux.HandleFunc("GET /api/pods/{ns}/{name}/diagnosis", func(w http.ResponseWriter, r *http.Request) {
		pod := state.Pod(r.PathValue("ns"), r.PathValue("name"))
		if pod == nil {
			http.Error(w, `{"error":"pod not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, diagnose.Diagnose(state, pod))
	})

	mux.HandleFunc("GET /api/capacity", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, orEmpty(diagnose.Capacity(state)))
	})

	mux.HandleFunc("GET /api/watch", func(w http.ResponseWriter, r *http.Request) {
		serveWatch(state, w, r)
	})

	return cors(mux)
}

func unixNow() int64 { return time.Now().Unix() }

// orEmpty turns a nil slice into an empty one so JSON is [] not null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type wsMessage struct {
	Type string `json:"type"` // "pending-pods" | "capacity"
	Data any    `json:"data"`
}

// serveWatch pushes state snapshots to the client on every (debounced) state
// change, plus an initial snapshot on connect.
func serveWatch(state *watcher.State, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// Drain client messages so pings/close frames are processed.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	push := func() bool {
		for _, msg := range []wsMessage{
			{Type: "pending-pods", Data: orEmpty(diagnose.PendingPods(state, unixNow))},
			{Type: "capacity", Data: orEmpty(diagnose.Capacity(state))},
		} {
			if err := conn.WriteJSON(msg); err != nil {
				return false
			}
		}
		return true
	}

	if !push() {
		return
	}

	changes := state.Subscribe()
	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			debounce.Reset(500 * time.Millisecond)
		case <-debounce.C:
			if !push() {
				return
			}
		}
	}
}
