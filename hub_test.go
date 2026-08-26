package main

import "testing"

func TestOldestPlayer(t *testing.T) {
	r := newRoom("ABCDE")
	p1 := &Player{ID: "1", JoinSeq: 1}
	p2 := &Player{ID: "2", JoinSeq: 2}
	p3 := &Player{ID: "3", JoinSeq: 3}
	r.Players[p3.ID] = p3
	r.Players[p1.ID] = p1
	r.Players[p2.ID] = p2
	if got := r.oldestPlayer(); got != p1 {
		t.Fatalf("oldestPlayer() = %v, want p1", got.ID)
	}
}

func TestRoomStateTransition(t *testing.T) {
	r := newRoom("ABCDE")
	r.Players["1"] = &Player{ID: "1", IsHost: true}
	r.Players["2"] = &Player{ID: "2"}
	if r.Status != StatusWaiting {
		t.Fatalf("initial status = %q", r.Status)
	}
	r.Status = StatusPlaying
	r.RoundID++
	if r.Status != StatusPlaying || r.RoundID != 1 {
		t.Fatalf("playing state not established")
	}
	r.Status = StatusFinished
	if r.Status != StatusFinished {
		t.Fatalf("finished state not established")
	}
}
