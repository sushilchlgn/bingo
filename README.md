# Bingo — multiplayer WebSocket game

Real-time multiplayer 5x5 bingo. One host per room calls numbers manually;
players' cards are generated and validated server-side so nobody can fake
a win.

## Run it

```
go mod tidy
go run .
```

Then open http://localhost:8080 in a browser tab. Open it again in another
tab (or send the room code to a friend on another machine on the same
network / after deploying) to play with more players.

## How it works

- The first person to connect to a room becomes the **host**. The host
  clicks "Call Next Number"; everyone else just watches their card
  auto-highlight as numbers come in.
- Cards are generated on the server (`card.go`) using standard BINGO
  column ranges (B 1-15, I 16-30, N 31-45 w/ free center, G 46-60, O 61-75)
  and never trust the client for win checks — `claim_bingo` is verified
  against the server's own record of called numbers.
- If the host disconnects, host status passes to another player
  automatically so the game doesn't get stuck.

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
- `ws.go` — WebSocket connection handling and the message protocol (start_game, call_number, claim_bingo)
- `card.go` — server-authoritative card generation and win detection
- `static/index.html` + `static/app.js` — frontend client
