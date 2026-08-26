package main

import "math/rand"

// Card is a 5x5 Bingo card.
//
// Every card contains every number from 1 to 25 exactly once.
// The arrangement is randomized for each player.
//
// There is NO free space.
type Card [5][5]int

// generateCard creates a randomized 5x5 card containing
// every number from 1 through 25 exactly once.
func generateCard() Card {
	var card Card

	numbers := rand.Perm(25)

	index := 0

	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			card[row][col] = numbers[index] + 1
			index++
		}
	}

	return card
}

// marks returns a 5x5 grid indicating which numbers
// have already been called.
//
// This will be used in later phases.
func (c Card) marks(called map[int]bool) [5][5]bool {
	var marked [5][5]bool

	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			number := c[row][col]

			if called[number] {
				marked[row][col] = true
			}
		}
	}

	return marked
}

// hasBingo checks the standard 5x5 winning patterns:
//
// - 5 rows
// - 5 columns
// - top-left to bottom-right diagonal
// - top-right to bottom-left diagonal
//
// This is included now so Phase 2/3 can use it.
// Phase 1 does not call this function yet.
func (c Card) hasBingo(called map[int]bool) bool {
	marked := c.marks(called)

	// Rows.
	for row := 0; row < 5; row++ {
		complete := true

		for col := 0; col < 5; col++ {
			if !marked[row][col] {
				complete = false
				break
			}
		}

		if complete {
			return true
		}
	}

	// Columns.
	for col := 0; col < 5; col++ {
		complete := true

		for row := 0; row < 5; row++ {
			if !marked[row][col] {
				complete = false
				break
			}
		}

		if complete {
			return true
		}
	}

	// Main diagonal.
	complete := true

	for i := 0; i < 5; i++ {
		if !marked[i][i] {
			complete = false
			break
		}
	}

	if complete {
		return true
	}

	// Opposite diagonal.
	complete = true

	for i := 0; i < 5; i++ {
		if !marked[i][4-i] {
			complete = false
			break
		}
	}

	return complete
}