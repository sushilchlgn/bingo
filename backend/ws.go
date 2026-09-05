package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return originAllowed(r.Header.Get("Origin"), r)
		},
}

func originAllowed(origin string, r *http.Request) bool {
	if origin == "" {
		return true
	}
	configured := strings.TrimSpace(os.Getenv("BINGO_ALLOWED_ORIGINS"))
	for _, allowed := range strings.Split(configured, ",") {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(allowed), "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}

	u, err := url.Parse(origin)
	if err != nil || u.Host != r.Host {
		return false
	}
	requestScheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		requestScheme = "https"
	}
	return strings.EqualFold(u.Scheme, requestScheme)
}

type clientMsg struct {
	Type    string `json:"type"`
	RoundID uint64 `json:"roundId,omitempty"`
	Number  int    `json:"number,omitempty"`
	Card    *Card  `json:"card,omitempty"`
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("name contains invalid UTF-8")
	}
	if utf8.RuneCountInString(name) > 20 {
		return "", fmt.Errorf("name must be 20 characters or fewer")
	}
	return name, nil
}

func (h *Hub) handleWS(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("room")))
	name, nameErr := normalizeName(r.URL.Query().Get("name"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if code == "" || nameErr != nil {
		msg := "room and a valid name are required"
		if nameErr != nil {
			msg = nameErr.Error()
		}
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	room, ok := h.getRoom(code)
	if !ok {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	// A game cannot be joined after it has started, except by a player using
	// the private token issued to their temporarily disconnected session.
	room.mu.Lock()
	reconnectPlayer := room.findDisconnectedByTokenLocked(token)
	if room.Status != StatusWaiting && reconnectPlayer == nil {
		room.mu.Unlock()
		http.Error(w, "game already started; joining is closed", http.StatusConflict)
		return
	}
	if reconnectPlayer == nil && len(room.Players) >= MaxPlayersPerRoom {
		room.mu.Unlock()
		http.Error(w, "room is full", http.StatusConflict)
		return
	}
	room.mu.Unlock()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	conn.SetReadLimit(maxMessageSize)

	room.mu.Lock()
	// Re-check after the WebSocket handshake, then either restore the existing
	// player or create a new lobby player.
	reconnectPlayer = room.findDisconnectedByTokenLocked(token)
	if reconnectPlayer == nil && (room.Status != StatusWaiting || len(room.Players) >= MaxPlayersPerRoom) {
		room.mu.Unlock()
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "joining is closed"))
		_ = conn.Close()
		return
	}

	isReconnect := reconnectPlayer != nil
	isHost := len(room.Players) == 0
	player := reconnectPlayer
	if player == nil {
		player = newPlayer(conn, newPlayerID(), name, room.nextJoinSeq())
		player.IsHost = isHost
		room.Players[player.ID] = player
	} else {
		// Use a new connection wrapper so the old read/write pumps cannot close
		// or mutate the newly restored connection.
		old := reconnectPlayer
		player = newPlayer(conn, old.ID, old.Name, old.JoinSeq)
		player.SessionToken = old.SessionToken
		player.Card = old.Card
		player.Ready = old.Ready
		player.IsHost = old.IsHost
		room.Players[player.ID] = player
		if room.Status == StatusPlaying {
			room.Paused = room.connectedPlayerCountLocked() < MinPlayersToStart
		}
	}
	room.mu.Unlock()

	go player.writePump()

	player.send(map[string]interface{}{
		"type":        "welcome",
		"playerId":    player.ID,
		"isHost":      player.IsHost,
		"roomCode":    code,
		"status":      room.Status,
		"roundId":     room.RoundID,
		"card":        player.Card,
		"token":       player.SessionToken,
		"reconnected": isReconnect,
		"paused":      room.Paused,
		"outcome":     room.Outcome,
	})

	room.mu.Lock()
	player.send(map[string]interface{}{
		"type":           "room_state",
		"players":        room.publicPlayers(),
		"status":         room.Status,
		"roundId":        room.RoundID,
		"activePlayerId": room.ActivePlayerID,
		"readyCount":     readyCount(room),
		"playerCount":    len(room.Players),
		"calledNumbers":  append([]int(nil), room.Order...),
		"paused":         room.Paused,
		"outcome":        room.Outcome,
	})
	room.broadcastRoomState()
	if room.Status == StatusFinished {
		if room.Outcome == "abandoned" {
			player.send(map[string]interface{}{
				"type": "game_abandoned", "message": "The round was abandoned because a player left.", "roundId": room.RoundID,
			})
		} else {
			finalCards := make(map[string]Card, len(room.Players))
			winnerName := ""
			for id, p := range room.Players {
				finalCards[id] = p.Card
				if id == room.Winner {
					winnerName = p.Name
				}
			}
			player.send(map[string]interface{}{
				"type": "game_over", "winnerId": room.Winner, "winnerName": winnerName,
				"roundId": room.RoundID, "calledNumbers": append([]int(nil), room.Order...), "cards": finalCards,
			})
		}
	}
	room.mu.Unlock()

	defer func() {
		player.close()

		room.mu.Lock()
		// A normal socket close is treated as a temporary disconnect. Keep the
		// player record and token so a browser refresh can restore it.
		if current, exists := room.Players[player.ID]; !exists || current != player {
			room.mu.Unlock()
			return
		}
		if player.IntentionalLeave {
			h.removePlayerLocked(room, player)
			empty := len(room.Players) == 0
			room.mu.Unlock()
			if empty {
				h.deleteRoom(code)
			}
			return
		}

		player.Connected = false
		player.DisconnectedUntil = time.Now().Add(DisconnectGracePeriod)
		if room.Status == StatusPlaying {
			room.Paused = true
		}
		player.disconnectTimer = time.AfterFunc(DisconnectGracePeriod, func() {
			h.expireDisconnectedPlayer(room, player, code)
		})
		room.broadcast(map[string]interface{}{
			"type":     "player_disconnected",
			"playerId": player.ID,
			"seconds":  int(DisconnectGracePeriod.Seconds()),
		})
		room.broadcastRoomState()
		room.mu.Unlock()
	}()

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

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
		h.handleMessage(room, player, msg)
	}
}

