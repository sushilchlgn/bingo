package main

import (
	"log"
	"math/rand"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // demo: allow any origin
}

// clientMsg is the shape of any message a client sends us.
type clientMsg struct {
	Type string `json:"type"`
}

func (h *Hub) handleWS(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("room")
	name := r.URL.Query().Get("name")
	if code == "" || name == "" {
		http.Error(w, "room and name are required", http.StatusBadRequest)
		return
	}

	room, ok := h.getRoom(code)
	if !ok {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}

	room.mu.Lock()
	isHost := len(room.Players) == 0 // first person into the room is host
	player := &Player{
		ID:     newPlayerID(),
		Name:   name,
		Conn:   conn,
		Card:   generateCard(),
		IsHost: isHost,
	}
	room.Players[player.ID] = player
	room.mu.Unlock()

	// Tell the new player who they are and hand them their card.
	player.send(map[string]interface{}{
		"type":     "welcome",
		"playerId": player.ID,
		"isHost":   isHost,
		"roomCode": code,
		"card":     player.Card,
	})

	room.mu.Lock()
	room.broadcastRoomState()
	// Catch the new player up on anything already called, in case they
	// joined mid-game or reconnected.
	if len(room.Order) > 0 {
		player.send(map[string]interface{}{
			"type":          "call_history",
			"calledNumbers": room.Order,
		})
	}
	room.mu.Unlock()

	defer func() {
		conn.Close()
		room.mu.Lock()
		wasHost := player.IsHost
		delete(room.Players, player.ID)
		empty := len(room.Players) == 0
		if wasHost && !empty {
			// Hand the host role to whoever's been around longest that we
			// still have a handle on (map order is random, but any player
			// is a reasonable fallback for a casual game).
			for _, p := range room.Players {
				p.IsHost = true
				p.send(map[string]interface{}{"type": "promoted_to_host"})
				break
			}
		}
		if !empty {
			room.broadcastRoomState()
		}
		room.mu.Unlock()
		if empty {
			h.deleteRoom(code)
		}
	}()

	for {
		var msg clientMsg
		if err := conn.ReadJSON(&msg); err != nil {
			break // client disconnected or sent garbage; either way, stop
		}
		h.handleMessage(room, player, msg.Type)
	}
}

func (h *Hub) handleMessage(room *Room, player *Player, msgType string) {
	switch msgType {
	case "start_game":
		room.mu.Lock()
		if player.IsHost && room.Status == StatusWaiting {
			room.Status = StatusPlaying
			room.broadcastRoomState()
		}
		room.mu.Unlock()

	case "call_number":
		room.mu.Lock()
		if !player.IsHost || room.Status != StatusPlaying {
			room.mu.Unlock()
			return
		}
		remaining := make([]int, 0, 75)
		for n := 1; n <= 75; n++ {
			if !room.Called[n] {
				remaining = append(remaining, n)
			}
		}
		if len(remaining) == 0 {
			room.mu.Unlock()
			return
		}
		next := remaining[rand.Intn(len(remaining))]
		room.Called[next] = true
		room.Order = append(room.Order, next)
		room.broadcast(map[string]interface{}{
			"type":          "number_called",
			"number":        next,
			"calledNumbers": room.Order,
		})
		room.mu.Unlock()

	case "claim_bingo":
		room.mu.Lock()
		if room.Status != StatusPlaying {
			room.mu.Unlock()
			return
		}
		valid := player.Card.hasBingo(room.Called)
		if valid {
			room.Status = StatusFinished
			room.Winner = player.ID
			room.broadcast(map[string]interface{}{
				"type":       "game_over",
				"winnerId":   player.ID,
				"winnerName": player.Name,
			})
		} else {
			player.send(map[string]interface{}{
				"type":    "bingo_result",
				"valid":   false,
				"message": "Not a bingo yet — keep playing.",
			})
		}
		room.mu.Unlock()
	}
}
