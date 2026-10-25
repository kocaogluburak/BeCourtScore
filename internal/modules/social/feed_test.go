package social

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"courtscore/internal/platform/sse"
)

type fakeFeedStore struct {
	mu         sync.Mutex
	users      map[string]UserSummary
	optOut     map[string]bool
	friends    map[string]map[string]bool // a -> b accepted
	activities map[string]activityRow
	byMatch    map[string]string
	reactions  map[string]map[string]bool // activityID -> userID
	comments   map[string]commentRow
	seq        int
}

func newFakeFeedStore() *fakeFeedStore {
	return &fakeFeedStore{
		users:      map[string]UserSummary{},
		optOut:     map[string]bool{},
		friends:    map[string]map[string]bool{},
		activities: map[string]activityRow{},
		byMatch:    map[string]string{},
		reactions:  map[string]map[string]bool{},
		comments:   map[string]commentRow{},
	}
}

func (f *fakeFeedStore) addUser(id, name string) {
	n := name
	f.users[id] = UserSummary{ID: id, Name: &n}
}

func (f *fakeFeedStore) addFriend(a, b string) {
	if f.friends[a] == nil {
		f.friends[a] = map[string]bool{}
	}
	if f.friends[b] == nil {
		f.friends[b] = map[string]bool{}
	}
	f.friends[a][b] = true
	f.friends[b][a] = true
}

func (f *fakeFeedStore) insertActivity(_ context.Context, rec MatchRecord) (activityRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	id := fmt.Sprintf("act-%d", f.seq)
	mid := rec.ID
	row := activityRow{
		ID: id, ActorID: rec.ActorID, Type: activityTypeMatch, MatchID: &mid, Sport: rec.Sport,
		Summary: ActivitySummary{
			PlayerAName: rec.PlayerAName, PlayerBName: rec.PlayerBName,
			SetsA: rec.SetsA, SetsB: rec.SetsB, WinnerSide: rec.WinnerSide, Winner: rec.Winner,
			A2: rec.PlayerA2Name, B2: rec.PlayerB2Name, PlayedAt: rec.PlayedAt,
		},
		CreatedAt: time.Now().UTC(),
	}
	if u, ok := f.users[rec.ActorID]; ok {
		row.Actor = u
	}
	f.activities[id] = row
	f.byMatch[rec.ID] = id
	return row, nil
}

func (f *fakeFeedStore) deleteByMatchID(_ context.Context, matchID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byMatch[matchID]
	if !ok {
		return nil
	}
	delete(f.activities, id)
	delete(f.byMatch, matchID)
	delete(f.reactions, id)
	for cid, c := range f.comments {
		if c.ActivityID == id {
			delete(f.comments, cid)
		}
	}
	return nil
}

