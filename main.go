package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	hub := newHub()

	mux := http.NewServeMux()

	// POST /api/rooms -> {"code": "AB3XZ"}. Call this once to create a room,
	// then have the creator connect to /ws?room=CODE&name=... — the first
	// socket connection into a room becomes its host.
	mux.HandleFunc("/api/rooms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		room := hub.createRoom()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"code": room.Code})
	})

	mux.HandleFunc("/ws", hub.handleWS)

	mux.Handle("/", http.FileServer(http.Dir("static")))

	addr := ":8080"
	log.Printf("bingo server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
