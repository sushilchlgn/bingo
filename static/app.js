let ws = null;
let myPlayerId = null;
let isHost = false;
let leavingRoom = false;
let roomCode = null;
let roundId = 0;
let status = 'waiting';
let activePlayerId = '';
let paused = false;
let outcome = '';
let reconnectToken = '';
let card = Array.from({ length: 5 }, () => Array(5).fill(0));
let calledNumbers = new Set();
let players = [];
let selectedNumber = null;
let selectedCell = null;

const $ = id => document.getElementById(id);
const screenJoin = $('screen-join');
const screenGame = $('screen-game');
const nameInput = $('nameInput');
const codeInput = $('codeInput');
const createBtn = $('createBtn');
const joinBtn = $('joinBtn');
const joinError = $('joinError');
const roomCodeDisplay = $('roomCodeDisplay');
const statusDisplay = $('statusDisplay');
const hostControls = $('hostControls');
const startBtn = $('startBtn');
const playAgainBtn = $('playAgainBtn');
const leaveBtn = $('leaveBtn');
const playerCount = $('playerCount');
const lobbyView = $('lobbyView');
const playingView = $('playingView');
const finishedView = $('finishedView');
const editorGrid = $('editorGrid');
const numberPalette = $('numberPalette');
const numberInput = $('numberInput');
const placeInputBtn = $('placeInputBtn');
const clearCardBtn = $('clearCardBtn');
const randomizeCardBtn = $('randomizeCardBtn');
const readyBtn = $('readyBtn');
const cardMessage = $('cardMessage');
const readySummary = $('readySummary');
const playerList = $('playerList');
const gameGrid = $('gameGrid');
const bingoBtn = $('bingoBtn');
const bingoProgress = $('bingoProgress');
const turnDisplay = $('turnDisplay');
const lastCalled = $('lastCalled');
const callHistory = $('callHistory');
const playingPlayerList = $('playingPlayerList');
const winnerDisplay = $('winnerDisplay');
const finalCards = $('finalCards');
const toast = $('toast');

function showToast(message, ms = 3000) {
  toast.textContent = message;
  toast.style.display = 'block';
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => toast.style.display = 'none', ms);
}

function emptyCard() {
  return Array.from({ length: 5 }, () => Array(5).fill(0));
}

function cardComplete(c = card) {
  const seen = new Set();
  for (const row of c) {
    for (const n of row) {
      if (!Number.isInteger(n) || n < 1 || n > 25 || seen.has(n)) return false;
      seen.add(n);
    }
  }
  return seen.size === 25;
}

function countPlaced() {
  return card.flat().filter(n => n >= 1 && n <= 25).length;
}

function usedNumbers() {
  return new Set(card.flat().filter(n => n >= 1 && n <= 25));
}

function statusLabel() {
  if (status === 'waiting') return 'Lobby — 2 to 5 players, complete cards auto-ready';
  if (status === 'playing' && paused) return 'Game paused — waiting for reconnect';
  if (status === 'playing') return 'Game in progress';
  if (status === 'finished') return 'Game over';
  return status;
}

function resetLocalState() {
  myPlayerId = null;
  isHost = false;
  roomCode = null;
  roundId = 0;
  status = 'waiting';
  activePlayerId = '';
  paused = false;
  outcome = '';
  reconnectToken = '';
  card = emptyCard();
  calledNumbers = new Set();
  players = [];
  selectedNumber = null;
  selectedCell = null;
}

function loadReconnectSession(code, name) {
  try {
    const saved = JSON.parse(localStorage.getItem('bingoReconnect') || 'null');
    if (saved?.room === code && saved?.name === name && saved?.token) return saved.token;
  } catch {}
  return '';
}

function saveReconnectSession(code, name, token) {
  reconnectToken = token;
  try { localStorage.setItem('bingoReconnect', JSON.stringify({ room: code, name, token })); } catch {}
}

