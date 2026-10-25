package invite

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var validSources = map[string]bool{
	"share_card": true, "rematch": true, "profile": true, "generic": true,
}

var validSports = map[string]bool{
	"TENNIS": true, "PADEL": true, "SQUASH": true, "PING_PONG": true, "PICKLEBALL": true,
}

type store interface {
	insert(ctx context.Context, inviterID, code, source string, sport, sourceMatchID *string) (Share, error)
	getByCode(ctx context.Context, code string) (Share, error)
	incrementClick(ctx context.Context, code string) (Share, error)
	recordSignup(ctx context.Context, code, userID string) error
	userSummary(ctx context.Context, id string) (UserSummary, error)
}

// CreateInput is the body of POST /v1/invites.
type CreateInput struct {
	Source        string
	Sport         *string
	SourceMatchID *string
}

// Created is the authenticated mint response.
type Created struct {
	Code  string `json:"code"`
	URL   string `json:"url"`
	QRURL string `json:"qr_url,omitempty"`
}

// PublicInvite is returned by GET /v1/public/invite/{code}.
type PublicInvite struct {
	Inviter UserSummary `json:"inviter"`
	Sport   *string     `json:"sport,omitempty"`
	URL     string      `json:"url"`
}

// Service mints tracked invite codes and records attribution.
type Service struct {
	repo            store
	publicWebOrigin string
	newCode         func() (string, error)
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		repo:            &repo{pool: pool},
		publicWebOrigin: "https://court-score.com",
		newCode:         newShareToken,
	}
}

func (s *Service) SetPublicWebOrigin(origin string) {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin != "" {
		s.publicWebOrigin = origin
	}
}

func (s *Service) shareURL(code string) string {
	origin := s.publicWebOrigin
	if origin == "" {
		origin = "https://court-score.com"
	}
	return origin + "/i/" + code
}

// Create mints a unique URL-safe invite code for the authenticated inviter.
func (s *Service) Create(ctx context.Context, userID string, in CreateInput) (Created, error) {
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "generic"
	}
	if !validSources[source] {
		return Created{}, fmt.Errorf("%w: unknown source %q", ErrInvalid, source)
	}
	var sport *string
	if in.Sport != nil {
		sp := strings.TrimSpace(*in.Sport)
		if sp != "" {
			if !validSports[sp] {
				return Created{}, fmt.Errorf("%w: unknown sport %q", ErrInvalid, sp)
			}
			sport = &sp
		}
	}
	var matchID *string
	if in.SourceMatchID != nil {
		id := strings.TrimSpace(*in.SourceMatchID)
		if id != "" {
			matchID = &id
		}
	}
	var last error
	for i := 0; i < 4; i++ {
		code, err := s.newCode()
		if err != nil {
			return Created{}, err
		}
		sh, err := s.repo.insert(ctx, userID, code, source, sport, matchID)
		if err != nil {
			last = err
			continue
		}
		return Created{Code: sh.Code, URL: s.shareURL(sh.Code)}, nil
	}
	return Created{}, fmt.Errorf("mint invite: %w", last)
}

// ResolvePublic increments click_count and returns the inviter projection.
func (s *Service) ResolvePublic(ctx context.Context, code string) (PublicInvite, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return PublicInvite{}, ErrNotFound
	}
	sh, err := s.repo.incrementClick(ctx, code)
	if err != nil {
		return PublicInvite{}, err
	}
	inviter, err := s.repo.userSummary(ctx, sh.InviterID)
	if err != nil {
		inviter = UserSummary{ID: sh.InviterID}
	}
	return PublicInvite{Inviter: inviter, Sport: sh.Sport, URL: s.shareURL(sh.Code)}, nil
}

// RecordSignup attributes a new user to an invite code. Best-effort: invalid
// codes and already-attributed users are ignored and never fail signup.
func (s *Service) RecordSignup(ctx context.Context, code, userID string) error {
	code = strings.TrimSpace(code)
	userID = strings.TrimSpace(userID)
	if code == "" || userID == "" {
		return nil
	}
	if _, err := s.repo.getByCode(ctx, code); err != nil {
		return nil
	}
	_ = s.repo.recordSignup(ctx, code, userID)
	return nil
}

func newShareToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])), nil
}
