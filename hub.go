package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"sync"

	"github.com/gorilla/websocket"
)

type Player struct {
	ID     string
	Name   string
	Conn   *websocket.Conn
	Card   Card
	IsHost bool
	sendMu sync.Mutex // serializes writes to Conn (gorilla conns aren't write-safe from multiple goroutines)
}

func (p *Player) send(v interface{}) {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	if err := p.Conn.WriteJSON(v); err != nil {
		log.Printf("write error to %s: %v", p.Name, err)
	}
}

type GameStatus string

const (
	StatusWaiting  GameStatus = "waiting"
	StatusPlaying  GameStatus = "playing"
	StatusFinished GameStatus = "finished"
)

type Room struct {
	Code    string
	Players map[string]*Player // keyed by player ID
	Called  map[int]bool
	Order   []int // order numbers were called, for display
	Status  GameStatus
	Winner  string // player ID
	mu      sync.Mutex
}

func newRoom(code string) *Room {
	return &Room{
		Code:    code,
		Players: make(map[string]*Player),
		Called:  make(map[int]bool),
		Status:  StatusWaiting,
	}
}

// publicPlayer is the JSON-safe view of a player sent to all clients.
type publicPlayer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IsHost bool   `json:"isHost"`
}

func (r *Room) publicPlayers() []publicPlayer {
	out := make([]publicPlayer, 0, len(r.Players))
	for _, p := range r.Players {
		out = append(out, publicPlayer{ID: p.ID, Name: p.Name, IsHost: p.IsHost})
	}
	return out
}

// broadcast sends v to every player currently in the room. Caller must hold r.mu.
func (r *Room) broadcast(v interface{}) {
	for _, p := range r.Players {
		p.send(v)
	}
}

// broadcastRoomState tells everyone who's in the room and what the status is.
// Caller must hold r.mu.
func (r *Room) broadcastRoomState() {
	r.broadcast(map[string]interface{}{
		"type":    "room_state",
		"players": r.publicPlayers(),
		"status":  r.Status,
	})
}

type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

func newHub() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I to avoid ambiguity

func randomCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(codeChars))))
		b[i] = codeChars[idx.Int64()]
	}
	return string(b)
}

// createRoom makes a new room with a unique 5-character code.
func (h *Hub) createRoom() *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	var code string
	for {
		code = randomCode(5)
		if _, exists := h.rooms[code]; !exists {
			break
		}
	}
	r := newRoom(code)
	h.rooms[code] = r
	return r
}

func (h *Hub) getRoom(code string) (*Room, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[code]
	return r, ok
}

func (h *Hub) deleteRoom(code string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, code)
}

func newPlayerID() string {
	return randomCode(10)
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