function clearReconnectSession() {
  reconnectToken = '';
  try { localStorage.removeItem('bingoReconnect'); } catch {}
}

function connect(code, name) {
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws';
  const token = loadReconnectSession(code, name);
  const tokenParam = token ? `&token=${encodeURIComponent(token)}` : '';
  ws = new WebSocket(`${protocol}://${location.host}/ws?room=${encodeURIComponent(code)}&name=${encodeURIComponent(name)}${tokenParam}`);

  ws.onopen = () => { joinError.textContent = ''; };
  ws.onerror = () => { joinError.textContent = 'Could not connect. Check the room code or whether joining is still open.'; };
  ws.onclose = () => {
    if (!leavingRoom) showToast('Disconnected from server.');
  };
  ws.onmessage = event => {
    try { handleMessage(JSON.parse(event.data)); }
    catch { showToast('Received invalid server data.'); }
  };
}

createBtn.onclick = async () => {
  const name = nameInput.value.trim();
  if (!name) { joinError.textContent = 'Enter your name first.'; return; }
  try {
    const res = await fetch('/api/rooms', { method: 'POST' });
    if (!res.ok) throw new Error('room creation failed');
    const data = await res.json();
    clearReconnectSession();
    connect(data.code, name);
  } catch {
    joinError.textContent = 'Could not create room.';
  }
};

joinBtn.onclick = () => {
  const name = nameInput.value.trim();
  const code = codeInput.value.trim().toUpperCase();
  if (!name) { joinError.textContent = 'Enter your name first.'; return; }
  if (!code) { joinError.textContent = 'Enter a room code.'; return; }
  connect(code, name);
};

function randomizeCard() {
  const me = players.find(p => p.id === myPlayerId);

  if (me?.ready) {
    showToast('You are READY. Become UNREADY before changing your card.');
    return;
  }

  // Create numbers 1–25.
  const numbers = Array.from({ length: 25 }, (_, i) => i + 1);

  // Fisher-Yates shuffle.
  for (let i = numbers.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [numbers[i], numbers[j]] = [numbers[j], numbers[i]];
  }

  // Arrange the shuffled numbers into the 5×5 card.
  card = Array.from({ length: 5 }, (_, row) =>
    numbers.slice(row * 5, row * 5 + 5)
  );

  selectedNumber = null;
  selectedCell = null;

  // Immediately save the complete card to the server.
  send('set_card', { card });

  renderLobby();

  showToast('🎲 Your card has been randomized!');
}
randomizeCardBtn.onclick = randomizeCard;

