package social

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	users       map[string]UserSummary
	friendships map[string]Friendship // key: ordered pair a|b
	byID        map[string]Friendship
	friends      []UserSummary
	requests     []IncomingRequest
	friendsFlag  bool
	friendsErr   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:       map[string]UserSummary{},
		friendships: map[string]Friendship{},
		byID:        map[string]Friendship{},
	}
}

func pairKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

func (f *fakeStore) getUserSummary(_ context.Context, id string) (UserSummary, error) {
	u, ok := f.users[id]
	if !ok {
		return UserSummary{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeStore) searchUsers(context.Context, string, string, int, int) ([]SearchResult, int64, error) {
	return nil, 0, nil
}

func (f *fakeStore) findBetween(_ context.Context, a, b string) (Friendship, error) {
	fr, ok := f.friendships[pairKey(a, b)]
	if !ok {
		return Friendship{}, ErrNotFound
	}
	return fr, nil
}

func (f *fakeStore) insertRequest(_ context.Context, requesterID, addresseeID string) (Friendship, error) {
	fr := Friendship{
		ID: "f-new", RequesterID: requesterID, AddresseeID: addresseeID,
		Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.friendships[pairKey(requesterID, addresseeID)] = fr
	f.byID[fr.ID] = fr
	return fr, nil
}

func (f *fakeStore) reopenRequest(_ context.Context, id, requesterID, addresseeID string) (Friendship, error) {
	fr := Friendship{
		ID: id, RequesterID: requesterID, AddresseeID: addresseeID,
		Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.friendships[pairKey(requesterID, addresseeID)] = fr
	f.byID[id] = fr
	return fr, nil
}

func (f *fakeStore) updateStatus(_ context.Context, id, addresseeID, status string) (Friendship, error) {
	fr, ok := f.byID[id]
	if !ok || fr.AddresseeID != addresseeID || fr.Status != "pending" {
		return Friendship{}, ErrNotFound
	}
	fr.Status = status
	f.byID[id] = fr
	f.friendships[pairKey(fr.RequesterID, fr.AddresseeID)] = fr
	return fr, nil
}

func (f *fakeStore) listFriends(context.Context, string, int, int) ([]UserSummary, int64, error) {
	return f.friends, int64(len(f.friends)), nil
}

func (f *fakeStore) listIncomingRequests(context.Context, string, int, int) ([]IncomingRequest, int64, error) {
	return f.requests, int64(len(f.requests)), nil
}

func (f *fakeStore) deleteAccepted(_ context.Context, a, b string) error {
	fr, ok := f.friendships[pairKey(a, b)]
	if !ok || fr.Status != "accepted" {
		return ErrNotFound
	}
	delete(f.friendships, pairKey(a, b))
	delete(f.byID, fr.ID)
	return nil
}

func (f *fakeStore) areFriends(context.Context, string, string) (bool, error) {
	return f.friendsFlag, f.friendsErr
}

func testService(f *fakeStore) *Service { return &Service{repo: f} }

func TestSendRequest_SelfRejected(t *testing.T) {
	svc := testService(newFakeStore())
	_, err := svc.SendRequest(context.Background(), "u1", "u1")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestSendRequest_ReopensAfterReject(t *testing.T) {
	f := newFakeStore()
	f.users["u1"] = UserSummary{ID: "u1"}
	f.users["u2"] = UserSummary{ID: "u2"}
	rejected := Friendship{ID: "f1", RequesterID: "u1", AddresseeID: "u2", Status: "rejected"}
	f.friendships[pairKey("u1", "u2")] = rejected
	f.byID["f1"] = rejected
	svc := testService(f)

	got, err := svc.SendRequest(context.Background(), "u2", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" || got.RequesterID != "u2" || got.AddresseeID != "u1" {
		t.Fatalf("got %+v", got)
	}
}

func TestSendRequest_ConflictWhenPending(t *testing.T) {
	f := newFakeStore()
	f.users["u2"] = UserSummary{ID: "u2"}
	pending := Friendship{ID: "f1", RequesterID: "u1", AddresseeID: "u2", Status: "pending"}
	f.friendships[pairKey("u1", "u2")] = pending
	svc := testService(f)

	_, err := svc.SendRequest(context.Background(), "u1", "u2")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestAreFriends_Delegates(t *testing.T) {
	f := newFakeStore()
	f.friendsFlag = true
	svc := testService(f)
	ok, err := svc.AreFriends(context.Background(), "a", "b")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestGetUserProfile_PendingDirections(t *testing.T) {
	f := newFakeStore()
	f.users["u1"] = UserSummary{ID: "u1"}
	f.users["u2"] = UserSummary{ID: "u2"}
	f.friendships[pairKey("u1", "u2")] = Friendship{
		ID: "f1", RequesterID: "u1", AddresseeID: "u2", Status: "pending",
	}
	svc := testService(f)

	got, err := svc.GetUserProfile(context.Background(), "u1", "u2")
	if err != nil {
		t.Fatal(err)
	}
	if got.FriendshipStatus != "pending_sent" {
		t.Fatalf("status=%q", got.FriendshipStatus)
	}

	got, err = svc.GetUserProfile(context.Background(), "u2", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.FriendshipStatus != "pending_received" {
		t.Fatalf("status=%q", got.FriendshipStatus)
	}
}

func TestAcceptReject_AddresseeOnly(t *testing.T) {
	f := newFakeStore()
	pending := Friendship{ID: "f1", RequesterID: "u1", AddresseeID: "u2", Status: "pending"}
	f.byID["f1"] = pending
	f.friendships[pairKey("u1", "u2")] = pending
	svc := testService(f)

	_, err := svc.AcceptRequest(context.Background(), "u1", "f1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("requester accept err=%v", err)
	}
	got, err := svc.RejectRequest(context.Background(), "u2", "f1")
	if err != nil || got.Status != "rejected" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
