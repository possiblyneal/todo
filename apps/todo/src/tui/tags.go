package tui

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/possiblyneal/todo/apps/todo/src/store"
)

// rankTags orders Tags by how often they are carried, with a bit of variation
// thrown in so the sidebar does not read the same every time. The operator
// asked for discovery rather than a stable ranking: a Tag carried twice should
// usually sit above one carried once, and should sometimes sit below it.
//
// The draw is Efraimidis-Spirakis weighted sampling without replacement: each
// Tag gets the key u^(1/w) for a uniform u and its weight w, and the keys are
// ranked. The expected order is the frequency order, and every other order has
// a chance proportional to how close the counts are.
func rankTags(tags []store.Tag, r *rand.Rand) []store.Tag {
	keyed := make([]struct {
		tag store.Tag
		key float64
	}, len(tags))
	for i, tag := range tags {
		// A Tag nothing carries still has a weight, so it can surface.
		weight := float64(tag.Count) + 1
		keyed[i].tag = tag
		keyed[i].key = math.Pow(r.Float64(), 1/weight)
	}
	// Descending by key, stably, so two Tags drawing the same key keep the
	// order they were counted in rather than swapping about between draws.
	slices.SortStableFunc(keyed, func(a, b struct {
		tag store.Tag
		key float64
	}) int {
		return cmp.Compare(b.key, a.key)
	})
	ranked := make([]store.Tag, len(keyed))
	for i, k := range keyed {
		ranked[i] = k.tag
	}
	return ranked
}
