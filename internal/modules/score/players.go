package score

import (
	"fmt"
	"strings"
)

func trimPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

// normalizePartners blanks empty partner slots and rejects a user id without a name
// or the same account on more than one slot.
func normalizePartners(a2Name, b2Name, aID, bID, a2ID, b2ID *string) (na2, nb2, ia, ib, ia2, ib2 *string, err error) {
	na2, nb2 = trimPtr(a2Name), trimPtr(b2Name)
	ia, ib, ia2, ib2 = trimPtr(aID), trimPtr(bID), trimPtr(a2ID), trimPtr(b2ID)
	if ia2 != nil && na2 == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("%w: partner name required", ErrInvalid)
	}
	if ib2 != nil && nb2 == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("%w: partner name required", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, id := range []*string{ia, ib, ia2, ib2} {
		if id == nil {
			continue
		}
		if seen[*id] {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("%w: duplicate player", ErrInvalid)
		}
		seen[*id] = true
	}
	return na2, nb2, ia, ib, ia2, ib2, nil
}

func sideLabel(name string, partner *string) string {
	if partner == nil || strings.TrimSpace(*partner) == "" {
		return name
	}
	return name + " / " + strings.TrimSpace(*partner)
}

func idEq(id *string, userID string) bool {
	return id != nil && *id != "" && *id == userID
}

func containsID(ids []string, userID string) bool {
	for _, id := range ids {
		if id == userID {
			return true
		}
	}
	return false
}

// inviteeIDs is every registered player except the starter, in slot order.
func inviteeIDs(starter string, ids ...*string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id == nil || *id == "" || *id == starter || seen[*id] {
			continue
		}
		seen[*id] = true
		out = append(out, *id)
	}
	return out
}

func sideOf(m Match, userID string) string {
	if idEq(m.PlayerAUserID, userID) || idEq(m.PlayerA2UserID, userID) {
		return "A"
	}
	if idEq(m.PlayerBUserID, userID) || idEq(m.PlayerB2UserID, userID) {
		return "B"
	}
	return ""
}

type namedPlayer struct {
	id   string
	name string
}

func opponentsOf(m Match, userID string) []namedPlayer {
	side := sideOf(m, userID)
	var slots []namedPlayer
	switch side {
	case "A":
		slots = slotPlayers(m.PlayerBUserID, m.PlayerBName, m.PlayerB2UserID, m.PlayerB2Name)
	case "B":
		slots = slotPlayers(m.PlayerAUserID, m.PlayerAName, m.PlayerA2UserID, m.PlayerA2Name)
	}
	return slots
}

func slotPlayers(id1 *string, name1 string, id2 *string, name2 *string) []namedPlayer {
	var out []namedPlayer
	if id1 != nil && *id1 != "" {
		out = append(out, namedPlayer{id: *id1, name: name1})
	}
	if id2 != nil && *id2 != "" {
		n := ""
		if name2 != nil {
			n = *name2
		}
		out = append(out, namedPlayer{id: *id2, name: n})
	}
	return out
}

func liveInvitees(m LiveMatch) []string {
	return inviteeIDs(m.CreatedBy, m.PlayerAUserID, m.PlayerA2UserID, m.PlayerBUserID, m.PlayerB2UserID)
}

func remainingInvitees(m LiveMatch) []string {
	var left []string
	for _, id := range liveInvitees(m) {
		if !containsID(m.DeclinedUserIDs, id) {
			left = append(left, id)
		}
	}
	return left
}

func matchHasUser(m Match, userID string) bool {
	return idEq(m.PlayerAUserID, userID) || idEq(m.PlayerA2UserID, userID) ||
		idEq(m.PlayerBUserID, userID) || idEq(m.PlayerB2UserID, userID)
}

func liveUserInSlots(userID string, m LiveMatch) bool {
	return idEq(m.PlayerAUserID, userID) || idEq(m.PlayerA2UserID, userID) ||
		idEq(m.PlayerBUserID, userID) || idEq(m.PlayerB2UserID, userID)
}