func validMessageType(msgType string) bool {
	switch msgType {
	case "set_card", "ready", "start_game", "call_number", "claim_bingo", "play_again", "leave":
		return true
	default:
		return false
	}
}

func (h *Hub) removePlayerLocked(room *Room, player *Player) {
	if player.disconnectTimer != nil {
		player.disconnectTimer.Stop()
		player.disconnectTimer = nil
	}
	delete(room.Players, player.ID)
	player.Connected = false

	if player.IsHost {
		if newHost := room.oldestPlayer(); newHost != nil {
			newHost.IsHost = true
			newHost.send(map[string]interface{}{"type": "promoted_to_host", "roundId": room.RoundID})
		}
	}

	abandoned := false
	if room.Status == StatusPlaying && room.connectedPlayerCountLocked() < MinPlayersToStart {
		room.Paused = true
		room.Status = StatusFinished
		room.Outcome = "abandoned"
		room.ActivePlayerID = ""
		abandoned = true
	}
	room.broadcastRoomState()
	if abandoned {
		room.broadcast(map[string]interface{}{
			"type":    "game_abandoned",
			"message": "The round ended because fewer than two players remained connected.",
			"roundId": room.RoundID,
		})
	}
}

func (h *Hub) expireDisconnectedPlayer(room *Room, player *Player, code string) {
	room.mu.Lock()
	if current, exists := room.Players[player.ID]; !exists || current != player || player.Connected || time.Now().Before(player.DisconnectedUntil) {
		room.mu.Unlock()
		return
	}
	h.removePlayerLocked(room, player)
	empty := len(room.Players) == 0
	room.mu.Unlock()
	if empty {
		h.deleteRoom(code)
	}
}

func readyCount(room *Room) int {
	count := 0
	for _, p := range room.Players {
		if p.Ready {
			count++
		}
	}
	return count
}

func sendError(player *Player, message string) {
	player.send(map[string]interface{}{
		"type":    "error",
		"message": message,
	})
}

func validRound(room *Room, msg clientMsg) bool {
	return msg.RoundID == 0 || msg.RoundID == room.RoundID
}

func (h *Hub) handleMessage(room *Room, player *Player, msg clientMsg) {
	switch msg.Type {
	case "set_card":
		h.handleSetCard(room, player, msg)
	case "ready":
		h.handleReady(room, player, msg)
	case "start_game":
		h.handleStartGame(room, player, msg)
	case "call_number":
		h.handleCallNumber(room, player, msg)
	case "claim_bingo":
		h.handleClaimBingo(room, player, msg)
	case "play_again":
		h.handlePlayAgain(room, player, msg)
	case "leave":
		h.handleLeave(room, player)
	}
}

func (h *Hub) handleLeave(room *Room, player *Player) {
	room.mu.Lock()
	if current, exists := room.Players[player.ID]; exists && current == player {
		player.IntentionalLeave = true
	}
	room.mu.Unlock()
	player.close()
}

func (h *Hub) handleSetCard(room *Room, player *Player, msg clientMsg) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Status != StatusWaiting || !validRound(room, msg) {
		return
	}
	if msg.Card == nil {
		sendError(player, "Card data is required.")
		return
	}
	if err := msg.Card.validatePartialCard(); err != nil {
		sendError(player, "Card may contain each number from 1 to 25 at most once; complete all 25 cells before starting.")
		return
	}

	player.Card = *msg.Card
	// A complete valid card is immediately ready. Players can still use the
	// READY control to become unready when they want to edit the card again.
	player.Ready = player.Card.isComplete()

	player.send(map[string]interface{}{
		"type":         "card_saved",
		"card":         player.Card,
		"cardComplete": player.Card.isComplete(),
		"ready":        player.Ready,
		"roundId":      room.RoundID,
	})
	room.broadcastRoomState()
}

