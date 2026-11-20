package social

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"courtscore/internal/platform/sse"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	activityTypeMatch   = "match_recorded"
	reactionKudos       = "kudos"
	maxCommentLen         = 280
	commentPushCooldown   = 60 * time.Second
	commentPushDailyCap   = 10
	commentPostMinInterval = 3 * time.Second
	commentPostDailyCap    = 40
)

// MatchRecord is the slim snapshot score passes through ActivitySink.
type MatchRecord struct {
	ID           string
	ActorID      string
	Sport        string
	PlayerAName  string
	PlayerBName  string
	PlayerA2Name *string
	PlayerB2Name *string
	SetsA        int
	SetsB        int
	WinnerSide   string
	Winner       string
	PlayedAt     time.Time
}

// ActivitySummary is the denormalized match snapshot stored on an activity.
type ActivitySummary struct {
	PlayerAName string    `json:"player_a_name"`
	PlayerBName string    `json:"player_b_name"`
	SetsA       int       `json:"sets_a"`
	SetsB       int       `json:"sets_b"`
	WinnerSide  string    `json:"winner_side"`
	Winner      string    `json:"winner"`
	A2          *string   `json:"a2,omitempty"`
	B2          *string   `json:"b2,omitempty"`
	PlayedAt    time.Time `json:"played_at"`
}

// ActivityDTO is the feed item returned to clients.
type ActivityDTO struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Actor          UserSummary     `json:"actor"`
	MatchID        *string         `json:"match_id"`
	Sport          string          `json:"sport"`
	Summary        ActivitySummary `json:"summary"`
	ReactionCount  int             `json:"reaction_count"`
	CommentCount   int             `json:"comment_count"`
	ViewerHasKudos bool            `json:"viewer_has_kudos"`
	CreatedAt      time.Time       `json:"created_at"`
}

// CommentDTO is a short comment on an activity.
type CommentDTO struct {
	ID         string      `json:"id"`
	ActivityID string      `json:"activity_id"`
	User       UserSummary `json:"user"`
	Body       string      `json:"body"`
	CreatedAt  time.Time   `json:"created_at"`
}

// ReactionResult is returned after adding kudos.
type ReactionResult struct {
	ActivityID     string `json:"activity_id"`
	ReactionCount  int    `json:"reaction_count"`
	ViewerHasKudos bool   `json:"viewer_has_kudos"`
}

// ReactionAddedEvent is published over SSE as feed.reaction_added.
type ReactionAddedEvent struct {
	ActivityID    string      `json:"activity_id"`
	ReactionCount int         `json:"reaction_count"`
	User          UserSummary `json:"user"`
}

// CommentAddedEvent is published over SSE as feed.comment_added.
type CommentAddedEvent struct {
	ActivityID   string     `json:"activity_id"`
	CommentCount int        `json:"comment_count"`
	Comment      CommentDTO `json:"comment"`
}

// FeedSettings is the caller's publish preference.
type FeedSettings struct {
	FeedOptOut bool `json:"feed_opt_out"`
}

type activityRow struct {
	ID             string
	ActorID        string
	Type           string
	MatchID        *string
	Sport          string
	Summary        ActivitySummary
	CreatedAt      time.Time
	ReactionCount  int
	CommentCount   int
	ViewerHasKudos bool
	Actor          UserSummary
}

type commentRow struct {
	ID         string
	ActivityID string
	UserID     string
	Body       string
	CreatedAt  time.Time
	User       UserSummary
}

type commentDayCount struct {
	day string
	n   int
}

type feedStore interface {
	insertActivity(ctx context.Context, rec MatchRecord) (activityRow, error)
	deleteByMatchID(ctx context.Context, matchID string) error
	listVisible(ctx context.Context, viewerID string, limit, offset int) ([]activityRow, int64, error)
	getActivity(ctx context.Context, id string) (activityRow, error)
	addReaction(ctx context.Context, activityID, userID, typ string) (inserted bool, count int, err error)
	removeReaction(ctx context.Context, activityID, userID, typ string) (count int, err error)
	listComments(ctx context.Context, activityID string, limit, offset int) ([]commentRow, int64, error)
	addComment(ctx context.Context, activityID, userID, body string) (commentRow, error)
	getComment(ctx context.Context, id string) (commentRow, error)
	deleteComment(ctx context.Context, id string) error
	friendIDs(ctx context.Context, userID string) ([]string, error)
	areFriends(ctx context.Context, a, b string) (bool, error)
	getUserSummary(ctx context.Context, id string) (UserSummary, error)
	isOptedOut(ctx context.Context, userID string) (bool, error)
	getFeedOptOut(ctx context.Context, userID string) (bool, error)
	setFeedOptOut(ctx context.Context, userID string, optOut bool) error
}

