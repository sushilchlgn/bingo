package main

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	mathrand "math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	MaxPlayersPerRoom     = 5
	SendQueueSize         = 64
	MinPlayersToStart     = 2
	DisconnectGracePeriod = 45 * time.Second
)

var (
	errInvalidCard     = errors.New("card must contain every number from 1 to 25 exactly once")
	errDuplicateNumber = errors.New("card contains a duplicate number")
)

type Player struct {
	ID      string
	Name    string
	Conn    *websocket.Conn
	Card    Card
	Ready   bool
	IsHost  bool
	JoinSeq uint64

	SessionToken      string
	Connected         bool
	DisconnectedUntil time.Time
	IntentionalLeave  bool
	disconnectTimer   *time.Timer

	sendCh    chan interface{}
	done      chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
}

func newPlayer(conn *websocket.Conn, id, name string, joinSeq uint64) *Player {
	return &Player{
		ID:           id,
		Name:         name,
		Conn:         conn,
		JoinSeq:      joinSeq,
		SessionToken: randomCode(32),
		Connected:    true,
		sendCh:       make(chan interface{}, SendQueueSize),
		done:         make(chan struct{}),
	}
}

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
		log.Printf("send queue full for player %s (%s); closing connection", p.ID, p.Name)
		p.close()
		return false
	}
}

func (p *Player) close() {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		close(p.done)
		if p.Conn != nil {
			_ = p.Conn.Close()
		}
	})
}

func (p *Player) writePump() {
	defer p.close()
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case msg := <-p.sendCh:
			if err := p.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := p.Conn.WriteJSON(msg); err != nil {
				log.Printf("write error to %s: %v", p.Name, err)
				return
			}
		case <-ticker.C:
			if err := p.Conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := p.Conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				log.Printf("ping error to %s: %v", p.Name, err)
				return
			}
		}
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
	Players map[string]*Player
	Called  map[int]bool
	Order   []int
	Status  GameStatus
	Winner  string
	Outcome string
	RoundID uint64
	Paused  bool

	// ActivePlayerID is the only player allowed to call a number during a turn.
	ActivePlayerID string

	joinSeq uint64
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

type publicPlayer struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	IsHost           bool   `json:"isHost"`
	Ready            bool   `json:"ready"`
	CardComplete     bool   `json:"cardComplete"`
	BingoCount       int    `json:"bingoCount"`
	BingoProgress    string `json:"bingoProgress"`
	Connected        bool   `json:"connected"`
	ReconnectSeconds int    `json:"reconnectSeconds"`
}

func (r *Room) publicPlayers() []publicPlayer {
	out := make([]publicPlayer, 0, len(r.Players))
	players := r.sortedPlayers()
	for _, p := range players {
		count := p.Card.completedLineCount(r.Called)
		if count > BingoLines {
			count = BingoLines
		}
		out = append(out, publicPlayer{
			ID:               p.ID,
			Name:             p.Name,
			IsHost:           p.IsHost,
			Ready:            p.Ready,
			CardComplete:     p.Card.isComplete(),
			BingoCount:       count,
			BingoProgress:    "BINGO"[:count],
			Connected:        p.Connected,
			ReconnectSeconds: reconnectSeconds(p),
		})
	}
	return out
}

func (r *Room) sortedPlayers() []*Player {
	out := make([]*Player, 0, len(r.Players))
	for _, p := range r.Players {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].JoinSeq < out[j].JoinSeq
	})
	return out
}

func (r *Room) broadcast(v interface{}) {
	for _, p := range r.Players {
		p.send(v)
	}
}

func reconnectSeconds(p *Player) int {
	if p.Connected || p.DisconnectedUntil.IsZero() {
		return 0
	}
	remaining := int(time.Until(p.DisconnectedUntil).Seconds())
	if remaining < 0 {
		return 0
	}
	return remaining + 1
}

func (r *Room) broadcastRoomState() {
	readyCount := 0
	for _, p := range r.Players {
		if p.Ready {
			readyCount++
		}
	}

	r.broadcast(map[string]interface{}{
		"type":           "room_state",
		"players":        r.publicPlayers(),
		"status":         r.Status,
		"roundId":        r.RoundID,
		"activePlayerId": r.ActivePlayerID,
		"readyCount":     readyCount,
		"playerCount":    len(r.Players),
		"calledNumbers":  append([]int(nil), r.Order...),
		"paused":         r.Paused,
		"outcome":        r.Outcome,
	})
}

func (r *Room) nextJoinSeq() uint64 {
	r.joinSeq++
	return r.joinSeq
}

func (r *Room) oldestPlayer() *Player {
	var oldest *Player
	for _, p := range r.Players {
		if !p.Connected {
			continue
		}
		if oldest == nil || p.JoinSeq < oldest.JoinSeq {
			oldest = p
		}
	}
	return oldest
}

func (r *Room) findDisconnectedByTokenLocked(token string) *Player {
	if token == "" {
		return nil
	}
	for _, p := range r.Players {
		if !p.Connected && p.SessionToken == token && time.Now().Before(p.DisconnectedUntil) {
			return p
		}
	}
	return nil
}

func (r *Room) connectedPlayerCountLocked() int {
	count := 0
	for _, p := range r.Players {
		if p.Connected {
			count++
		}
	}
	return count
}

func (r *Room) hasDisconnectedLocked() bool {
	for _, p := range r.Players {
		if !p.Connected {
			return true
		}
	}
	return false
}

// advanceTurnLocked selects the next connected player after currentID.
// Caller must hold r.mu.
func (r *Room) advanceTurnLocked(currentID string) {
	players := r.sortedPlayers()
	if len(players) == 0 {
		r.ActivePlayerID = ""
		return
	}

	if currentID == "" {
		r.ActivePlayerID = players[0].ID
		return
	}

	for i, p := range players {
		if p.ID == currentID {
			next := (i + 1) % len(players)
			r.ActivePlayerID = players[next].ID
			return
		}
	}

	// Current player disconnected; pick the first remaining player.
	r.ActivePlayerID = players[0].ID
}

func (r *Room) allReady() bool {
	if r.connectedPlayerCountLocked() < MinPlayersToStart {
		return false
	}
	for _, p := range r.Players {
		if !p.Connected || !p.Ready || !p.Card.isComplete() {
			return false
		}
	}
	return true
}

type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

func newHub() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(codeChars))))
		b[i] = codeChars[idx.Int64()]
	}
	return string(b)
}

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

// randomPlayerIndex uses math/rand only for selecting the first turn. It does not
// affect any authoritative card/number validation.
func randomPlayerIndex(n int) int {
	if n <= 1 {
		return 0
	}
	return mathrand.Intn(n)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
