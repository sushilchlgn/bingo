package main

import (
	"math/rand"
)

// Card is a 5x5 grid of numbers. Column ranges follow classic BINGO rules:
// B: 1-15, I: 16-30, N: 31-45 (center is free), G: 46-60, O: 61-75.
type Card [5][5]int

const freeSpace = 0 // sentinel for the free center cell

// generateCard produces a fresh, valid, randomized bingo card.
func generateCard() Card {
	var c Card
	for col := 0; col < 5; col++ {
		low := col*15 + 1
		high := col*15 + 15
		nums := rand.Perm(high - low + 1) // 0..14 shuffled
		picked := 0
		for row := 0; row < 5; row++ {
			if col == 2 && row == 2 {
				c[row][col] = freeSpace
				continue
			}
			c[row][col] = low + nums[picked]
			picked++
		}
	}
	return c
}

// marks returns a 5x5 bool grid: true where the card's number has been
// called (or is the free space). This is recomputed server-side from the
// canonical called-numbers set, so a client can never fake a mark.
func (c Card) marks(called map[int]bool) [5][5]bool {
	var m [5][5]bool
	for r := 0; r < 5; r++ {
		for col := 0; col < 5; col++ {
			if c[r][col] == freeSpace || called[c[r][col]] {
				m[r][col] = true
			}
		}
	}
	return m
}

// hasBingo checks all standard win patterns: 5 rows, 5 columns, 2 diagonals.
func (c Card) hasBingo(called map[int]bool) bool {
	m := c.marks(called)

	for r := 0; r < 5; r++ {
		if m[r][0] && m[r][1] && m[r][2] && m[r][3] && m[r][4] {
			return true
		}
	}
	for col := 0; col < 5; col++ {
		if m[0][col] && m[1][col] && m[2][col] && m[3][col] && m[4][col] {
			return true
		}
	}
	if m[0][0] && m[1][1] && m[2][2] && m[3][3] && m[4][4] {
		return true
	}
	if m[0][4] && m[1][3] && m[2][2] && m[3][1] && m[4][0] {
		return true
	}
	return false
}
