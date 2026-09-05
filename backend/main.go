package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(origin, r) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	hub := newHub()

	mux := http.NewServeMux()

	// ------------------------------------------------------------
	// CREATE ROOM
	// ------------------------------------------------------------

	mux.HandleFunc(
		"/api/rooms",
		func(w http.ResponseWriter, r *http.Request) {

			if r.Method != http.MethodPost {
				http.Error(
					w,
					"method not allowed",
					http.StatusMethodNotAllowed,
				)

				return
			}

			room := hub.createRoom()

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			_ = json.NewEncoder(w).Encode(
				map[string]string{
					"code": room.Code,
				},
			)
		},
	)

	// ------------------------------------------------------------
	// WEBSOCKET
	// ------------------------------------------------------------

	mux.HandleFunc(
		"/ws",
		hub.handleWS,
	)

	// ------------------------------------------------------------
	// STATIC FRONTEND
	// ------------------------------------------------------------

	mux.Handle(
		"/",
		http.FileServer(
			http.Dir("../frontend"),
		),
	)

	addr := ":8080"

	log.Printf(
		"Bingo server listening on %s",
		addr,
	)

	if err := http.ListenAndServe(
		addr,
		withCORS(mux),
	); err != nil {
		log.Fatal(err)
	}
}