function handleMessage(msg) {
  if (msg.roundId !== undefined && msg.type !== 'welcome') {
    const incoming = Number(msg.roundId);
    if (msg.type !== 'new_round' && incoming < roundId) return;
  }

  switch (msg.type) {
    case 'welcome':
      myPlayerId = msg.playerId;
      isHost = !!msg.isHost;
      roomCode = msg.roomCode;
      status = msg.status || 'waiting';
      roundId = Number(msg.roundId || 0);
      paused = !!msg.paused;
      outcome = msg.outcome || '';
      card = normalizeCard(msg.card);
      if (msg.token) saveReconnectSession(roomCode, nameInput.value.trim(), msg.token);
      enterGameScreen();
      if (msg.reconnected) showToast('Reconnected to your active game.');
      break;

    case 'room_state':
      status = msg.status;
      roundId = Number(msg.roundId || roundId);
      activePlayerId = msg.activePlayerId || '';
      if (Array.isArray(msg.calledNumbers)) calledNumbers = new Set(msg.calledNumbers);
      paused = !!msg.paused;
      outcome = msg.outcome || '';
      players = msg.players || [];
      playerCount.textContent = `(${players.length}/5)`;
      const me = players.find(p => p.id === myPlayerId);
      if (me) isHost = !!me.isHost;
      renderAll();
      break;

    case 'card_saved':
      card = normalizeCard(msg.card);
      showToast(msg.ready ? 'Card saved and marked READY automatically.' : 'Card updated. Complete all 25 cells to become READY.');
      renderLobby();
      break;

    case 'game_started':
      status = 'playing';
      roundId = Number(msg.roundId || roundId);
      activePlayerId = msg.activePlayerId || '';
      paused = !!msg.paused;
      outcome = msg.outcome || '';
      calledNumbers = new Set();
      showToast(activePlayerId === myPlayerId ? '🎲 You start! Choose a number from your card.' : 'Game started!');
      renderAll();
      break;

    case 'number_called':
      calledNumbers = new Set(msg.calledNumbers || []);
      activePlayerId = msg.activePlayerId || '';
      lastCalled.textContent = String(msg.number);
      renderPlaying();
      break;

    case 'bingo_result':
      if (!msg.valid) showToast(msg.message || 'Not a BINGO yet.');
      break;

    case 'game_abandoned':
      status = 'finished';
      paused = false;
      outcome = 'abandoned';
      activePlayerId = '';
      winnerDisplay.textContent = msg.message || 'Game abandoned because a player left.';
      finalCards.innerHTML = '';
      renderAll();
      showToast('The game was abandoned because a player did not reconnect.', 6000);
      break;

    case 'game_over':
      status = 'finished';
      activePlayerId = '';
      calledNumbers = new Set(msg.calledNumbers || []);
      winnerDisplay.textContent = `🏆 ${msg.winnerName} got BINGO!`;
      renderFinalCards(msg.cards || {}, msg.winnerId);
      showToast(msg.winnerId === myPlayerId ? '🎉 You got BINGO!' : `${msg.winnerName} got BINGO!`, 6000);
      renderAll();
      break;

    case 'new_round':
      status = 'waiting';
      roundId = Number(msg.roundId || roundId);
      activePlayerId = '';
      paused = false;
      outcome = '';
      calledNumbers = new Set();
      card = normalizeCard(msg.card);
      selectedNumber = null;
      selectedCell = null;
      winnerDisplay.textContent = '🏆 Game over';
      finalCards.innerHTML = '';
      renderAll();
      break;

    case 'promoted_to_host':
      isHost = true;
      showToast('The host left — you are now the host.');
      renderAll();
      break;

    case 'error':
      showToast(msg.message || 'Action rejected.');
      break;
  }
}

function normalizeCard(value) {
  if (!Array.isArray(value) || value.length !== 5) return emptyCard();
  return value.map(row => Array.isArray(row) && row.length === 5 ? row.map(Number) : [0, 0, 0, 0, 0]);
}

function send(type, extra = {}) {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    showToast('Not connected to the server.');
    return;
  }
  ws.send(JSON.stringify({ type, roundId, ...extra }));
}

function enterGameScreen() {
  screenJoin.style.display = 'none';
  screenGame.style.display = 'block';
  roomCodeDisplay.textContent = roomCode;
  renderAll();
}

function renderAll() {
  statusDisplay.textContent = statusLabel();
  hostControls.style.display = isHost ? 'flex' : 'none';
  startBtn.style.display = status === 'waiting' ? 'inline-block' : 'none';
  playAgainBtn.style.display = status === 'finished' ? 'inline-block' : 'none';

  if (status === 'waiting') {
    lobbyView.classList.remove('hidden');
    playingView.classList.add('hidden');
    finishedView.classList.add('hidden');
    renderLobby();
  } else if (status === 'playing') {
    lobbyView.classList.add('hidden');
    playingView.classList.remove('hidden');
    finishedView.classList.add('hidden');
    renderPlaying();
  } else {
    lobbyView.classList.add('hidden');
    playingView.classList.add('hidden');
    finishedView.classList.remove('hidden');
  }

  renderPlayers(players);
}