func (f *fakeFeedStore) listVisible(_ context.Context, viewerID string, limit, offset int) ([]activityRow, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []activityRow
	for _, row := range f.activities {
		if !f.friends[viewerID][row.ActorID] {
			continue
		}
		if f.optOut[row.ActorID] {
			continue
		}
		row = f.decorateLocked(row, viewerID)
		all = append(all, row)
	}
	// newest first
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].CreatedAt.After(all[i].CreatedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	total := int64(len(all))
	if offset > len(all) {
		return []activityRow{}, total, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}

func (f *fakeFeedStore) decorateLocked(row activityRow, viewerID string) activityRow {
	if u, ok := f.users[row.ActorID]; ok {
		row.Actor = u
	}
	row.ReactionCount = len(f.reactions[row.ID])
	n := 0
	for _, c := range f.comments {
		if c.ActivityID == row.ID {
			n++
		}
	}
	row.CommentCount = n
	row.ViewerHasKudos = f.reactions[row.ID][viewerID]
	return row
}

func (f *fakeFeedStore) getActivity(_ context.Context, id string) (activityRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.activities[id]
	if !ok {
		return activityRow{}, ErrNotFound
	}
	return f.decorateLocked(row, ""), nil
}

func (f *fakeFeedStore) addReaction(_ context.Context, activityID, userID, _ string) (bool, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.activities[activityID]; !ok {
		return false, 0, ErrNotFound
	}
	if f.reactions[activityID] == nil {
		f.reactions[activityID] = map[string]bool{}
	}
	inserted := !f.reactions[activityID][userID]
	f.reactions[activityID][userID] = true
	return inserted, len(f.reactions[activityID]), nil
}

func (f *fakeFeedStore) removeReaction(_ context.Context, activityID, userID, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reactions[activityID] != nil {
		delete(f.reactions[activityID], userID)
	}
	return len(f.reactions[activityID]), nil
}

func (f *fakeFeedStore) listComments(_ context.Context, activityID string, limit, offset int) ([]commentRow, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []commentRow
	for _, c := range f.comments {
		if c.ActivityID == activityID {
			if u, ok := f.users[c.UserID]; ok {
				c.User = u
			}
			all = append(all, c)
		}
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].CreatedAt.Before(all[i].CreatedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	total := int64(len(all))
	if offset > len(all) {
		return []commentRow{}, total, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}

func (f *fakeFeedStore) addComment(_ context.Context, activityID, userID, body string) (commentRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	row := commentRow{
		ID: fmt.Sprintf("c-%d", f.seq), ActivityID: activityID, UserID: userID,
		Body: body, CreatedAt: time.Now().UTC(), User: f.users[userID],
	}
	f.comments[row.ID] = row
	return row, nil
}

func (f *fakeFeedStore) getComment(_ context.Context, id string) (commentRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.comments[id]
	if !ok {
		return commentRow{}, ErrNotFound
	}
	return c, nil
}

func (f *fakeFeedStore) deleteComment(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.comments[id]; !ok {
		return ErrNotFound
	}
	delete(f.comments, id)
	return nil
}

func (f *fakeFeedStore) friendIDs(_ context.Context, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id := range f.friends[userID] {
		ids = append(ids, id)
	}
	return ids, nil
}

func (f *fakeFeedStore) areFriends(_ context.Context, a, b string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.friends[a][b], nil
}

func (f *fakeFeedStore) getUserSummary(_ context.Context, id string) (UserSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return UserSummary{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeFeedStore) isOptedOut(_ context.Context, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.optOut[userID], nil
}

func (f *fakeFeedStore) getFeedOptOut(_ context.Context, userID string) (bool, error) {
	return f.isOptedOut(context.Background(), userID)
}

func (f *fakeFeedStore) setFeedOptOut(_ context.Context, userID string, optOut bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.optOut[userID] = optOut
	return nil
}

type recPush struct {
	mu    sync.Mutex
	calls []pushCall
}

type pushCall struct {
	userID, title, body, typ, activityID string
}

func (p *recPush) SendToUser(_ context.Context, userID, title, body string, data map[string]string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := pushCall{userID: userID, title: title, body: body}
	if data != nil {
		c.typ = data["type"]
		c.activityID = data["activity_id"]
	}
	p.calls = append(p.calls, c)
	return nil
}

func (p *recPush) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func newFeed(store *fakeFeedStore, hub *sse.Hub, push PushSender) *FeedService {
	return &FeedService{
		repo:            store,
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

func rec(actor, match string) MatchRecord {
	return MatchRecord{
		ID: match, ActorID: actor, Sport: "TENNIS",
		PlayerAName: "Ada", PlayerBName: "Grace", SetsA: 2, SetsB: 1,
		WinnerSide: "A", Winner: "Ada", PlayedAt: time.Now().UTC(),
	}
}

func waitSSE(t *testing.T, ch <-chan sse.Event, typ string) sse.Event {
	t.Helper()
	select {
	case ev := <-ch:
		if ev.Type != typ {
			t.Fatalf("sse type=%s want %s", ev.Type, typ)
		}
		return ev
	case <-time.After(time.Second):
		t.Fatalf("expected SSE %s", typ)
		return sse.Event{}
	}
}

func TestFeedList_OnlyAcceptedFriends(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("friend", "Friend")
	store.addUser("stranger", "Stranger")
	store.addUser("opted", "Opted")
	store.addFriend("friend", "actor")
	store.addFriend("friend", "opted")
	store.optOut["opted"] = true

	svc := newFeed(store, nil, nil)
	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	svc.OnMatchRecorded(context.Background(), rec("opted", "m2"))
	svc.OnMatchRecorded(context.Background(), rec("stranger", "m3"))

	items, total, err := svc.ListFeed(context.Background(), "friend", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].Actor.ID != "actor" {
		t.Fatalf("items=%+v total=%d", items, total)
	}

	items, total, err = svc.ListFeed(context.Background(), "stranger", 20, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("stranger should see empty feed: items=%+v total=%d err=%v", items, total, err)
	}
}

func TestOnMatchRecorded_FansSSEAndRespectsOptOut(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("friend", "Friend")
	store.addFriend("actor", "friend")

	hub := sse.NewHub()
	ch, unsub := hub.Subscribe("friend")
	defer unsub()
	svc := newFeed(store, hub, nil)

	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	ev := waitSSE(t, ch, "feed.activity_created")
	dto, ok := ev.Data.(ActivityDTO)
	if !ok || dto.MatchID == nil || *dto.MatchID != "m1" {
		t.Fatalf("dto=%+v", ev.Data)
	}
	if len(store.activities) != 1 {
		t.Fatalf("activities=%d", len(store.activities))
	}

	store.optOut["actor"] = true
	svc.OnMatchRecorded(context.Background(), rec("actor", "m2"))
	if len(store.activities) != 1 {
		t.Fatalf("opted-out actor still wrote: %d", len(store.activities))
	}
	select {
	case ev := <-ch:
		t.Fatalf("unexpected extra SSE %s", ev.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAddReaction_IdempotentCoalescedPush(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("friend", "Friend")
	store.addUser("other", "Other")
	store.addFriend("actor", "friend")
	store.addFriend("actor", "other")

	hub := sse.NewHub()
	ch, unsub := hub.Subscribe("actor")
	defer unsub()
	push := &recPush{}
	svc := newFeed(store, hub, push)
	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	var actID string
	for id := range store.activities {
		actID = id
	}

	first, err := svc.AddReaction(context.Background(), "friend", actID, "kudos")
	if err != nil {
		t.Fatal(err)
	}
	if first.ReactionCount != 1 || !first.ViewerHasKudos {
		t.Fatalf("first=%+v", first)
	}
	dup, err := svc.AddReaction(context.Background(), "friend", actID, "kudos")
	if err != nil {
		t.Fatal(err)
	}
	if dup.ReactionCount != 1 {
		t.Fatalf("dup count=%d", dup.ReactionCount)
	}
	second, err := svc.AddReaction(context.Background(), "other", actID, "kudos")
	if err != nil {
		t.Fatal(err)
	}
	if second.ReactionCount != 2 {
		t.Fatalf("second=%+v", second)
	}
	if push.count() != 1 || push.calls[0].userID != "actor" || push.calls[0].typ != "feed.reaction_added" {
		t.Fatalf("push calls=%+v", push.calls)
	}

	self, err := svc.AddReaction(context.Background(), "actor", actID, "kudos")
	if err != nil || self.ReactionCount != 3 {
		t.Fatalf("self=%+v err=%v", self, err)
	}
	if push.count() != 1 {
		t.Fatalf("self-kudos should not push: %+v", push.calls)
	}
	waitSSE(t, ch, "feed.reaction_added")
}

func TestAddComment_ValidatePushRateLimitAndDelete(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("friend", "Friend")
	store.addUser("other", "Other")
	store.addFriend("actor", "friend")
	store.addFriend("actor", "other")

	hub := sse.NewHub()
	ch, unsub := hub.Subscribe("actor")
	defer unsub()
	push := &recPush{}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	svc := newFeed(store, hub, push)
	svc.now = func() time.Time { return now }
	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	var actID string
	for id := range store.activities {
		actID = id
	}

	if _, err := svc.AddComment(context.Background(), "friend", actID, "   "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty: %v", err)
	}
	long := make([]rune, 281)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := svc.AddComment(context.Background(), "friend", actID, string(long)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too long: %v", err)
	}

	c1, err := svc.AddComment(context.Background(), "friend", actID, "nice match")
	if err != nil {
		t.Fatal(err)
	}
	waitSSE(t, ch, "feed.comment_added")
	if push.count() != 1 || push.calls[0].typ != "feed.comment_added" {
		t.Fatalf("push=%+v", push.calls)
	}

	if _, err := svc.AddComment(context.Background(), "other", actID, "gg"); err != nil {
		t.Fatal(err)
	}
	if push.count() != 1 {
		t.Fatalf("second comment within 60s should not push: %d", push.count())
	}

	now = now.Add(61 * time.Second)
	if _, err := svc.AddComment(context.Background(), "other", actID, "again"); err != nil {
		t.Fatal(err)
	}
	if push.count() != 2 {
		t.Fatalf("after cooldown want 2 pushes, got %d", push.count())
	}

	if err := svc.DeleteComment(context.Background(), "friend", actID, c1.ID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(commentPostMinInterval)
	c2, err := svc.AddComment(context.Background(), "other", actID, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteComment(context.Background(), "actor", actID, c2.ID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(commentPostMinInterval)
	c3, err := svc.AddComment(context.Background(), "other", actID, "nope")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteComment(context.Background(), "friend", actID, c3.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other delete: %v", err)
	}
}

func TestAddComment_DailyPushCap(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("friend", "Friend")
	store.addFriend("actor", "friend")
	push := &recPush{}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	svc := newFeed(store, nil, push)
	svc.now = func() time.Time { return now }
	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	var actID string
	for id := range store.activities {
		actID = id
	}

	for i := 0; i < commentPushDailyCap; i++ {
		if _, err := svc.AddComment(context.Background(), "friend", actID, fmt.Sprintf("c%d", i)); err != nil {
			t.Fatal(err)
		}
		now = now.Add(61 * time.Second)
	}
	if push.count() != commentPushDailyCap {
		t.Fatalf("want %d daily pushes, got %d", commentPushDailyCap, push.count())
	}
	if _, err := svc.AddComment(context.Background(), "friend", actID, "over"); err != nil {
		t.Fatal(err)
	}
	if push.count() != commentPushDailyCap {
		t.Fatalf("11th comment should be capped: %d", push.count())
	}

	now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	if _, err := svc.AddComment(context.Background(), "friend", actID, "next-day"); err != nil {
		t.Fatal(err)
	}
	if push.count() != commentPushDailyCap+1 {
		t.Fatalf("next day should push again: %d", push.count())
	}
}

func TestAddComment_PostRateLimit(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("friend", "Friend")
	store.addFriend("actor", "friend")
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	svc := newFeed(store, nil, nil)
	svc.now = func() time.Time { return now }
	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	var actID string
	for id := range store.activities {
		actID = id
	}

	if _, err := svc.AddComment(context.Background(), "friend", actID, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(context.Background(), "friend", actID, "too-soon"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("interval: %v", err)
	}
	now = now.Add(commentPostMinInterval)
	if _, err := svc.AddComment(context.Background(), "friend", actID, "ok"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < commentPostDailyCap-2; i++ {
		now = now.Add(commentPostMinInterval)
		if _, err := svc.AddComment(context.Background(), "friend", actID, fmt.Sprintf("n%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(commentPostMinInterval)
	if _, err := svc.AddComment(context.Background(), "friend", actID, "over"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("daily: %v", err)
	}
	now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	if _, err := svc.AddComment(context.Background(), "friend", actID, "next-day"); err != nil {
		t.Fatal(err)
	}
}

func TestFeedVisibilityForbidden(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("stranger", "Stranger")
	svc := newFeed(store, nil, nil)
	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	var actID string
	for id := range store.activities {
		actID = id
	}

	if _, err := svc.AddReaction(context.Background(), "stranger", actID, "kudos"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("reaction: %v", err)
	}
	if _, _, err := svc.ListComments(context.Background(), "stranger", actID, 20, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("list comments: %v", err)
	}
	if _, err := svc.AddComment(context.Background(), "stranger", actID, "hi"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("comment: %v", err)
	}
	if err := svc.DeleteComment(context.Background(), "stranger", actID, "nope"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("delete: %v", err)
	}
}

func TestOnMatchDeleted_RemovesActivity(t *testing.T) {
	store := newFakeFeedStore()
	store.addUser("actor", "Actor")
	store.addUser("friend", "Friend")
	store.addFriend("actor", "friend")
	svc := newFeed(store, nil, nil)
	svc.OnMatchRecorded(context.Background(), rec("actor", "m1"))
	var actID string
	for id := range store.activities {
		actID = id
	}
	if _, err := svc.AddReaction(context.Background(), "friend", actID, "kudos"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(context.Background(), "friend", actID, "gg"); err != nil {
		t.Fatal(err)
	}
	svc.OnMatchDeleted(context.Background(), "m1")
	if len(store.activities) != 0 {
		t.Fatalf("activity leftover: %+v", store.activities)
	}
	if len(store.reactions) != 0 || len(store.comments) != 0 {
		t.Fatalf("cascade leftover reactions=%d comments=%d", len(store.reactions), len(store.comments))
	}
}
