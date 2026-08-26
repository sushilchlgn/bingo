package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1024
)

// WebSocket upgrader.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,

	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")

		// Non-browser clients.
		if origin == "" {
			return true
		}

		u, err := url.Parse(origin)

		if err != nil {
			return false
		}

		if u.Host != r.Host {
			return false
		}

		requestScheme := "http"

		if r.TLS != nil ||
			strings.EqualFold(
				r.Header.Get("X-Forwarded-Proto"),
				"https",
			) {
			requestScheme = "https"
		}

		return strings.EqualFold(
			u.Scheme,
			requestScheme,
		)
	},
}

// clientMsg is the message sent by the browser.
type clientMsg struct {
	Type string `json:"type"`

	// Used later by gameplay.
	RoundID uint64 `json:"roundId,omitempty"`
}

// normalizeName validates player names.
func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)

	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	if !utf8.ValidString(name) {
		return "", fmt.Errorf("name contains invalid UTF-8")
	}

	if utf8.RuneCountInString(name) > 20 {
		return "", fmt.Errorf(
			"name must be 20 characters or fewer",
		)
	}

	return name, nil
}

// handleWS handles a WebSocket connection.
func (h *Hub) handleWS(
	w http.ResponseWriter,
	r *http.Request,
) {
	code := strings.ToUpper(
		strings.TrimSpace(
			r.URL.Query().Get("room"),
		),
	)

	name, nameErr := normalizeName(
		r.URL.Query().Get("name"),
	)

	if code == "" || nameErr != nil {
		msg := "room and a valid name are required"

		if nameErr != nil {
			msg = nameErr.Error()
		}

		http.Error(
			w,
			msg,
			http.StatusBadRequest,
		)

		return
	}

	room, ok := h.getRoom(code)

	if !ok {
		http.Error(
			w,
			"room not found",
			http.StatusNotFound,
		)

		return
	}

	// ------------------------------------------------------------
	// IMPORTANT:
	// Players can join ONLY while the room is waiting.
	// ------------------------------------------------------------

	room.mu.Lock()

	if room.Status != StatusWaiting {
		room.mu.Unlock()

		http.Error(
			w,
			"game has already started; joining is closed",
			http.StatusConflict,
		)

		return
	}

	if len(room.Players) >= MaxPlayersPerRoom {
		room.mu.Unlock()

		http.Error(
			w,
			"room is full",
			http.StatusConflict,
		)

		return
	}

	room.mu.Unlock()

	// Upgrade to WebSocket.
	conn, err := upgrader.Upgrade(
		w,
		r,
		nil,
	)

	if err != nil {
		log.Printf(
			"upgrade error: %v",
			err,
		)

		return
	}

	conn.SetReadLimit(maxMessageSize)

	// ------------------------------------------------------------
	// Re-check room state after WebSocket upgrade.
	//
	// Another player may have joined or the host may have started
	// the game while the handshake was happening.
	// ------------------------------------------------------------

	room.mu.Lock()

	if room.Status != StatusWaiting {
		room.mu.Unlock()

		_ = conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(
				websocket.CloseTryAgainLater,
				"game has already started",
			),
		)

		_ = conn.Close()

		return
	}

	if len(room.Players) >= MaxPlayersPerRoom {
		room.mu.Unlock()

		_ = conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(
				websocket.CloseTryAgainLater,
				"room is full",
			),
		)

		_ = conn.Close()

		return
	}

	// First player becomes host.
	isHost := len(room.Players) == 0

	player := newPlayer(
		conn,
		newPlayerID(),
		name,
		room.nextJoinSeq(),
	)

	player.IsHost = isHost

	// Every player gets a private randomized card.
	player.Card = generateCard()

	// New players start NOT READY.
	player.Ready = false

	room.Players[player.ID] = player

	room.mu.Unlock()

	// Start dedicated writer.
	go player.writePump()

	// ------------------------------------------------------------
	// Send private welcome message.
	// ------------------------------------------------------------

	player.send(map[string]interface{}{
		"type":     "welcome",
		"playerId": player.ID,
		"isHost":   player.IsHost,
		"roomCode": room.Code,
		"ready":    player.Ready,
		"roundId":  room.RoundID,

		// PRIVATE.
		// This card is sent only to this player.
		"card": player.Card,
	})

	// ------------------------------------------------------------
	// Send current room state.
	// ------------------------------------------------------------

	room.mu.Lock()

	player.send(map[string]interface{}{
		"type":    "room_state",
		"players": room.publicPlayers(),
		"status":  room.Status,
		"roundId": room.RoundID,
	})

	room.broadcastRoomState()

	room.mu.Unlock()

	// ------------------------------------------------------------
	// Connection cleanup.
	// ------------------------------------------------------------

	defer func() {
		player.close()

		room.mu.Lock()

		wasHost := player.IsHost

		delete(
			room.Players,
			player.ID,
		)

		empty := len(room.Players) == 0

		// If host leaves, promote the oldest remaining player.
		if wasHost && !empty {
			newHost := room.oldestPlayer()

			if newHost != nil {
				newHost.IsHost = true

				newHost.send(map[string]interface{}{
					"type": "promoted_to_host",
				})
			}
		}

		if !empty {
			room.broadcastRoomState()
		}

		room.mu.Unlock()

		// Delete completely empty room.
		if empty {
			h.deleteRoom(code)
		}
	}()

	// ------------------------------------------------------------
	// WebSocket read configuration.
	// ------------------------------------------------------------

	conn.SetReadDeadline(
		time.Now().Add(pongWait),
	)

	conn.SetPongHandler(
		func(string) error {
			return conn.SetReadDeadline(
				time.Now().Add(pongWait),
			)
		},
	)

	// ------------------------------------------------------------
	// Main message loop.
	// ------------------------------------------------------------

	for {
		var msg clientMsg

		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		if !validMessageType(msg.Type) {
			player.send(map[string]interface{}{
				"type":    "error",
				"message": "Unknown game action.",
			})

			continue
		}

		h.handleMessage(
			room,
			player,
			msg,
		)
	}
}

