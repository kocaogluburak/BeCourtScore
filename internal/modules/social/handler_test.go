package social

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
	"courtscore/internal/platform/sse"
)

// ── stub service ─────────────────────────────────────────────────────────────

type stubService struct {
	searchResults []SearchResult
	profile       UserProfile
	friendship    Friendship
	friends       []UserSummary
	requests      []IncomingRequest
	total         int64

	searchErr   error
	profileErr  error
	sendErr     error
	acceptErr   error
	rejectErr   error
	listErr     error
	unfriendErr error
}

func (s *stubService) SearchUsers(_ context.Context, _, q string, _, _ int) ([]SearchResult, int64, error) {
	if s.searchErr != nil {
		return nil, 0, s.searchErr
	}
	return s.searchResults, s.total, nil
}

func (s *stubService) GetUserProfile(_ context.Context, _, _ string) (UserProfile, error) {
	return s.profile, s.profileErr
}

func (s *stubService) SendRequest(_ context.Context, _, _ string) (Friendship, error) {
	return s.friendship, s.sendErr
}

func (s *stubService) AcceptRequest(_ context.Context, _, _ string) (Friendship, error) {
	return s.friendship, s.acceptErr
}

func (s *stubService) RejectRequest(_ context.Context, _, _ string) (Friendship, error) {
	return s.friendship, s.rejectErr
}

func (s *stubService) ListFriends(_ context.Context, _ string, _, _ int) ([]UserSummary, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.friends, s.total, nil
}

func (s *stubService) ListIncomingRequests(_ context.Context, _ string, _, _ int) ([]IncomingRequest, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.requests, s.total, nil
}

func (s *stubService) Unfriend(_ context.Context, _, _ string) error { return s.unfriendErr }

func authedRequest(method, target string, body []byte, userID string) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	ctx := context.WithValue(r.Context(), authkit.UserIDKey, userID)
	return r.WithContext(ctx)
}

func ptr(s string) *string { return &s }

// ── GET /v1/users/search ─────────────────────────────────────────────────────

