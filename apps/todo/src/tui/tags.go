package tui

import (
	"math"
	"math/rand/v2"

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
	// Insertion sort by key, descending: the sidebar holds tens of Tags, not
	// thousands, and this keeps the draw's order stable for equal keys.
	for i := 1; i < len(keyed); i++ {
		for j := i; j > 0 && keyed[j].key > keyed[j-1].key; j-- {
			keyed[j], keyed[j-1] = keyed[j-1], keyed[j]
		}
	}
	ranked := make([]store.Tag, len(keyed))
	for i, k := range keyed {
		ranked[i] = k.tag
	}
	return ranked
}