// FeedService owns the friend activity feed.
// FeedPulseHook notifies users to reload the weekly pulse after feed interaction.
type FeedPulseHook interface {
	NotifyFeedPulse(ctx context.Context, userIDs []string)
}

type FeedService struct {
	repo  feedStore
	hub   *sse.Hub
	push  PushSender
	pulse FeedPulseHook
	now   func() time.Time

	mu               sync.Mutex
	lastCommentPush  map[string]time.Time
	commentPending   map[string]int
	commentDay       map[string]commentDayCount
	lastCommentPost  map[string]time.Time
	commentPostDay   map[string]commentDayCount
}

// NewFeedService wires a FeedService to Postgres.
func NewFeedService(pool *pgxpool.Pool, hub *sse.Hub, push PushSender) *FeedService {
	return &FeedService{
		repo:            &feedRepo{pool: pool},
		hub:             hub,
		push:            push,
		now:             func() time.Time { return time.Now().UTC() },
		lastCommentPush: map[string]time.Time{},
		commentPending:  map[string]int{},
		commentDay:      map[string]commentDayCount{},
		lastCommentPost: map[string]time.Time{},
		commentPostDay:  map[string]commentDayCount{},
	}
}

func (s *FeedService) SetPulseHook(h FeedPulseHook) { s.pulse = h }

func (s *FeedService) notifyPulse(ctx context.Context, callerID, actorID string) {
	if s.pulse == nil {
		return
	}
	ids := assembleAudience([]string{callerID, actorID})
	if len(ids) == 0 {
		return
	}
	s.pulse.NotifyFeedPulse(ctx, ids)
}

// OnMatchRecorded creates one activity and fans SSE to the actor's friends.
// Best-effort: errors are logged, never returned (score hook must not fail).
func (s *FeedService) OnMatchRecorded(ctx context.Context, rec MatchRecord) {
	if rec.ActorID == "" || rec.ID == "" {
		return
	}
	opted, err := s.repo.isOptedOut(ctx, rec.ActorID)
	if err != nil {
		slog.Warn("feed: opt-out check failed", "err", err)
		return
	}
	if opted {
		return
	}
	row, err := s.repo.insertActivity(ctx, rec)
	if err != nil {
		slog.Warn("feed: insert activity failed", "err", err)
		return
	}
	actor, err := s.repo.getUserSummary(ctx, rec.ActorID)
	if err != nil {
		slog.Warn("feed: actor lookup failed", "err", err)
		return
	}
	dto := activityDTO(row, actor)
	dto.ViewerHasKudos = false
	s.publishToFriends(ctx, rec.ActorID, sse.Event{Type: "feed.activity_created", Data: dto})
}

// OnMatchDeleted removes the activity for a deleted match (cascades reactions/comments).
func (s *FeedService) OnMatchDeleted(ctx context.Context, matchID string) {
	if matchID == "" {
		return
	}
	if err := s.repo.deleteByMatchID(ctx, matchID); err != nil {
		slog.Warn("feed: delete activity failed", "match_id", matchID, "err", err)
	}
}

// ListFeed returns accepted friends' activities visible to viewer (newest first).
func (s *FeedService) ListFeed(ctx context.Context, viewerID string, limit, offset int) ([]ActivityDTO, int64, error) {
	rows, total, err := s.repo.listVisible(ctx, viewerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ActivityDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, activityDTO(row, row.Actor))
	}
	return out, total, nil
}

