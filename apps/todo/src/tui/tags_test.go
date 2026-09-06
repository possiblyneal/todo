package tui

import (
	"math/rand/v2"
	"testing"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// TestTagsRankByFrequencyWithVariation pins both halves of what the operator
// asked for: the sidebar usually leads with the Tag carried most, and it does
// not read the same every time.
func TestTagsRankByFrequencyWithVariation(t *testing.T) {
	tags := []store.Tag{
		{ID: "a", Name: "often", Count: 20},
		{ID: "b", Name: "sometimes", Count: 5},
		{ID: "c", Name: "rarely", Count: 1},
		{ID: "d", Name: "never", Count: 0},
	}

	r := rand.New(rand.NewPCG(7, 11))
	const draws = 500
	leads, orders := 0, map[string]bool{}
	for range draws {
		ranked := rankTags(tags, r)
		if len(ranked) != len(tags) {
			t.Fatalf("a draw returned %d Tags, want all %d", len(ranked), len(tags))
		}
		var order string
		for _, tag := range ranked {
			order += tag.ID
		}
		orders[order] = true
		if ranked[0].ID == "a" {
			leads++
		}
	}

	if leads <= draws/2 {
		t.Errorf("the most-carried Tag led %d of %d draws, want it usually first", leads, draws)
	}
	if leads == draws {
		t.Error("the most-carried Tag led every draw, want some variation")
	}
	if len(orders) < 4 {
		t.Errorf("the draws produced %d orders, want the sidebar to vary", len(orders))
	}
}

// TestATagNobodyCarriesCanStillSurface keeps a new Tag reachable: a count of
// zero is a weight of one, not a weight of none.
func TestATagNobodyCarriesCanStillSurface(t *testing.T) {
	tags := []store.Tag{{ID: "carried", Count: 3}, {ID: "new", Count: 0}}

	r := rand.New(rand.NewPCG(3, 5))
	for range 200 {
		if rankTags(tags, r)[0].ID == "new" {
			return
		}
	}
	t.Error("a Tag nothing carries never surfaced in 200 draws")
}
