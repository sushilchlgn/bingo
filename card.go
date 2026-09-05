package main

// Card is a 5x5 Bingo card containing every number from 1 through 25 exactly once.
// There is no free space in this version of the game.
type Card [5][5]int

const (
	CardSize   = 5
	CardNumber = 25
	BingoLines = 5
)

// BingoLine identifies one completed row, column, or diagonal.
type BingoLine struct {
	Kind  string `json:"kind"`
	Index int    `json:"index"`
}

// validateCard returns nil only when the card contains exactly the numbers 1..25,
// with no duplicates and no empty/invalid cells.
func (c Card) validateCard() error {
	seen := [CardNumber + 1]bool{}

	for r := 0; r < CardSize; r++ {
		for col := 0; col < CardSize; col++ {
			n := c[r][col]
			if n < 1 || n > CardNumber {
				return errInvalidCard
			}
			if seen[n] {
				return errDuplicateNumber
			}
			seen[n] = true
		}
	}

	return nil
}

// validatePartialCard accepts empty cells while a player is editing, but still
// rejects out-of-range values and duplicates.
func (c Card) validatePartialCard() error {
	seen := [CardNumber + 1]bool{}
	for r := 0; r < CardSize; r++ {
		for col := 0; col < CardSize; col++ {
			n := c[r][col]
			if n == 0 {
				continue
			}
			if n < 1 || n > CardNumber {
				return errInvalidCard
			}
			if seen[n] {
				return errDuplicateNumber
			}
			seen[n] = true
		}
	}
	return nil
}

// isComplete is kept separate because the client may send a partially built card.
func (c Card) isComplete() bool {
	return c.validateCard() == nil
}

func (c Card) contains(number int) bool {
	if number < 1 || number > CardNumber {
		return false
	}
	for r := 0; r < CardSize; r++ {
		for col := 0; col < CardSize; col++ {
			if c[r][col] == number {
				return true
			}
		}
	}
	return false
}

// marks derives the player's marked cells from the server's canonical called set.
// The client never tells the server which cells are marked.
func (c Card) marks(called map[int]bool) [5][5]bool {
	var marked [5][5]bool
	for r := 0; r < CardSize; r++ {
		for col := 0; col < CardSize; col++ {
			marked[r][col] = called[c[r][col]]
		}
	}
	return marked
}

// completedLines returns every distinct completed row, column, and diagonal.
// A BINGO claim is valid only after five such lines have been completed. The
// server counts lines from the canonical called-number set, never from client
// supplied marks.
func (c Card) completedLines(called map[int]bool) []BingoLine {
	m := c.marks(called)
	lines := make([]BingoLine, 0, 12)

	for r := 0; r < CardSize; r++ {
		complete := true
		for col := 0; col < CardSize; col++ {
			if !m[r][col] {
				complete = false
				break
			}
		}
		if complete {
			lines = append(lines, BingoLine{Kind: "row", Index: r})
		}
	}

	for col := 0; col < CardSize; col++ {
		complete := true
		for r := 0; r < CardSize; r++ {
			if !m[r][col] {
				complete = false
				break
			}
		}
		if complete {
			lines = append(lines, BingoLine{Kind: "column", Index: col})
		}
	}

	mainDiagonal := true
	antiDiagonal := true
	for i := 0; i < CardSize; i++ {
		if !m[i][i] {
			mainDiagonal = false
		}
		if !m[i][CardSize-1-i] {
			antiDiagonal = false
		}
	}
	if mainDiagonal {
		lines = append(lines, BingoLine{Kind: "diagonal", Index: 0})
	}
	if antiDiagonal {
		lines = append(lines, BingoLine{Kind: "diagonal", Index: 1})
	}

	return lines
}

func (c Card) completedLineCount(called map[int]bool) int {
	return len(c.completedLines(called))
}

// hasBingo implements the requested B-I-N-G-O progression: one completed line
// advances the count by one, and five completed lines are required to win.
func (c Card) hasBingo(called map[int]bool) bool {
	return c.completedLineCount(called) >= BingoLines
}
