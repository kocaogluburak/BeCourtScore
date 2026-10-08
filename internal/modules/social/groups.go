package social

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Group is a private invite-code group. The weekly ladder is derived
// from finished matches; it is not stored on the group.
type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Sport       string    `json:"sport"`
	OwnerID     string    `json:"owner_id"`
	InviteCode  string    `json:"invite_code"`
	MemberCount int       `json:"member_count"`
	IsOwner     bool      `json:"is_owner"`
	CreatedAt   time.Time `json:"created_at"`
}

// GroupMember is one current member.
type GroupMember struct {
	UserID   string    `json:"user_id"`
	Name     string    `json:"name"`
	Nickname *string   `json:"nickname,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
	IsOwner  bool      `json:"is_owner"`
}

// Standing is one row of this ISO week's ladder.
type Standing struct {
	Rank    int    `json:"rank"`
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
	Wins    int    `json:"wins"`
	Losses  int    `json:"losses"`
	Matches int    `json:"matches"`
	SetDiff int    `json:"set_diff"`
}

// GroupDetail is the group plus members and this week's standings.
// Visible only to members.
type GroupDetail struct {
	Group
	Members   []GroupMember `json:"members"`
	Standings []Standing    `json:"standings"`
	WeekStart time.Time     `json:"week_start"`
	WeekEnd   time.Time     `json:"week_end"`
}

type groupStore interface {
	insertGroup(ctx context.Context, name, sport, ownerID, code string) (Group, error)
	listGroups(ctx context.Context, userID string) ([]Group, error)
	getGroup(ctx context.Context, id string) (Group, error)
	getGroupByCode(ctx context.Context, code string) (Group, error)
	addMember(ctx context.Context, groupID, userID string) error
	isMember(ctx context.Context, groupID, userID string) (bool, error)
	removeMember(ctx context.Context, groupID, userID string) error
	deleteGroup(ctx context.Context, groupID string) error
	listMembers(ctx context.Context, groupID string) ([]GroupMember, error)
	listWeekMatches(ctx context.Context, groupID, sport string, start, end time.Time) ([]ladderMatch, error)
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Service) inviteCode() string {
	if s.code != nil {
		return s.code()
	}
	return randomGroupCode(6)
}

// CreateGroup makes a group and adds the caller as owner and member.
// Invite-code collisions are retried; a persistent collision is ErrConflict.
func (s *Service) CreateGroup(ctx context.Context, userID, name, sport string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 40 {
		return Group{}, fmt.Errorf("%w: name", ErrInvalid)
	}
	sport = strings.ToUpper(strings.TrimSpace(sport))
	if !validGroupSport(sport) {
		return Group{}, fmt.Errorf("%w: sport", ErrInvalid)
	}
	for i := 0; i < 5; i++ {
		g, err := s.groups.insertGroup(ctx, name, sport, userID, s.inviteCode())
		if err == nil {
			g.IsOwner = true
			if g.MemberCount == 0 {
				g.MemberCount = 1
			}
			return g, nil
		}
		if errors.Is(err, ErrConflict) {
			continue
		}
		return Group{}, err
	}
	return Group{}, fmt.Errorf("%w: invite code", ErrConflict)
}

// ListGroups returns groups the caller belongs to.
func (s *Service) ListGroups(ctx context.Context, userID string) ([]Group, error) {
	groups, err := s.groups.listGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []Group{}
	}
	for i := range groups {
		groups[i].IsOwner = groups[i].OwnerID == userID
	}
	return groups, nil
}

// JoinGroup adds the caller to the group identified by invite code.
// Joining again is a no-op and returns the group.
func (s *Service) JoinGroup(ctx context.Context, userID, code string) (Group, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return Group{}, fmt.Errorf("%w: code", ErrInvalid)
	}
	g, err := s.groups.getGroupByCode(ctx, code)
	if err != nil {
		return Group{}, err
	}
	if err := s.groups.addMember(ctx, g.ID, userID); err != nil {
		return Group{}, err
	}
	members, err := s.groups.listMembers(ctx, g.ID)
	if err != nil {
		return Group{}, err
	}
	g.MemberCount = len(members)
	g.IsOwner = g.OwnerID == userID
	return g, nil
}

// GetGroup returns members and this week's standings. Non-members get ErrForbidden.
func (s *Service) GetGroup(ctx context.Context, userID, groupID string) (GroupDetail, error) {
	g, err := s.groups.getGroup(ctx, groupID)
	if err != nil {
		return GroupDetail{}, err
	}
	return s.detailForMember(ctx, userID, g)
}

// GetGroupByCode is the same view, addressed by invite code.
func (s *Service) GetGroupByCode(ctx context.Context, userID, code string) (GroupDetail, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return GroupDetail{}, fmt.Errorf("%w: code", ErrInvalid)
	}
	g, err := s.groups.getGroupByCode(ctx, code)
	if err != nil {
		return GroupDetail{}, err
	}
	return s.detailForMember(ctx, userID, g)
}

func (s *Service) detailForMember(ctx context.Context, userID string, g Group) (GroupDetail, error) {
	ok, err := s.groups.isMember(ctx, g.ID, userID)
	if err != nil {
		return GroupDetail{}, err
	}
	if !ok {
		return GroupDetail{}, ErrForbidden
	}
	members, err := s.groups.listMembers(ctx, g.ID)
	if err != nil {
		return GroupDetail{}, err
	}
	if members == nil {
		members = []GroupMember{}
	}
	for i := range members {
		members[i].IsOwner = members[i].UserID == g.OwnerID
	}
	start, end := isoWeek(s.clock())
	matches, err := s.groups.listWeekMatches(ctx, g.ID, g.Sport, start, end)
	if err != nil {
		return GroupDetail{}, err
	}
	standings := buildStandings(members, matches, g.Sport, start, end)
	if standings == nil {
		standings = []Standing{}
	}
	g.MemberCount = len(members)
	g.IsOwner = g.OwnerID == userID
	return GroupDetail{
		Group:     g,
		Members:   members,
		Standings: standings,
		WeekStart: start,
		WeekEnd:   end,
	}, nil
}

// LeaveGroup removes a non-owner member. The owner deletes the group instead.
func (s *Service) LeaveGroup(ctx context.Context, userID, groupID string) error {
	g, err := s.groups.getGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if g.OwnerID == userID {
		return fmt.Errorf("%w: owner deletes the group", ErrInvalid)
	}
	ok, err := s.groups.isMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return s.groups.removeMember(ctx, groupID, userID)
}

// DeleteGroup removes the group. Only the owner may delete it.
func (s *Service) DeleteGroup(ctx context.Context, userID, groupID string) error {
	g, err := s.groups.getGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if g.OwnerID != userID {
		return ErrForbidden
	}
	return s.groups.deleteGroup(ctx, groupID)
}
