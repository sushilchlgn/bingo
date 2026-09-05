# Bingo — multiplayer WebSocket game

Real-time multiplayer 5x5 bingo for 2–5 players. Players randomize a card
containing every number from 1 to 25, take turns calling numbers from their own
card, and each called number is marked on every player's card. Cards and win
checks are validated server-side so nobody can fake a win.

## Run it

```
go mod tidy
go run .
```

Then open http://localhost:8080 in a browser tab. Open it again in another
tab (or send the room code to a friend on another machine on the same
network / after deploying) to play with more players.

## How it works

- The first person to connect to a room becomes the **host**. A room requires
  at least 2 players and accepts at most 5 players. The host starts only after
  every player has a complete card; a complete card is marked READY
  automatically, and a player can become UNREADY to edit it again.
- The first turn is selected randomly. On each turn, the active player calls an
  uncalled number from their own card; that number is marked on every card and
  the turn advances to the next connected player.
- Cards use the numbers 1–25 exactly once. A completed row, column, or
  diagonal counts as one B-I-N-G-O letter. Five completed lines are required;
  the `claim_bingo` action is then validated against the server's called-number
  record before the game ends.
- The server never trusts client marks or win checks. Only a valid claim reveals
  all player cards.
- A browser refresh is treated as a temporary disconnect. The player keeps
  their ID, card, readiness, host status, and turn position for 45 seconds;
  the token in browser local storage lets them reconnect to the active room.
  During this grace period, a playing round is paused and the UI shows the
  reconnect countdown.
- Clicking **Leave Room** is intentional and removes the player immediately.
  If fewer than 2 connected players remain during a round, the round ends as
  **abandoned** rather than awarding an automatic win. If the host's grace
  period expires or the host intentionally leaves, host status passes to the
  oldest connected player.

## Deploying so friends elsewhere can join

Any small VM or PaaS that runs a Go binary works (Fly.io, Railway,
a $5 VPS, etc.). Build a binary with `go build -o bingo-server .`,
run it, and share `http://<your-server>:8080` plus the room code.
For real deployments, put it behind HTTPS (e.g. via a reverse proxy)
so the frontend can use `wss://` — the JS already auto-detects this
based on `location.protocol`.

## Files

- `main.go` — HTTP server setup: room-creation endpoint, WebSocket route, static file serving
- `hub.go` — Room/Player structs, thread-safe room registry
- `ws.go` — WebSocket connection handling and the message protocol (set_card, ready, start_game, call_number, claim_bingo, play_again)
- `card.go` — server-authoritative card validation, line counting, and win detection
- `static/index.html` + `static/app.js` — frontend client