// AddReaction adds kudos (idempotent). Pushes the activity author on the first kudos only.
func (s *FeedService) AddReaction(ctx context.Context, callerID, activityID, typ string) (ReactionResult, error) {
	if typ == "" {
		typ = reactionKudos
	}
	if typ != reactionKudos {
		return ReactionResult{}, fmt.Errorf("%w: unknown reaction type", ErrInvalid)
	}
	act, err := s.repo.getActivity(ctx, activityID)
	if err != nil {
		return ReactionResult{}, err
	}
	if err := s.requireVisible(ctx, callerID, act.ActorID); err != nil {
		return ReactionResult{}, err
	}
	inserted, count, err := s.repo.addReaction(ctx, activityID, callerID, typ)
	if err != nil {
		return ReactionResult{}, err
	}
	user, err := s.repo.getUserSummary(ctx, callerID)
	if err != nil {
		user = UserSummary{ID: callerID}
	}
	s.publishToAuthorAndFriends(ctx, act.ActorID, sse.Event{
		Type: "feed.reaction_added",
		Data: ReactionAddedEvent{ActivityID: activityID, ReactionCount: count, User: user},
	})
	if inserted && count == 1 && callerID != act.ActorID {
		s.pushAuthor(ctx, act.ActorID, "Congrats",
			fmt.Sprintf("%s sent you congrats", userDisplayLabel(user)),
			map[string]string{"type": "feed.reaction_added", "activity_id": activityID})
	}
	s.notifyPulse(ctx, callerID, act.ActorID)
	return ReactionResult{ActivityID: activityID, ReactionCount: count, ViewerHasKudos: true}, nil
}

// RemoveReaction removes the caller's kudos.
func (s *FeedService) RemoveReaction(ctx context.Context, callerID, activityID string) error {
	act, err := s.repo.getActivity(ctx, activityID)
	if err != nil {
		return err
	}
	if err := s.requireVisible(ctx, callerID, act.ActorID); err != nil {
		return err
	}
	_, err = s.repo.removeReaction(ctx, activityID, callerID, reactionKudos)
	if err == nil {
		s.notifyPulse(ctx, callerID, act.ActorID)
	}
	return err
}

// ListComments returns comments oldest→newest. Caller must be actor or accepted friend.
func (s *FeedService) ListComments(ctx context.Context, callerID, activityID string, limit, offset int) ([]CommentDTO, int64, error) {
	act, err := s.repo.getActivity(ctx, activityID)
	if err != nil {
		return nil, 0, err
	}
	if err := s.requireVisible(ctx, callerID, act.ActorID); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.listComments(ctx, activityID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]CommentDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, commentDTO(row))
	}
	return out, total, nil
}

// AddComment posts a 1–280 char comment and fans SSE; push is rate-limited.
func (s *FeedService) AddComment(ctx context.Context, callerID, activityID, body string) (CommentDTO, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > maxCommentLen {
		return CommentDTO{}, fmt.Errorf("%w: comment must be 1–280 characters", ErrInvalid)
	}
	act, err := s.repo.getActivity(ctx, activityID)
	if err != nil {
		return CommentDTO{}, err
	}
	if err := s.requireVisible(ctx, callerID, act.ActorID); err != nil {
		return CommentDTO{}, err
	}
	if err := s.allowCommentPost(callerID); err != nil {
		return CommentDTO{}, err
	}
	row, err := s.repo.addComment(ctx, activityID, callerID, body)
	if err != nil {
		return CommentDTO{}, err
	}
	if row.User.ID == "" {
		if u, uErr := s.repo.getUserSummary(ctx, callerID); uErr == nil {
			row.User = u
		} else {
			row.User = UserSummary{ID: callerID}
		}
	}
	dto := commentDTO(row)
	refreshed, _ := s.repo.getActivity(ctx, activityID)
	count := refreshed.CommentCount
	if count == 0 {
		count = 1
	}
	s.publishToAuthorAndFriends(ctx, act.ActorID, sse.Event{
		Type: "feed.comment_added",
		Data: CommentAddedEvent{ActivityID: activityID, CommentCount: count, Comment: dto},
	})
	if callerID != act.ActorID {
		s.pushComment(ctx, act.ActorID, activityID, dto.User)
	}
	s.notifyPulse(ctx, callerID, act.ActorID)
	return dto, nil
}

// DeleteComment allows the comment author or the activity author.
func (s *FeedService) DeleteComment(ctx context.Context, callerID, activityID, commentID string) error {
	act, err := s.repo.getActivity(ctx, activityID)
	if err != nil {
		return err
	}
	if err := s.requireVisible(ctx, callerID, act.ActorID); err != nil {
		return err
	}
	c, err := s.repo.getComment(ctx, commentID)
	if err != nil {
		return err
	}
	if c.ActivityID != activityID {
		return ErrNotFound
	}
	if callerID != c.UserID && callerID != act.ActorID {
		return ErrForbidden
	}
	if err := s.repo.deleteComment(ctx, commentID); err != nil {
		return err
	}
	s.notifyPulse(ctx, callerID, act.ActorID)
	return nil
}

