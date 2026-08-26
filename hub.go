package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Maximum number of players in one room.
	MaxPlayersPerRoom = 25

	// Minimum number of players required to start.
	MinPlayersToStart = 2

	// Size of each player's outgoing message queue.
	SendQueueSize = 64
)

// Player represents one connected player.
type Player struct {
	ID   string
	Name string
	Conn *websocket.Conn

	// Every player gets their own private card.
	Card Card

	// Ready means the player has finished preparing
	// and is ready for the host to start the game.
	Ready bool

	// The first player in the room becomes host.
	IsHost bool

	// Used for deterministic host promotion.
	JoinSeq uint64

	sendCh    chan interface{}
	done      chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
}

// newPlayer creates a new player.
func newPlayer(
	conn *websocket.Conn,
	id string,
	name string,
	joinSeq uint64,
) *Player {
	return &Player{
		ID:      id,
		Name:    name,
		Conn:    conn,
		JoinSeq: joinSeq,

		sendCh: make(chan interface{}, SendQueueSize),
		done:   make(chan struct{}),
	}
}

// send places a message into the player's outgoing queue.
//
// The actual WebSocket write happens inside writePump.
func (p *Player) send(v interface{}) bool {
	if p.closed.Load() {
		return false
	}

	select {
	case <-p.done:
		return false

	case p.sendCh <- v:
		return true

	default:
		log.Printf(
			"send queue full for player %s (%s); closing connection",
			p.ID,
			p.Name,
		)

		p.close()
		return false
	}
}

// close safely closes the player connection.
func (p *Player) close() {
	p.closeOnce.Do(func() {
		p.closed.Store(true)

		close(p.done)

		_ = p.Conn.Close()
	})
}

// writePump is the only goroutine that writes to the WebSocket.
func (p *Player) writePump() {
	defer p.close()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return

		case msg := <-p.sendCh:
			if err := p.Conn.SetWriteDeadline(
				time.Now().Add(writeWait),
			); err != nil {
				return
			}

			if err := p.Conn.WriteJSON(msg); err != nil {
				log.Printf(
					"write error to %s: %v",
					p.Name,
					err,
				)

				return
			}

		case <-ticker.C:
			if err := p.Conn.SetWriteDeadline(
				time.Now().Add(writeWait),
			); err != nil {
				return
			}

			if err := p.Conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(writeWait),
			); err != nil {
				log.Printf(
					"ping error to %s: %v",
					p.Name,
					err,
				)

				return
			}
		}
	}
}

// GameStatus describes the room lifecycle.
type GameStatus string

const (
	// Players can join and change ready state.
	StatusWaiting GameStatus = "waiting"

	// Game has started.
	// New players cannot join.
	StatusPlaying GameStatus = "playing"

	// Game has ended.
	StatusFinished GameStatus = "finished"
)

// Room contains all server-authoritative game state.
type Room struct {
	Code string

	Players map[string]*Player

	// These are prepared for the actual game in later phases.
	Called map[int]bool
	Order  []int

	Status GameStatus

	Winner string

	RoundID uint64

	// Used to determine host promotion order.
	joinSeq uint64

	mu sync.Mutex
}

// newRoom creates an empty room.
func newRoom(code string) *Room {
	return &Room{
		Code: code,

		Players: make(map[string]*Player),

		Called: make(map[int]bool),

		Status: StatusWaiting,
	}
}

// publicPlayer is the safe player information sent to clients.
//
// IMPORTANT:
// The card is intentionally NOT included here.
//
// A player's card must remain private during the game.
type publicPlayer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IsHost bool   `json:"isHost"`
	Ready  bool   `json:"ready"`
}

// publicPlayers creates a client-safe player list.
//
// Caller must hold r.mu.
func (r *Room) publicPlayers() []publicPlayer {
	out := make(
		[]publicPlayer,
		0,
		len(r.Players),
	)

	for _, p := range r.Players {
		out = append(out, publicPlayer{
			ID:     p.ID,
			Name:   p.Name,
			IsHost: p.IsHost,
			Ready:  p.Ready,
		})
	}

	return out
}

// playerSnapshot returns all players.
//
// Caller must hold r.mu.
func (r *Room) playerSnapshot() []*Player {
	out := make(
		[]*Player,
		0,
		len(r.Players),
	)

	for _, p := range r.Players {
		out = append(out, p)
	}

	return out
}

// broadcast sends a message to every current player.
//
// Caller must hold r.mu.
func (r *Room) broadcast(v interface{}) {
	for _, p := range r.Players {
		p.send(v)
	}
}

// broadcastRoomState broadcasts the current room state.
//
// Caller must hold r.mu.
func (r *Room) broadcastRoomState() {
	r.broadcast(map[string]interface{}{
		"type":    "room_state",
		"players": r.publicPlayers(),
		"status":  r.Status,
		"roundId": r.RoundID,
	})
}

// nextJoinSeq returns the next player ordering number.
//
// Caller must hold r.mu.
func (r *Room) nextJoinSeq() uint64 {
	r.joinSeq++

	return r.joinSeq
}

// oldestPlayer returns the earliest joined player.
//
// Used when the host leaves.
//
// Caller must hold r.mu.
func (r *Room) oldestPlayer() *Player {
	var oldest *Player

	for _, p := range r.Players {
		if oldest == nil || p.JoinSeq < oldest.JoinSeq {
			oldest = p
		}
	}

	return oldest
}

// Hub contains all rooms.
type Hub struct {
	mu sync.Mutex

	rooms map[string]*Room
}

// newHub creates a new room hub.
func newHub() *Hub {
	return &Hub{
		rooms: make(map[string]*Room),
	}
}

// Characters used for room codes.
//
// Ambiguous characters are removed:
//
// 0/O
// 1/I
const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// randomCode creates a cryptographically random code.
func randomCode(n int) string {
	b := make([]byte, n)

	for i := range b {
		index, err := rand.Int(
			rand.Reader,
			big.NewInt(int64(len(codeChars))),
		)

		if err != nil {
			panic(err)
		}

		b[i] = codeChars[index.Int64()]
	}

	return string(b)
}

// createRoom creates a unique five-character room.
func (h *Hub) createRoom() *Room {
	h.mu.Lock()
	defer h.mu.Unlock()

	for {
		code := randomCode(5)

		if _, exists := h.rooms[code]; exists {
			continue
		}

		room := newRoom(code)

		h.rooms[code] = room

		return room
	}
}

// getRoom finds a room.
func (h *Hub) getRoom(code string) (*Room, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, ok := h.rooms[code]

	return room, ok
}

// deleteRoom removes a room.
func (h *Hub) deleteRoom(code string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.rooms, code)
}

// newPlayerID creates a random player ID.
func newPlayerID() string {
	return randomCode(10)
}

// mustJSON is kept as a small utility for future phases.
func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)

	return b
}