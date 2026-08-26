// ============================================================
// CONNECTION STATE
// ============================================================

let ws = null;

let myPlayerId = null;

let isHost = false;

let myReady = false;

let card = null;

let roomCode = null;

let roundId = 0;


// ============================================================
// DOM
// ============================================================

const screenJoin =
  document.getElementById("screen-join");

const screenGame =
  document.getElementById("screen-game");

const nameInput =
  document.getElementById("nameInput");

const codeInput =
  document.getElementById("codeInput");

const createBtn =
  document.getElementById("createBtn");

const joinBtn =
  document.getElementById("joinBtn");

const joinError =
  document.getElementById("joinError");

const roomCodeDisplay =
  document.getElementById("roomCodeDisplay");

const statusDisplay =
  document.getElementById("statusDisplay");

const hostControls =
  document.getElementById("hostControls");

const readyBtn =
  document.getElementById("readyBtn");

const startBtn =
  document.getElementById("startBtn");

const waitingArea =
  document.getElementById("waitingArea");

const gameArea =
  document.getElementById("gameArea");

const readyStatus =
  document.getElementById("readyStatus");

const grid =
  document.getElementById("grid");

const playerList =
  document.getElementById("playerList");

const roomStatus =
  document.getElementById("roomStatus");

const toast =
  document.getElementById("toast");


// ============================================================
// TOAST
// ============================================================

function showToast(message, duration = 3000) {
  toast.textContent = message;

  toast.style.display = "block";

  clearTimeout(showToast.timer);

  showToast.timer = setTimeout(() => {
    toast.style.display = "none";
  }, duration);
}


// ============================================================
// CREATE ROOM
// ============================================================

createBtn.onclick = async () => {
  const name = nameInput.value.trim();

  if (!name) {
    joinError.textContent =
      "Enter your name first.";

    return;
  }

  createBtn.disabled = true;

  try {
    const response = await fetch(
      "/api/rooms",
      {
        method: "POST",
      }
    );

    if (!response.ok) {
      throw new Error(
        "Could not create room."
      );
    }

    const data = await response.json();

    connect(
      data.code,
      name
    );

  } catch (error) {
    joinError.textContent =
      error.message ||
      "Could not create room.";

    createBtn.disabled = false;
  }
};


// ============================================================
// JOIN ROOM
// ============================================================

joinBtn.onclick = () => {
  const name =
    nameInput.value.trim();

  const code =
    codeInput.value
      .trim()
      .toUpperCase();

  if (!name) {
    joinError.textContent =
      "Enter your name first.";

    return;
  }

  if (!code) {
    joinError.textContent =
      "Enter a room code.";

    return;
  }

  connect(
    code,
    name
  );
};


// ============================================================
// WEBSOCKET CONNECTION
// ============================================================

function connect(code, name) {

  const protocol =
    location.protocol === "https:"
      ? "wss"
      : "ws";

  const url =
    `${protocol}://${location.host}/ws` +
    `?room=${encodeURIComponent(code)}` +
    `&name=${encodeURIComponent(name)}`;

  ws = new WebSocket(url);

  ws.onopen = () => {
    joinError.textContent = "";

    showToast(
      "Connected to room."
    );
  };

  ws.onerror = () => {
    joinError.textContent =
      "Could not connect to the room.";

    createBtn.disabled = false;
  };

  ws.onclose = () => {
    showToast(
      "Disconnected from server."
    );
  };

  ws.onmessage = event => {

    let message;

    try {
      message =
        JSON.parse(event.data);

    } catch {
      showToast(
        "Received invalid server message."
      );

      return;
    }

    handleMessage(message);
  };
}


// ============================================================
// MESSAGE HANDLING
// ============================================================

function handleMessage(msg) {

  switch (msg.type) {

    // --------------------------------------------------------
    // WELCOME
    // --------------------------------------------------------

    case "welcome":

      myPlayerId =
        msg.playerId;

      isHost =
        msg.isHost;

      myReady =
        msg.ready;

      roomCode =
        msg.roomCode;

      roundId =
        Number(msg.roundId || 0);

      card =
        msg.card || null;

      enterGameScreen();

      break;


    // --------------------------------------------------------
    // ROOM STATE
    // --------------------------------------------------------

    case "room_state":

      if (
        msg.roundId !== undefined &&
        Number(msg.roundId) < roundId
      ) {
        return;
      }

      if (
        msg.roundId !== undefined
      ) {
        roundId =
          Number(msg.roundId);
      }

      renderPlayers(
        msg.players || []
      );

      updateRoomState(
        msg.status
      );

      break;


    // --------------------------------------------------------
    // GAME STARTED
    // --------------------------------------------------------

    case "game_started":

      roundId =
        Number(
          msg.roundId ||
          roundId
        );

      showToast(
        "🎮 Game started!"
      );

      updateRoomState(
        "playing"
      );

      renderGrid();

      break;


    // --------------------------------------------------------
    // PROMOTED TO HOST
    // --------------------------------------------------------

    case "promoted_to_host":

      isHost = true;

      updateHostControls();

      showToast(
        "You are now the host."
      );

      break;


    // --------------------------------------------------------
    // ERROR
    // --------------------------------------------------------

    case "error":

      showToast(
        msg.message ||
        "Something went wrong."
      );

      break;


    default:

      console.warn(
        "Unknown message:",
        msg
      );
  }
}