func (h *Hub) handleReady(room *Room, player *Player, msg clientMsg) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Status != StatusWaiting || !validRound(room, msg) {
		return
	}
	if !player.Card.isComplete() {
		sendError(player, "Complete your 5x5 card before becoming ready.")
		return
	}

	player.Ready = !player.Ready
	room.broadcastRoomState()
}

func (h *Hub) handleStartGame(room *Room, player *Player, msg clientMsg) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Status != StatusWaiting || !validRound(room, msg) {
		return
	}
	if !player.IsHost {
		sendError(player, "Only the host can start the game.")
		return
	}
	if len(room.Players) < MinPlayersToStart {
		sendError(player, "At least 2 players are required to start the game.")
		return
	}
	if !room.allReady() {
		sendError(player, "Every player must complete their card and be READY.")
		return
	}

	room.Status = StatusPlaying
	room.RoundID++
	room.Called = make(map[int]bool)
	room.Order = nil
	room.Winner = ""
	room.Outcome = ""
	room.Paused = false

	players := room.sortedPlayers()
	room.ActivePlayerID = players[randomPlayerIndex(len(players))].ID

	room.broadcast(map[string]interface{}{
		"type":           "game_started",
		"roundId":        room.RoundID,
		"activePlayerId": room.ActivePlayerID,
	})
	room.broadcastRoomState()
}

func (h *Hub) handleCallNumber(room *Room, player *Player, msg clientMsg) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Status != StatusPlaying || room.Paused || !validRound(room, msg) {
		return
	}
	if room.ActivePlayerID != player.ID {
		sendError(player, "It is not your turn.")
		return
	}
	number := msg.Number
	if number < 1 || number > CardNumber {
		sendError(player, "Choose a number from 1 to 25.")
		return
	}
	if room.Called[number] {
		sendError(player, "That number has already been called.")
		return
	}
	if !player.Card.contains(number) {
		sendError(player, "You can only call a number that is on your own card.")
		return
	}

	room.Called[number] = true
	room.Order = append(room.Order, number)
	currentID := player.ID
	room.advanceTurnLocked(currentID)

	room.broadcast(map[string]interface{}{
		"type":           "number_called",
		"number":         number,
		"calledNumbers":  append([]int(nil), room.Order...),
		"roundId":        room.RoundID,
		"activePlayerId": room.ActivePlayerID,
	})
	room.broadcastRoomState()
}

func (h *Hub) handleClaimBingo(room *Room, player *Player, msg clientMsg) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Status != StatusPlaying || room.Paused || !validRound(room, msg) {
		return
	}

	completedLines := player.Card.completedLines(room.Called)
	if len(completedLines) >= BingoLines {
		room.Status = StatusFinished
		room.Winner = player.ID
		room.ActivePlayerID = ""

		finalCards := make(map[string]Card, len(room.Players))
		for id, p := range room.Players {
			finalCards[id] = p.Card
		}

		room.broadcast(map[string]interface{}{
			"type":          "game_over",
			"winnerId":      player.ID,
			"winnerName":    player.Name,
			"roundId":       room.RoundID,
			"calledNumbers": append([]int(nil), room.Order...),
			"winningLines":  completedLines,
			"cards":         finalCards,
		})
		room.broadcastRoomState()
		return
	}

	player.send(map[string]interface{}{
		"type":          "bingo_result",
		"valid":         false,
		"lineCount":     len(completedLines),
		"bingoProgress": "BINGO"[:minInt(len(completedLines), BingoLines)],
		"message":       "Not a BINGO yet — complete 5 rows, columns, or diagonals.",
		"roundId":       room.RoundID,
	})
}

func (h *Hub) handlePlayAgain(room *Room, player *Player, msg clientMsg) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Status != StatusFinished || !validRound(room, msg) {
		return
	}
	if !player.IsHost {
		sendError(player, "Only the host can start a new game.")
		return
	}

	room.Called = make(map[int]bool)
	room.Order = nil
	room.Winner = ""
	room.Outcome = ""
	room.Paused = false
	room.ActivePlayerID = ""
	room.Status = StatusWaiting
	room.RoundID++

	for _, p := range room.Players {
		p.Card = Card{}
		p.Ready = false
		p.send(map[string]interface{}{
			"type":    "new_round",
			"card":    p.Card,
			"roundId": room.RoundID,
		})
	}

	room.broadcastRoomState()
}
