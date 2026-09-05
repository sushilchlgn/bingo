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

## Deploying with Netlify and a Go backend

Netlify hosts the static frontend, while the Go WebSocket server must run on a persistent backend host. The repository includes `netlify.toml`, which publishes the `static` directory, and `static/config.js`, which contains the backend URL used by the browser.

First deploy this project as a persistent Docker service using `Dockerfile` or `render.yaml` on Render, Railway, Fly.io, or a VPS. Configure the backend environment variable `BINGO_ALLOWED_ORIGINS` to the exact Netlify site origin, for example `https://your-site.netlify.app`. The backend must be reachable over HTTPS so its WebSocket endpoint is available as `wss://your-backend.example.com/ws`.

Then edit `static/config.js` before deploying the frontend:

```js
window.BINGO_CONFIG = {
  backendUrl: "https://your-backend.example.com"
};
```

Deploy the repository to Netlify using `netlify.toml`, or drag the `static` folder into Netlify Drop. The browser will send room creation requests to the configured backend and open WebSocket connections there. Local development continues to use same-origin `go run .` behavior when `backendUrl` is empty.

## Files

- `main.go` — HTTP server setup: room-creation endpoint, WebSocket route, static file serving
- `hub.go` — Room/Player structs, thread-safe room registry
- `ws.go` — WebSocket connection handling and the message protocol (set_card, ready, start_game, call_number, claim_bingo, play_again)
- `card.go` — server-authoritative card validation, line counting, and win detection
- `static/index.html` + `static/app.js` — frontend client
