package dom

import "github.com/yogisalomo/goowee/core"

// SplitDraws separates a batch's canvas draws (core.MutDraw) from the rest.
// The bridge applies the rest as one batch and then sends each draw's buffer
// as bytes, so the batch encoders never see a draw. With no draw in muts it
// returns muts, nil without allocating.
func SplitDraws(muts []core.Mutation) (rest, draws []core.Mutation) {
	first := -1
	for i := range muts {
		if muts[i].Type == core.MutDraw {
			first = i
			break
		}
	}
	if first < 0 {
		return muts, nil
	}
	rest = make([]core.Mutation, 0, len(muts)-1)
	rest = append(rest, muts[:first]...)
	for _, m := range muts[first:] {
		if m.Type == core.MutDraw {
			draws = append(draws, m)
		} else {
			rest = append(rest, m)
		}
	}
	return rest, draws
}
