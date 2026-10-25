package notify

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeNotifyStore struct {
	token          DeviceToken
	tokens         []string
	platformTokens []PlatformToken
	upsertErr      error
	deleteErr      error
	listErr        error
	deleted        string
}

func (f *fakeNotifyStore) upsert(_ context.Context, userID, token, platform string) (DeviceToken, error) {
	if f.upsertErr != nil {
		return DeviceToken{}, f.upsertErr
	}
	d := DeviceToken{
		ID: "d1", UserID: userID, Token: token, Platform: platform,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.token = d
	return d, nil
}

func (f *fakeNotifyStore) delete(_ context.Context, _, token string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = token
	return nil
}

func (f *fakeNotifyStore) tokensForUser(context.Context, string) ([]string, error) {
	return f.tokens, f.listErr
}

func (f *fakeNotifyStore) tokensWithPlatform(context.Context, string) ([]PlatformToken, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.platformTokens) > 0 {
		return f.platformTokens, nil
	}
	out := make([]PlatformToken, 0, len(f.tokens))
	for _, t := range f.tokens {
		out = append(out, PlatformToken{Token: t, Platform: "android"})
	}
	return out, nil
}

type recordingSender struct {
	tokens          []string
	localizedTokens []PlatformToken
	localized       LocalizedPush
	err             error
}

func (r *recordingSender) Send(_ context.Context, tokens []string, _, _ string, _ map[string]string) error {
	r.tokens = append([]string{}, tokens...)
	return r.err
}

func (r *recordingSender) SendLocalized(_ context.Context, tokens []PlatformToken, msg LocalizedPush) error {
	r.localizedTokens = append([]PlatformToken{}, tokens...)
	r.localized = msg
	return r.err
}

func TestRegister_ValidatesPlatformAndToken(t *testing.T) {
	svc := &Service{repo: &fakeNotifyStore{}, sender: NoopSender{}}
	_, err := svc.Register(context.Background(), "u1", "", "android")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty token err=%v", err)
	}
	_, err = svc.Register(context.Background(), "u1", "tok", "blackberry")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad platform err=%v", err)
	}
	got, err := svc.Register(context.Background(), "u1", "  abc  ", "Android")
	if err != nil || got.Token != "abc" || got.Platform != "android" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestUnregister_TrimsAndDeletes(t *testing.T) {
	store := &fakeNotifyStore{}
	svc := &Service{repo: store, sender: NoopSender{}}
	if err := svc.Unregister(context.Background(), "u1", "  tok  "); err != nil {
		t.Fatal(err)
	}
	if store.deleted != "tok" {
		t.Fatalf("deleted=%q", store.deleted)
	}
	if err := svc.Unregister(context.Background(), "u1", "   "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestSendToUser_BestEffort(t *testing.T) {
	sender := &recordingSender{}
	svc := &Service{repo: &fakeNotifyStore{tokens: []string{"t1", "t2"}}, sender: sender}
	if err := svc.SendToUser(context.Background(), "u1", "hi", "body", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if len(sender.tokens) != 2 {
		t.Fatalf("tokens=%v", sender.tokens)
	}

	// list failure must not surface
	svc = &Service{repo: &fakeNotifyStore{listErr: errors.New("db")}, sender: sender}
	if err := svc.SendToUser(context.Background(), "u1", "hi", "body", nil); err != nil {
		t.Fatal(err)
	}
}

func TestSendLocalizedToUser_SkipsWebAndFansOut(t *testing.T) {
	sender := &recordingSender{}
	store := &fakeNotifyStore{platformTokens: []PlatformToken{
		{Token: "ios1", Platform: "ios"},
		{Token: "web1", Platform: "web"},
		{Token: "and1", Platform: "android"},
	}}
	svc := &Service{repo: store, sender: sender}
	if err := svc.SendLocalizedToUser(context.Background(), "u1", "goal.reminder",
		"goal_reminder_push_title", "goal_reminder_push_body",
		[]string{"1", "2"}, map[string]string{"type": "goal.reminder"}); err != nil {
		t.Fatal(err)
	}
	if len(sender.localizedTokens) != 2 {
		t.Fatalf("tokens=%v", sender.localizedTokens)
	}
	if sender.localized.TitleLocKey != "goal_reminder_push_title" || sender.localized.Type != "goal.reminder" {
		t.Fatalf("msg=%+v", sender.localized)
	}
}