function renderLobby() {
  renderEditor();
  renderPalette();
  const me = players.find(p => p.id === myPlayerId);
  const complete = cardComplete();
  const ready = !!me?.ready;

  cardMessage.textContent = `${countPlaced()} / 25 numbers placed${complete ? ' — card complete ✓' : ''}`;
  cardMessage.style.color = complete ? 'var(--ok)' : 'var(--muted)';

  readyBtn.disabled = !complete;
  readyBtn.textContent = ready ? 'UNREADY / EDIT CARD' : 'READY';
  readyBtn.className = ready ? 'secondary' : '';

  const enoughPlayers = players.length >= 2;
  const allReady = enoughPlayers && players.every(p => p.ready && p.cardComplete);
  startBtn.disabled = !allReady;
  startBtn.title = allReady ? '' : (enoughPlayers ? 'Every player must complete a card.' : 'At least 2 players are required.');
  readySummary.textContent = `${players.filter(p => p.ready).length} / ${players.length} players ready${enoughPlayers ? '' : ' — need at least 2 players'}`;

  if (ready) {
    numberInput.disabled = true;
    placeInputBtn.disabled = true;
    clearCardBtn.disabled = true;
    randomizeCardBtn.disabled = true;
  } else {
    numberInput.disabled = false;
    placeInputBtn.disabled = false;
    clearCardBtn.disabled = false;
    randomizeCardBtn.disabled = false;
  }
}

function renderEditor() {
  editorGrid.innerHTML = '';
  for (let r = 0; r < 5; r++) {
    for (let c = 0; c < 5; c++) {
      const cell = document.createElement('div');
      const value = card[r][c];
      cell.className = 'cell' + (value ? '' : ' empty');
      cell.textContent = value || '·';
      if (selectedCell === r * 5 + c) cell.classList.add('selected');
      cell.dataset.index = String(r * 5 + c);
      cell.onclick = () => {
        selectedCell = r * 5 + c;
        if (selectedNumber !== null) placeNumber(selectedNumber, selectedCell);
        else renderEditor();
      };
      cell.ondragover = e => e.preventDefault();
      cell.ondrop = e => {
        e.preventDefault();
        const n = Number(e.dataTransfer.getData('text/plain'));
        if (n) placeNumber(n, r * 5 + c);
      };
      editorGrid.appendChild(cell);
    }
  }
}

function renderPalette() {
  const used = usedNumbers();
  numberPalette.innerHTML = '';
  for (let n = 1; n <= 25; n++) {
    const btn = document.createElement('button');
    btn.className = 'num' + (used.has(n) ? ' used' : '') + (selectedNumber === n ? ' selected' : '');
    btn.textContent = n;
    btn.draggable = !used.has(n);
    btn.disabled = used.has(n);
    btn.onclick = () => {
      selectedNumber = n;
      if (selectedCell !== null) placeNumber(n, selectedCell);
      else { showToast(`Number ${n} selected — now click a cell.`); renderPalette(); }
    };
    btn.ondragstart = e => e.dataTransfer.setData('text/plain', String(n));
    numberPalette.appendChild(btn);
  }
}

function placeNumber(n, index) {
  const me = players.find(p => p.id === myPlayerId);
  if (me?.ready) { showToast('You are READY. Become UNREADY before editing.'); return; }
  if (n < 1 || n > 25) { showToast('Use a number from 1 to 25.'); return; }

  const targetR = Math.floor(index / 5);
  const targetC = index % 5;
  const existingIndex = card.flat().indexOf(n);
  if (existingIndex !== -1 && existingIndex !== index) {
    showToast(`Number ${n} is already on your card.`);
    return;
  }

  card[targetR][targetC] = n;
  selectedNumber = null;
  selectedCell = null;
  renderLobby();

  // Keep partial edits server-side too; the server marks the player ready only
  // when all 25 numbers are valid and present.
  send('set_card', { card });
}

function clearCell(index) {
  const r = Math.floor(index / 5), c = index % 5;
  card[r][c] = 0;
  send('set_card', { card });
}

