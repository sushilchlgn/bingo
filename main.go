package main

import (
	"encoding/json"
	"log"
	"net/http"
)

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
			http.Dir("static"),
		),
	)

	addr := ":8080"

	log.Printf(
		"Bingo server listening on %s",
		addr,
	)

	if err := http.ListenAndServe(
		addr,
		mux,
	); err != nil {
		log.Fatal(err)
	}
}