func TestSearchUsers_ReturnsPagedResults(t *testing.T) {
	svc := &stubService{
		searchResults: []SearchResult{{UserSummary: UserSummary{ID: "u2", Nickname: ptr("ace")}, FriendshipStatus: "none"}},
		total:         1,
	}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.searchUsers(w, authedRequest(http.MethodGet, "/v1/users/search?q=ac", nil, "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got httpx.Page[SearchResult]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].FriendshipStatus != "none" {
		t.Errorf("items: %+v", got.Items)
	}
}

func TestSearchUsers_Returns400OnShortQuery(t *testing.T) {
	svc := &stubService{searchErr: ErrInvalid}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.searchUsers(w, authedRequest(http.MethodGet, "/v1/users/search?q=a", nil, "u1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

// ── GET /v1/users/{userID} ───────────────────────────────────────────────────

func TestGetUserProfile_Returns200WithFriendshipStatus(t *testing.T) {
	svc := &stubService{
		profile: UserProfile{
			UserSummary:      UserSummary{ID: "u2", Nickname: ptr("bob")},
			FriendshipStatus: "accepted",
		},
	}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.getUserProfile(w, authedRequest(http.MethodGet, "/v1/users/u2", nil, "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got UserProfile
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FriendshipStatus != "accepted" {
		t.Errorf("friendship_status: got %q, want %q", got.FriendshipStatus, "accepted")
	}
}

func TestGetUserProfile_Returns200ForNonFriendWithNoneStatus(t *testing.T) {
	svc := &stubService{
		profile: UserProfile{
			UserSummary:      UserSummary{ID: "u9", Nickname: ptr("stranger")},
			FriendshipStatus: "none",
		},
	}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.getUserProfile(w, authedRequest(http.MethodGet, "/v1/users/u9", nil, "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got UserProfile
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FriendshipStatus != "none" {
		t.Errorf("friendship_status: got %q, want %q", got.FriendshipStatus, "none")
	}
}

func TestGetUserProfile_Returns404WhenNotFound(t *testing.T) {
	svc := &stubService{profileErr: ErrNotFound}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.getUserProfile(w, authedRequest(http.MethodGet, "/v1/users/u9", nil, "u1"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

// ── POST /v1/friends/requests ────────────────────────────────────────────────

type fakePush struct {
	calls int
	last  string
	typ   string
}

func (p *fakePush) SendToUser(_ context.Context, userID, _, _ string, data map[string]string) error {
	p.calls++
	p.last = userID
	p.typ = data["type"]
	return nil
}

func TestSendRequest_Returns201AndPublishesSSE(t *testing.T) {
	svc := &stubService{
		friendship: Friendship{ID: "f1", RequesterID: "u1", AddresseeID: "u2", Status: "pending"},
		profile:    UserProfile{UserSummary: UserSummary{ID: "u1", Nickname: ptr("ace")}},
	}
	hub := sse.NewHub()
	ch, unsub := hub.Subscribe("u2")
	defer unsub()
	push := &fakePush{}
	h := &handler{svc: svc, hub: hub, push: push}

	body, _ := json.Marshal(map[string]string{"user_id": "u2"})
	w := httptest.NewRecorder()
	h.sendRequest(w, authedRequest(http.MethodPost, "/v1/friends/requests", body, "u1"))

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
	select {
	case ev := <-ch:
		if ev.Type != "friend.request_received" {
			t.Errorf("event type: got %q", ev.Type)
		}
	default:
		t.Error("expected SSE event for addressee")
	}
	if push.calls != 1 || push.last != "u2" || push.typ != "friend.request_received" {
		t.Fatalf("push: calls=%d last=%q typ=%q", push.calls, push.last, push.typ)
	}
}

func TestUserDisplayLabel(t *testing.T) {
	if got := userDisplayLabel(UserSummary{Nickname: ptr("ace")}); got != "ace" {
		t.Fatalf("nickname: got %q", got)
	}
	if got := userDisplayLabel(UserSummary{Name: ptr("Ada"), Surname: ptr("L")}); got != "Ada L" {
		t.Fatalf("full name: got %q", got)
	}
	if got := userDisplayLabel(UserSummary{}); got != "Someone" {
		t.Fatalf("empty: got %q", got)
	}
}

func TestSendRequest_Returns400WithoutUserID(t *testing.T) {
	h := &handler{svc: &stubService{}, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	h.sendRequest(w, authedRequest(http.MethodPost, "/v1/friends/requests", body, "u1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestSendRequest_Returns409OnDuplicate(t *testing.T) {
	svc := &stubService{sendErr: ErrConflict}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{"user_id": "u2"})
	w := httptest.NewRecorder()
	h.sendRequest(w, authedRequest(http.MethodPost, "/v1/friends/requests", body, "u1"))

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", w.Code)
	}
}

// ── POST /v1/friends/requests/{id}/accept ────────────────────────────────────

func TestAcceptRequest_Returns200AndNotifiesRequester(t *testing.T) {
	svc := &stubService{friendship: Friendship{ID: "f1", RequesterID: "u1", AddresseeID: "u2", Status: "accepted"}}
	hub := sse.NewHub()
	ch, unsub := hub.Subscribe("u1")
	defer unsub()
	h := &handler{svc: svc, hub: hub}

	w := httptest.NewRecorder()
	h.acceptRequest(w, authedRequest(http.MethodPost, "/v1/friends/requests/f1/accept", nil, "u2"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	select {
	case ev := <-ch:
		if ev.Type != "friend.request_accepted" {
			t.Errorf("event type: got %q", ev.Type)
		}
	default:
		t.Error("expected SSE event for requester")
	}
}

func TestAcceptRequest_Returns404WhenNotAddresseeOrMissing(t *testing.T) {
	svc := &stubService{acceptErr: ErrNotFound}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.acceptRequest(w, authedRequest(http.MethodPost, "/v1/friends/requests/f1/accept", nil, "u3"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

// ── GET /v1/friends ──────────────────────────────────────────────────────────

func TestListFriends_Returns200Paged(t *testing.T) {
	svc := &stubService{
		friends: []UserSummary{{ID: "u2", Nickname: ptr("bob")}},
		total:   1,
	}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.listFriends(w, authedRequest(http.MethodGet, "/v1/friends?page=1&page_size=20", nil, "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got httpx.Page[UserSummary]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "u2" || got.Total != 1 {
		t.Errorf("page: %+v", got)
	}
}

func TestListFriends_Returns500OnError(t *testing.T) {
	h := &handler{svc: &stubService{listErr: ErrNotFound}, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.listFriends(w, authedRequest(http.MethodGet, "/v1/friends", nil, "u1"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", w.Code)
	}
}

// ── GET /v1/friends/requests ─────────────────────────────────────────────────

func TestListRequests_Returns200Paged(t *testing.T) {
	svc := &stubService{
		requests: []IncomingRequest{{
			ID:        "f1",
			Requester: UserSummary{ID: "u2", Nickname: ptr("bob")},
		}},
		total: 1,
	}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.listRequests(w, authedRequest(http.MethodGet, "/v1/friends/requests", nil, "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got httpx.Page[IncomingRequest]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "f1" {
		t.Errorf("page: %+v", got)
	}
}

// ── POST /v1/friends/requests/{id}/reject ────────────────────────────────────

func TestRejectRequest_Returns200(t *testing.T) {
	svc := &stubService{friendship: Friendship{ID: "f1", RequesterID: "u1", AddresseeID: "u2", Status: "rejected"}}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.rejectRequest(w, authedRequest(http.MethodPost, "/v1/friends/requests/f1/reject", nil, "u2"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got Friendship
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "rejected" {
		t.Errorf("status: got %q, want rejected", got.Status)
	}
}

func TestRejectRequest_Returns404WhenMissing(t *testing.T) {
	svc := &stubService{rejectErr: ErrNotFound}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.rejectRequest(w, authedRequest(http.MethodPost, "/v1/friends/requests/f1/reject", nil, "u2"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

// ── DELETE /v1/friends/{userID} ──────────────────────────────────────────────

func TestUnfriend_Returns204(t *testing.T) {
	h := &handler{svc: &stubService{}, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.unfriend(w, authedRequest(http.MethodDelete, "/v1/friends/u2", nil, "u1"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", w.Code)
	}
}

func TestUnfriend_Returns404WhenNotFriends(t *testing.T) {
	svc := &stubService{unfriendErr: ErrNotFound}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.unfriend(w, authedRequest(http.MethodDelete, "/v1/friends/u2", nil, "u1"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}