// validMessageType contains Phase 1 actions.
//
// Gameplay actions are intentionally NOT implemented yet.
func validMessageType(msgType string) bool {
	switch msgType {
	case "set_ready":
		return true

	case "start_game":
		return true

	default:
		return false
	}
}

// handleMessage handles Phase 1 player actions.
func (h *Hub) handleMessage(
	room *Room,
	player *Player,
	msg clientMsg,
) {
	switch msg.Type {

	// ------------------------------------------------------------
	// READY / NOT READY
	// ------------------------------------------------------------

	case "set_ready":
		room.mu.Lock()

		// Ready state can only change before game starts.
		if room.Status != StatusWaiting {
			room.mu.Unlock()

			player.send(map[string]interface{}{
				"type":    "error",
				"message": "The game has already started.",
			})

			return
		}

		player.Ready = !player.Ready

		room.broadcastRoomState()

		room.mu.Unlock()

	// ------------------------------------------------------------
	// START GAME
	// ------------------------------------------------------------

	case "start_game":
		room.mu.Lock()

		// Only host can start.
		if !player.IsHost {
			room.mu.Unlock()

			player.send(map[string]interface{}{
				"type":    "error",
				"message": "Only the host can start the game.",
			})

			return
		}

		// Must still be waiting.
		if room.Status != StatusWaiting {
			room.mu.Unlock()

			player.send(map[string]interface{}{
				"type":    "error",
				"message": "The game has already started.",
			})

			return
		}

		// Need at least two players.
		if len(room.Players) < MinPlayersToStart {
			room.mu.Unlock()

			player.send(map[string]interface{}{
				"type":    "error",
				"message": "At least 2 players are required.",
			})

			return
		}

		// EVERY player must be ready.
		for _, p := range room.Players {
			if !p.Ready {
				room.mu.Unlock()

				player.send(map[string]interface{}{
					"type":    "error",
					"message": "Every player must be ready before the game starts.",
				})

				return
			}
		}

		// Start the game.
		room.Status = StatusPlaying
		room.RoundID++

		// Clear gameplay state.
		room.Called = make(map[int]bool)
		room.Order = nil
		room.Winner = ""

		// Notify everyone.
		room.broadcast(map[string]interface{}{
			"type":    "game_started",
			"roundId": room.RoundID,
		})

		room.broadcastRoomState()

		room.mu.Unlock()
	}
}