// ============================================================
// ENTER GAME SCREEN
// ============================================================

function enterGameScreen() {

  screenJoin.classList.add(
    "hidden"
  );

  screenGame.classList.remove(
    "hidden"
  );

  roomCodeDisplay.textContent =
    roomCode;

  renderGrid();

  updateHostControls();
}


// ============================================================
// ROOM STATUS
// ============================================================

function updateRoomState(status) {

  if (status === "waiting") {

    statusDisplay.textContent =
      "Waiting for players...";

    roomStatus.textContent =
      "Waiting for players";

    waitingArea.classList.remove(
      "hidden"
    );

    gameArea.classList.add(
      "hidden"
    );

  }

  else if (status === "playing") {

    statusDisplay.textContent =
      "Game in progress";

    roomStatus.textContent =
      "Game in progress";

    waitingArea.classList.add(
      "hidden"
    );

    gameArea.classList.remove(
      "hidden"
    );

  }

  else if (status === "finished") {

    statusDisplay.textContent =
      "Game finished";

    roomStatus.textContent =
      "Game finished";

    waitingArea.classList.add(
      "hidden"
    );

    gameArea.classList.remove(
      "hidden"
    );
  }

  updateHostControls();
}


// ============================================================
// HOST CONTROLS
// ============================================================

function updateHostControls() {

  if (isHost) {

    hostControls.classList.remove(
      "hidden"
    );

  } else {

    hostControls.classList.add(
      "hidden"
    );
  }

  readyBtn.textContent =
    myReady
      ? "Not Ready"
      : "Ready";

  // The Start button itself is only useful for host.
  if (isHost) {
    startBtn.classList.remove(
      "hidden"
    );
  } else {
    startBtn.classList.add(
      "hidden"
    );
  }
}


// ============================================================
// READY BUTTON
// ============================================================

readyBtn.onclick = () => {

  if (!ws) {
    return;
  }

  if (
    ws.readyState !==
    WebSocket.OPEN
  ) {
    showToast(
      "Not connected."
    );

    return;
  }

  ws.send(
    JSON.stringify({
      type: "set_ready",
    })
  );
};


// ============================================================
// START BUTTON
// ============================================================

startBtn.onclick = () => {

  if (!ws) {
    return;
  }

  if (
    ws.readyState !==
    WebSocket.OPEN
  ) {
    showToast(
      "Not connected."
    );

    return;
  }

  ws.send(
    JSON.stringify({
      type: "start_game",
      roundId: roundId,
    })
  );
};


// ============================================================
// PLAYER LIST
// ============================================================

function renderPlayers(players) {

  playerList.innerHTML = "";

  const me =
    players.find(
      player =>
        player.id === myPlayerId
    );

  if (me) {

    myReady =
      Boolean(me.ready);

    isHost =
      Boolean(me.isHost);
  }

  for (const player of players) {

    const li =
      document.createElement("li");

    li.textContent =
      player.name;

    if (player.isHost) {

      const hostTag =
        document.createElement("span");

      hostTag.className =
        "player-host";

      hostTag.textContent =
        "(host)";

      li.appendChild(
        hostTag
      );
    }

    const readyTag =
      document.createElement("span");

    if (player.ready) {

      readyTag.className =
        "player-ready";

      readyTag.textContent =
        "✓ ready";

    } else {

      readyTag.className =
        "player-not-ready";

      readyTag.textContent =
        "not ready";
    }

    li.appendChild(
      readyTag
    );

    playerList.appendChild(
      li
    );
  }

  updateReadyStatus(
    players
  );

  updateHostControls();
}


// ============================================================
// READY STATUS
// ============================================================

function updateReadyStatus(players) {

  if (!players.length) {
    readyStatus.textContent = "";
    return;
  }

  const readyCount =
    players.filter(
      player =>
        player.ready
    ).length;

  const total =
    players.length;

  if (readyCount === total) {

    readyStatus.textContent =
      `✓ All ${total} players are ready.`;

  } else {

    readyStatus.textContent =
      `${readyCount}/${total} players ready.`;
  }
}


// ============================================================
// CARD
// ============================================================

function renderGrid() {

  grid.innerHTML = "";

  if (!card) {
    return;
  }

  for (
    let row = 0;
    row < 5;
    row++
  ) {

    for (
      let col = 0;
      col < 5;
      col++
    ) {

      const number =
        card[row][col];

      const cell =
        document.createElement("div");

      cell.className =
        "cell";

      cell.textContent =
        number;

      grid.appendChild(
        cell
      );
    }
  }
}


// ============================================================
// ROOM CODE COPY
// ============================================================

roomCodeDisplay.onclick = async () => {

  if (!roomCode) {
    return;
  }

  try {

    await navigator.clipboard.writeText(
      roomCode
    );

    showToast(
      "Room code copied."
    );

  } catch {

    showToast(
      `Room code: ${roomCode}`
    );
  }
};


// ============================================================
// ENTER KEY SUPPORT
// ============================================================

nameInput.addEventListener(
  "keydown",
  event => {

    if (
      event.key === "Enter"
    ) {
      createBtn.click();
    }
  }
);

codeInput.addEventListener(
  "keydown",
  event => {

    if (
      event.key === "Enter"
    ) {
      joinBtn.click();
    }
  }
);