// Double-clicking a filled cell removes it so the player can correct the card.
editorGrid.ondblclick = e => {
  const cell = e.target.closest('.cell');
  if (!cell) return;
  const me = players.find(p => p.id === myPlayerId);
  if (me?.ready) return;
  clearCell(Number(cell.dataset.index));
  selectedCell = null;
  selectedNumber = null;
  renderLobby();
};

placeInputBtn.onclick = () => {
  const n = Number(numberInput.value);
  if (selectedCell === null) { showToast('Select a card cell first.'); return; }
  if (!Number.isInteger(n) || n < 1 || n > 25) { showToast('Enter a number from 1 to 25.'); return; }
  placeNumber(n, selectedCell);
  numberInput.value = '';
};

numberInput.onkeydown = e => { if (e.key === 'Enter') placeInputBtn.click(); };

clearCardBtn.onclick = () => {
  const me = players.find(p => p.id === myPlayerId);
  if (me?.ready) return;
  card = emptyCard();
  selectedNumber = null;
  selectedCell = null;
  send('set_card', { card });
  renderLobby();
};

readyBtn.onclick = () => {
  send('ready');
};

startBtn.onclick = () => send('start_game');
playAgainBtn.onclick = () => send('play_again');

function leaveRoom() {
  leavingRoom = true;
  if (ws && ws.readyState === WebSocket.OPEN) send('leave');
  clearReconnectSession();
  if (ws) ws.close();
  resetLocalState();
  screenGame.style.display = 'none';
  screenJoin.style.display = 'block';
  joinError.textContent = '';
  showToast('You left the room.');
  setTimeout(() => { leavingRoom = false; }, 0);
}
leaveBtn.onclick = leaveRoom;

function renderPlaying() {
  gameGrid.innerHTML = '';
  for (let r = 0; r < 5; r++) {
    for (let c = 0; c < 5; c++) {
      const n = card[r][c];
      const cell = document.createElement('div');
      cell.className = 'cell' + (calledNumbers.has(n) ? ' marked' : '') + ' locked';
      cell.textContent = n;
      if (!paused && activePlayerId === myPlayerId && !calledNumbers.has(n)) {
        cell.classList.remove('locked');
        cell.onclick = () => send('call_number', { number: n });
      }
      gameGrid.appendChild(cell);
    }
  }

  const current = players.find(p => p.id === activePlayerId);
  const disconnected = players.find(p => !p.connected);
  turnDisplay.textContent = paused
    ? `Waiting for ${disconnected?.name || 'player'} to reconnect${disconnected?.reconnectSeconds ? ` — ${disconnected.reconnectSeconds}s` : ''}`
    : (current ? `${current.name}${activePlayerId === myPlayerId ? ' — YOUR TURN' : ''}` : '—');
  turnDisplay.className = 'turn-box' + (activePlayerId === myPlayerId ? ' turn' : '');
  renderCallHistory();
  renderPlayingPlayers();
  const me = players.find(p => p.id === myPlayerId);
  const bingoCount = Math.min(Number(me?.bingoCount || 0), 5);
  const progress = me?.bingoProgress || '—';
  bingoProgress.textContent = `${progress} (${bingoCount}/5 completed lines)`;
  bingoProgress.className = bingoCount >= 5 ? 'bingo-progress complete' : 'bingo-progress';
  bingoBtn.disabled = paused || bingoCount < 5;
  bingoBtn.textContent = bingoCount >= 5 ? 'BINGO! — Validate' : `BINGO! (${bingoCount}/5)`;
}

function renderCallHistory() {
  callHistory.innerHTML = '';
  const order = [...calledNumbers];
  if (order.length) lastCalled.textContent = String(order[order.length - 1]);
  else lastCalled.textContent = '—';
  order.slice().reverse().forEach(n => {
    const span = document.createElement('span');
    span.textContent = n;
    callHistory.appendChild(span);
  });
}

