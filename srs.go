package main

import "time"
import "sort"

var boxIntervals = map[int]time.Duration{
	1: 0,
	2: 24 * time.Hour,
	3: 3 * 24 * time.Hour,
	4: 7 * 24 * time.Hour,
	5: 21 * 24 * time.Hour,
}

func Grade(card *Card, correct bool, now time.Time) {
	card.Touch(now)
	if correct {
		if card.Box < 5 {
			card.Box++
		}
		card.Streak++
	} else {
		if card.Box > 1 {
			card.Box--
		}
		if card.Streak > 0 {
			card.Streak = 0
		}
	}
	card.NextDue = now.Add(boxIntervals[card.Box])
}

func DueCards(cards []Card, now time.Time) []Card {
	out := []Card{}
	for _, c := range cards {
		if c.Due(now) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SeenCount > out[j].SeenCount })
	return out
}
