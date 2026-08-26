// --- State ---
let ws = null;
let myPlayerId = null;
let isHost = false;
let card = null;              // 5x5 array from server
let calledNumbers = new Set();
let roomCode = null;

// --- DOM refs ---
const screenJoin = document.getElementById('screen-join');
const screenGame = document.getElementById('screen-game');
const nameInput = document.getElementById('nameInput');
const codeInput = document.getElementById('codeInput');
const createBtn = document.getElementById('createBtn');
const joinBtn = document.getElementById('joinBtn');
const joinError = document.getElementById('joinError');

const roomCodeDisplay = document.getElementById('roomCodeDisplay');
const statusDisplay = document.getElementById('statusDisplay');
const hostControls = document.getElementById('hostControls');
const startBtn = document.getElementById('startBtn');
const callBtn = document.getElementById('callBtn');
const bingoBtn = document.getElementById('bingoBtn');
const grid = document.getElementById('grid');
const lastCalled = document.getElementById('lastCalled');
const callHistory = document.getElementById('callHistory');
const playerList = document.getElementById('playerList');
const toast = document.getElementById('toast');

function showToast(msg, ms = 3000) {
  toast.textContent = msg;
  toast.style.display = 'block';
  clearTimeout(showToast._t);
  showToast._t = setTimeout(() => (toast.style.display = 'none'), ms);
}

// --- Room creation / joining ---
createBtn.onclick = async () => {
  const name = nameInput.value.trim();
  if (!name) { joinError.textContent = 'Enter your name first.'; return; }
  const res = await fetch('/api/rooms', { method: 'POST' });
  const data = await res.json();
  connect(data.code, name);
};

joinBtn.onclick = () => {
  const name = nameInput.value.trim();
  const code = codeInput.value.trim().toUpperCase();
  if (!name) { joinError.textContent = 'Enter your name first.'; return; }
  if (!code) { joinError.textContent = 'Enter a room code.'; return; }
  connect(code, name);
};

function connect(code, name) {
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${protocol}://${location.host}/ws?room=${encodeURIComponent(code)}&name=${encodeURIComponent(name)}`);

  ws.onopen = () => { joinError.textContent = ''; };
  ws.onerror = () => { joinError.textContent = 'Could not connect. Check the room code.'; };
  ws.onclose = () => { showToast('Disconnected from server.'); };
  ws.onmessage = (evt) => handleMessage(JSON.parse(evt.data));
}

// --- Message handling ---
function handleMessage(msg) {
  switch (msg.type) {
    case 'welcome':
      myPlayerId = msg.playerId;
      isHost = msg.isHost;
      card = msg.card;
      roomCode = msg.roomCode;
      enterGameScreen();
      break;

    case 'room_state':
      renderPlayers(msg.players);
      statusDisplay.textContent = statusLabel(msg.status);
      const me = msg.players.find(p => p.id === myPlayerId);
      if (me) isHost = me.isHost;
      hostControls.style.display = isHost ? 'block' : 'none';
      startBtn.style.display = (isHost && msg.status === 'waiting') ? 'inline-block' : 'none';
      callBtn.style.display = (isHost && msg.status === 'playing') ? 'inline-block' : 'none';
      break;

    case 'call_history':
      calledNumbers = new Set(msg.calledNumbers);
      renderCallHistory(msg.calledNumbers);
      renderGrid();
      break;

    case 'number_called':
      calledNumbers.add(msg.number);
      lastCalled.textContent = formatCall(msg.number);
      renderCallHistory(msg.calledNumbers);
      renderGrid();
      bingoBtn.disabled = false;
      break;

    case 'bingo_result':
      if (!msg.valid) showToast(msg.message || 'Not a bingo yet.');
      break;

    case 'game_over':
      statusDisplay.textContent = `🏆 ${msg.winnerName} won!`;
      showToast(msg.winnerId === myPlayerId ? '🎉 You got BINGO!' : `${msg.winnerName} got BINGO!`, 6000);
      bingoBtn.disabled = true;
      callBtn.style.display = 'none';
      break;

    case 'promoted_to_host':
      isHost = true;
      showToast('The host left — you are now the host.');
      hostControls.style.display = 'block';
      break;
  }
}

function statusLabel(s) {
  if (s === 'waiting') return 'Waiting for host to start…';
  if (s === 'playing') return 'Game in progress';
  if (s === 'finished') return 'Game over';
  return '';
}

function formatCall(n) {
  const col = Math.floor((n - 1) / 15);
  const letter = ['B', 'I', 'N', 'G', 'O'][col];
  return `${letter}-${n}`;
}

// --- Rendering ---
function enterGameScreen() {
  screenJoin.style.display = 'none';
  screenGame.style.display = 'block';
  roomCodeDisplay.textContent = roomCode;
  renderGrid();
}

function renderGrid() {
  grid.innerHTML = '';
  for (let r = 0; r < 5; r++) {
    for (let c = 0; c < 5; c++) {
      const val = card[r][c];
      const div = document.createElement('div');
      div.className = 'cell';
      if (val === 0) {
        div.classList.add('free');
        div.textContent = '★';
      } else {
        div.textContent = val;
        if (calledNumbers.has(val)) div.classList.add('marked');
      }
      grid.appendChild(div);
    }
  }
}

function renderCallHistory(order) {
  callHistory.innerHTML = '';
  order.slice().reverse().forEach(n => {
    const span = document.createElement('span');
    span.textContent = formatCall(n);
    callHistory.appendChild(span);
  });
  lastCalled.textContent = order.length ? formatCall(order[order.length - 1]) : '—';
}

function renderPlayers(players) {
  playerList.innerHTML = '';
  players.forEach(p => {
    const li = document.createElement('li');
    li.textContent = p.name;
    if (p.isHost) {
      const tag = document.createElement('span');
      tag.className = 'host-tag';
      tag.textContent = '(host)';
      li.appendChild(tag);
    }
    playerList.appendChild(li);
  });
}

// --- Controls ---
startBtn.onclick = () => ws.send(JSON.stringify({ type: 'start_game' }));
callBtn.onclick = () => ws.send(JSON.stringify({ type: 'call_number' }));
bingoBtn.onclick = () => ws.send(JSON.stringify({ type: 'claim_bingo' }));