bingoBtn.onclick = () => send('claim_bingo');

function renderPlayers(list) {
  playerList.innerHTML = '';
  list.forEach(p => {
    const li = document.createElement('li');
    li.className = 'player';
    const left = document.createElement('span');
    left.textContent = p.name;
    if (p.isHost) left.textContent += ' 👑';
    const right = document.createElement('span');
    right.className = p.ready ? 'ready' : 'notready';
    right.textContent = p.ready ? 'READY' : 'NOT READY';
    if (p.cardComplete) right.textContent += ' ✓';
    if (status === 'playing' && p.bingoCount > 0) {
      right.textContent += ` · ${p.bingoProgress} (${p.bingoCount}/5)`;
    }
    li.append(left, right);
    playerList.appendChild(li);
  });
}

function renderPlayingPlayers() {
  playingPlayerList.innerHTML = '';
  players.forEach(p => {
    const li = document.createElement('li');
    li.className = 'player';
    const left = document.createElement('span');
    left.textContent = p.name + (p.isHost ? ' 👑' : '');
    const right = document.createElement('span');
    if (!p.connected) {
      right.textContent = p.reconnectSeconds ? `RECONNECTING (${p.reconnectSeconds}s)` : 'DISCONNECTED';
      right.className = 'notready';
    } else if (p.id === activePlayerId) {
      right.textContent = p.id === myPlayerId ? 'YOUR TURN' : 'TURN';
      right.className = 'turn';
    } else right.textContent = 'playing';
    li.append(left, right);
    playingPlayerList.appendChild(li);
  });
}

function renderFinalCards(cards, winnerId) {
  finalCards.innerHTML = '';
  const entries = Object.entries(cards);
  entries.sort((a, b) => {
    if (a[0] === winnerId) return -1;
    if (b[0] === winnerId) return 1;
    return 0;
  });

  entries.forEach(([id, raw]) => {
    const p = players.find(x => x.id === id);
    const wrap = document.createElement('div');
    wrap.className = 'panel mini-card' + (id === winnerId ? ' winner' : '');
    const title = document.createElement('h3');
    title.textContent = (p?.name || id) + (id === winnerId ? ' 🏆' : '');
    wrap.appendChild(title);
    const head = document.createElement('div');
    head.className = 'bingo-head';
    ['B', 'I', 'N', 'G', 'O'].forEach(x => { const d = document.createElement('div'); d.textContent = x; head.appendChild(d); });
    wrap.appendChild(head);
    const grid = document.createElement('div');
    grid.className = 'bingo-grid';
    const c = normalizeCard(raw);
    for (let r = 0; r < 5; r++) for (let col = 0; col < 5; col++) {
      const d = document.createElement('div');
      d.className = 'cell' + (calledNumbers.has(c[r][col]) ? ' marked' : '');
      d.textContent = c[r][col];
      grid.appendChild(d);
    }
    wrap.appendChild(grid);
    finalCards.appendChild(wrap);
  });
}

// Allow pressing Escape to clear the selected editor number.
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') { selectedNumber = null; selectedCell = null; renderLobby(); }
});

// A full refresh starts a fresh client session. Do not restore stale player,
// room, or game state from the previous page instance.
function initializeJoinScreen() {
  screenJoin.style.display = 'block';
  screenGame.style.display = 'none';
  joinError.textContent = '';
  roomCodeDisplay.textContent = '';
  statusDisplay.textContent = '';
  resetLocalState();
}

window.addEventListener('beforeunload', () => {
  if (ws && ws.readyState === WebSocket.OPEN) ws.close();
});

window.addEventListener('pageshow', event => {
  if (!event.persisted) return;
  leavingRoom = true;
  if (ws) ws.close();
  initializeJoinScreen();
  setTimeout(() => { leavingRoom = false; }, 0);
});

initializeJoinScreen();
