package invite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeInviteStore struct {
	shares     map[string]Share
	users      map[string]UserSummary
	attributed map[string]string
	insertErr  error
}

func (f *fakeInviteStore) insert(_ context.Context, inviterID, code, source string, sport, sourceMatchID *string) (Share, error) {
	if f.insertErr != nil {
		return Share{}, f.insertErr
	}
	if f.shares == nil {
		f.shares = map[string]Share{}
	}
	if _, exists := f.shares[code]; exists {
		return Share{}, errors.New("duplicate code")
	}
	s := Share{
		ID: "sh-1", Code: code, InviterID: inviterID, Source: source,
		Sport: sport, SourceMatchID: sourceMatchID, CreatedAt: time.Now(),
	}
	f.shares[code] = s
	return s, nil
}

func (f *fakeInviteStore) getByCode(_ context.Context, code string) (Share, error) {
	if s, ok := f.shares[code]; ok {
		return s, nil
	}
	return Share{}, ErrNotFound
}

func (f *fakeInviteStore) incrementClick(_ context.Context, code string) (Share, error) {
	s, ok := f.shares[code]
	if !ok {
		return Share{}, ErrNotFound
	}
	s.ClickCount++
	now := time.Now()
	s.LastClickedAt = &now
	f.shares[code] = s
	return s, nil
}

func (f *fakeInviteStore) recordSignup(_ context.Context, code, userID string) error {
	if _, ok := f.shares[code]; !ok {
		return ErrNotFound
	}
	if f.attributed == nil {
		f.attributed = map[string]string{}
	}
	if _, exists := f.attributed[userID]; exists {
		return nil
	}
	f.attributed[userID] = code
	s := f.shares[code]
	s.SignupCount++
	f.shares[code] = s
	return nil
}

func (f *fakeInviteStore) userSummary(_ context.Context, id string) (UserSummary, error) {
	if f.users != nil {
		if u, ok := f.users[id]; ok {
			return u, nil
		}
	}
	return UserSummary{ID: id}, nil
}

func newInviteSvc(store *fakeInviteStore) *Service {
	n := 0
	return &Service{
		repo:            store,
		publicWebOrigin: "https://court-score.com",
		newCode: func() (string, error) {
			n++
			return "code" + strings.Repeat("x", n), nil
		},
	}
}

func TestCreate_MintsURLSafeCode(t *testing.T) {
	store := &fakeInviteStore{}
	svc := newInviteSvc(store)
	src := "TENNIS"
	got, err := svc.Create(context.Background(), "u1", CreateInput{Source: "share_card", Sport: &src})
	if err != nil {
		t.Fatal(err)
	}
	if got.Code == "" || strings.ContainsAny(got.Code, "+/=") {
		t.Fatalf("code not URL-safe: %q", got.Code)
	}
	if got.URL != "https://court-score.com/i/"+got.Code {
		t.Fatalf("url=%s", got.URL)
	}
	if store.shares[got.Code].Source != "share_card" {
		t.Fatalf("stored: %+v", store.shares[got.Code])
	}
}

func TestCreate_RejectsUnknownSource(t *testing.T) {
	svc := newInviteSvc(&fakeInviteStore{})
	if _, err := svc.Create(context.Background(), "u1", CreateInput{Source: "email"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestResolvePublic_IncrementsClick(t *testing.T) {
	store := &fakeInviteStore{shares: map[string]Share{
		"abc": {Code: "abc", InviterID: "inv1"},
	}, users: map[string]UserSummary{"inv1": {ID: "inv1"}}}
	svc := newInviteSvc(store)
	got, err := svc.ResolvePublic(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if store.shares["abc"].ClickCount != 1 {
		t.Fatalf("click_count=%d", store.shares["abc"].ClickCount)
	}
	if got.Inviter.ID != "inv1" || got.URL != "https://court-score.com/i/abc" {
		t.Fatalf("public: %+v", got)
	}
}

func TestResolvePublic_Invalid404(t *testing.T) {
	svc := newInviteSvc(&fakeInviteStore{shares: map[string]Share{}})
	if _, err := svc.ResolvePublic(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestRecordSignup_IncrementsAndSetsUser(t *testing.T) {
	store := &fakeInviteStore{shares: map[string]Share{
		"abc": {Code: "abc", InviterID: "inv1"},
	}}
	svc := newInviteSvc(store)
	if err := svc.RecordSignup(context.Background(), "abc", "new-user"); err != nil {
		t.Fatal(err)
	}
	if store.attributed["new-user"] != "abc" {
		t.Fatalf("attributed=%v", store.attributed)
	}
	if store.shares["abc"].SignupCount != 1 {
		t.Fatalf("signup_count=%d", store.shares["abc"].SignupCount)
	}
}

func TestRecordSignup_InvalidCodeIsNoop(t *testing.T) {
	store := &fakeInviteStore{shares: map[string]Share{}}
	svc := newInviteSvc(store)
	if err := svc.RecordSignup(context.Background(), "nope", "new-user"); err != nil {
		t.Fatal(err)
	}
	if len(store.attributed) != 0 {
		t.Fatalf("should not attribute: %v", store.attributed)
	}
}

func TestNewShareToken_URLSafeUnique(t *testing.T) {
	a, err := newShareToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newShareToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("tokens collided")
	}
	if strings.ContainsAny(a, "+/=") || strings.ContainsAny(b, "+/=") {
		t.Fatalf("not URL-safe: %q %q", a, b)
	}
}
