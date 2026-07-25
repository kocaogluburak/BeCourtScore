package tournament

// MatchPlan is a single fixture produced by the draw, before it is persisted.
// A and B hold participant IDs; an empty string means an empty/BYE slot.
// NextRound/NextPos point to the match the winner advances into (-1 = none).
type MatchPlan struct {
	Round     int
	Position  int
	A         string
	B         string
	Winner    string // pre-decided for BYE matches
	Status    string // READY | BYE | PENDING
	NextRound int
	NextPos   int
}

func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// buildSingleElim generates a single-elimination bracket from an ordered list
// of participant IDs (already shuffled for RANDOM, or as-listed for MANUAL).
// Real matches are placed first (top of the list), BYEs fall to the tail; the
// lone participant of a BYE match is auto-advanced into the next round.
func buildSingleElim(ordered []string) []MatchPlan {
	n := len(ordered)
	if n < 2 {
		return nil
	}
	size := nextPow2(n)

	rounds := 0
	for s := size; s > 1; s >>= 1 {
		rounds++
	}

	type node struct {
		a, b, winner, status string
	}
	grid := make([][]*node, rounds+1) // 1-indexed by round
	for r := 1; r <= rounds; r++ {
		cnt := size >> r
		grid[r] = make([]*node, cnt)
		for p := 0; p < cnt; p++ {
			grid[r][p] = &node{status: "PENDING"}
		}
	}

	byes := size - n
	real := n - size/2 // number of two-player matches in round 1
	idx := 0
	for m := 0; m < real; m++ {
		grid[1][m].a = ordered[idx]
		idx++
		grid[1][m].b = ordered[idx]
		idx++
		grid[1][m].status = "READY"
	}
	for m := real; m < size/2; m++ {
		grid[1][m].a = ordered[idx]
		idx++
		grid[1][m].status = "BYE"
		grid[1][m].winner = grid[1][m].a
	}
	_ = byes

	// Advance BYE winners into round 2 and mark newly-complete matches READY.
	if rounds >= 2 {
		for pos, nd := range grid[1] {
			if nd.winner == "" {
				continue
			}
			parent := grid[2][pos/2]
			if pos%2 == 0 {
				parent.a = nd.winner
			} else {
				parent.b = nd.winner
			}
		}
		for _, nd := range grid[2] {
			if nd.status == "PENDING" && nd.a != "" && nd.b != "" {
				nd.status = "READY"
			}
		}
	}

	var plans []MatchPlan
	for r := 1; r <= rounds; r++ {
		for pos, nd := range grid[r] {
			mp := MatchPlan{
				Round:     r,
				Position:  pos,
				A:         nd.a,
				B:         nd.b,
				Winner:    nd.winner,
				Status:    nd.status,
				NextRound: -1,
				NextPos:   -1,
			}
			if r < rounds {
				mp.NextRound = r + 1
				mp.NextPos = pos / 2
			}
			plans = append(plans, mp)
		}
	}
	return plans
}

// buildRoundRobin generates a single round-robin schedule using the circle
// method so no participant plays twice in the same round.
func buildRoundRobin(ordered []string) []MatchPlan {
	n := len(ordered)
	if n < 2 {
		return nil
	}

	players := make([]string, len(ordered))
	copy(players, ordered)
	if len(players)%2 == 1 {
		players = append(players, "") // dummy = BYE for that round
	}
	size := len(players)
	rounds := size - 1

	var plans []MatchPlan
	for r := 1; r <= rounds; r++ {
		pos := 0
		for i := 0; i < size/2; i++ {
			home := players[i]
			away := players[size-1-i]
			if home == "" || away == "" {
				continue
			}
			plans = append(plans, MatchPlan{
				Round:     r,
				Position:  pos,
				A:         home,
				B:         away,
				Status:    "READY",
				NextRound: -1,
				NextPos:   -1,
			})
			pos++
		}
		// Rotate: keep players[0] fixed, move last into position 1.
		last := players[size-1]
		copy(players[2:], players[1:size-1])
		players[1] = last
	}
	return plans
}
