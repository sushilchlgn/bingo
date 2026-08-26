package main

import "testing"

func TestGenerateCardContainsEveryNumberOnce(t *testing.T) {
	card := generateCard()

	seen := make(map[int]bool)

	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			number := card[row][col]

			if number < 1 || number > 25 {
				t.Fatalf(
					"invalid number %d at [%d][%d]",
					number,
					row,
					col,
				)
			}

			if seen[number] {
				t.Fatalf(
					"duplicate number %d",
					number,
				)
			}

			seen[number] = true
		}
	}

	if len(seen) != 25 {
		t.Fatalf(
			"expected 25 unique numbers, got %d",
			len(seen),
		)
	}

	for number := 1; number <= 25; number++ {
		if !seen[number] {
			t.Fatalf(
				"number %d is missing",
				number,
			)
		}
	}
}

func TestGenerateDifferentCardsCanHaveDifferentLayouts(t *testing.T) {
	card1 := generateCard()
	card2 := generateCard()

	same := true

	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			if card1[row][col] != card2[row][col] {
				same = false
				break
			}
		}

		if !same {
			break
		}
	}

	if same {
		t.Log(
			"two generated cards happened to have the same layout; extremely unlikely but valid",
		)
	}
}

func TestBingoRows(t *testing.T) {
	card := Card{
		{1, 2, 3, 4, 5},
		{6, 7, 8, 9, 10},
		{11, 12, 13, 14, 15},
		{16, 17, 18, 19, 20},
		{21, 22, 23, 24, 25},
	}

	called := map[int]bool{
		1: true,
		2: true,
		3: true,
		4: true,
		5: true,
	}

	if !card.hasBingo(called) {
		t.Fatal("expected first row to be Bingo")
	}
}

func TestBingoColumn(t *testing.T) {
	card := Card{
		{1, 2, 3, 4, 5},
		{6, 7, 8, 9, 10},
		{11, 12, 13, 14, 15},
		{16, 17, 18, 19, 20},
		{21, 22, 23, 24, 25},
	}

	called := map[int]bool{
		1:  true,
		6:  true,
		11: true,
		16: true,
		21: true,
	}

	if !card.hasBingo(called) {
		t.Fatal("expected first column to be Bingo")
	}
}

func TestBingoDiagonal(t *testing.T) {
	card := Card{
		{1, 2, 3, 4, 5},
		{6, 7, 8, 9, 10},
		{11, 12, 13, 14, 15},
		{16, 17, 18, 19, 20},
		{21, 22, 23, 24, 25},
	}

	called := map[int]bool{
		1:  true,
		7:  true,
		13: true,
		19: true,
		25: true,
	}

	if !card.hasBingo(called) {
		t.Fatal("expected diagonal to be Bingo")
	}
}

func TestNoBingo(t *testing.T) {
	card := Card{
		{1, 2, 3, 4, 5},
		{6, 7, 8, 9, 10},
		{11, 12, 13, 14, 15},
		{16, 17, 18, 19, 20},
		{21, 22, 23, 24, 25},
	}

	called := map[int]bool{
		1: true,
		2: true,
		3: true,
	}

	if card.hasBingo(called) {
		t.Fatal("expected no Bingo")
	}
}