// GetSettings returns the caller's feed_opt_out flag.
func (s *FeedService) GetSettings(ctx context.Context, userID string) (FeedSettings, error) {
	opt, err := s.repo.getFeedOptOut(ctx, userID)
	if err != nil {
		return FeedSettings{}, err
	}
	return FeedSettings{FeedOptOut: opt}, nil
}

// SetSettings updates feed_opt_out. Opting out only stops publishing, not viewing.
func (s *FeedService) SetSettings(ctx context.Context, userID string, optOut bool) (FeedSettings, error) {
	if err := s.repo.setFeedOptOut(ctx, userID, optOut); err != nil {
		return FeedSettings{}, err
	}
	return FeedSettings{FeedOptOut: optOut}, nil
}

func (s *FeedService) allowCommentPost(userID string) error {
	now := s.now()
	day := now.UTC().Format("2006-01-02")
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.lastCommentPost[userID]; ok && !last.IsZero() && now.Sub(last) < commentPostMinInterval {
		return fmt.Errorf("%w: comment too soon", ErrRateLimited)
	}
	dc := s.commentPostDay[userID]
	if dc.day != day {
		dc = commentDayCount{day: day, n: 0}
	}
	if dc.n >= commentPostDailyCap {
		return fmt.Errorf("%w: daily comment cap", ErrRateLimited)
	}
	s.lastCommentPost[userID] = now
	dc.n++
	s.commentPostDay[userID] = dc
	return nil
}

func (s *FeedService) requireVisible(ctx context.Context, callerID, actorID string) error {
	if callerID == actorID {
		return nil
	}
	ok, err := s.repo.areFriends(ctx, callerID, actorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (s *FeedService) publishToFriends(ctx context.Context, actorID string, ev sse.Event) {
	if s.hub == nil {
		return
	}
	ids, err := s.repo.friendIDs(ctx, actorID)
	if err != nil {
		slog.Warn("feed: friend fan-out failed", "err", err)
		return
	}
	for _, id := range ids {
		s.hub.Publish(id, ev)
	}
}

func (s *FeedService) publishToAuthorAndFriends(ctx context.Context, actorID string, ev sse.Event) {
	if s.hub != nil {
		s.hub.Publish(actorID, ev)
	}
	s.publishToFriends(ctx, actorID, ev)
}

func (s *FeedService) pushAuthor(ctx context.Context, authorID, title, body string, data map[string]string) {
	if s.push == nil || authorID == "" {
		return
	}
	if err := s.push.SendToUser(ctx, authorID, title, body, data); err != nil {
		slog.Warn("feed: push failed", "err", err)
	}
}

func (s *FeedService) pushComment(ctx context.Context, authorID, activityID string, commenter UserSummary) {
	if s.push == nil {
		return
	}
	now := s.now()
	day := now.UTC().Format("2006-01-02")
	s.mu.Lock()
	pending := s.commentPending[activityID] + 1
	s.commentPending[activityID] = pending
	last := s.lastCommentPush[activityID]
	dc := s.commentDay[activityID]
	if dc.day != day {
		dc = commentDayCount{day: day, n: 0}
	}
	if dc.n >= commentPushDailyCap {
		s.mu.Unlock()
		return
	}
	if !last.IsZero() && now.Sub(last) < commentPushCooldown {
		s.mu.Unlock()
		return
	}
	s.lastCommentPush[activityID] = now
	s.commentPending[activityID] = 0
	dc.n++
	s.commentDay[activityID] = dc
	s.mu.Unlock()

	body := fmt.Sprintf("%s commented on your match", userDisplayLabel(commenter))
	if pending > 1 {
		body = fmt.Sprintf("%d new comments on your match", pending)
	}
	s.pushAuthor(ctx, authorID, "New comment", body, map[string]string{
		"type":        "feed.comment_added",
		"activity_id": activityID,
	})
}

func activityDTO(row activityRow, actor UserSummary) ActivityDTO {
	return ActivityDTO{
		ID:             row.ID,
		Type:           row.Type,
		Actor:          actor,
		MatchID:        row.MatchID,
		Sport:          row.Sport,
		Summary:        row.Summary,
		ReactionCount:  row.ReactionCount,
		CommentCount:   row.CommentCount,
		ViewerHasKudos: row.ViewerHasKudos,
		CreatedAt:      row.CreatedAt,
	}
}

func commentDTO(row commentRow) CommentDTO {
	return CommentDTO{
		ID:         row.ID,
		ActivityID: row.ActivityID,
		User:       row.User,
		Body:       row.Body,
		CreatedAt:  row.CreatedAt,
	}
}
