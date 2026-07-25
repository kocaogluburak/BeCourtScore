package tournament

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

// svcFacade is the subset of Service methods used by the HTTP handler, so tests
// can substitute a stub without hitting the database.
type svcFacade interface {
	CreateTournament(ctx context.Context, userID string, in CreateInput) (Tournament, error)
	GetTournament(ctx context.Context, userID, slug string) (TournamentView, error)
	ListMine(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Tournament, int64, error)
	JoinTournament(ctx context.Context, userID, slug, code string) (Participant, error)
	ListParticipants(ctx context.Context, tournamentID string, limit, offset int) ([]Participant, int64, error)
	Delete(ctx context.Context, userID, tournamentID string) error
	Leave(ctx context.Context, userID, tournamentID string) error
	Lock(ctx context.Context, userID, tournamentID string) (Tournament, error)
	Draw(ctx context.Context, userID, tournamentID, mode string, seeding []string) (Bracket, error)
	GetBracket(ctx context.Context, tournamentID string) (Bracket, error)
	SubmitResult(ctx context.Context, userID, matchID, winnerID, score string) (TournamentMatch, error)
	ConfirmResult(ctx context.Context, userID, matchID string, approve bool) (TournamentMatch, error)
}

type handler struct {
	svc svcFacade
}

// writeErr maps domain sentinel errors to HTTP status codes.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, ErrWrongCode):
		httpx.Error(w, http.StatusForbidden, "invalid join code")
	case errors.Is(err, ErrInvalid):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrWrongState):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrFull):
		httpx.Error(w, http.StatusConflict, "tournament full")
	case errors.Is(err, ErrAlreadyJoined):
		httpx.Error(w, http.StatusConflict, "already joined")
	default:
		// Unmapped errors are real failures (usually DB/query issues); log the
		// underlying cause so it is visible in server logs, not just a bare 500.
		slog.Error("tournament: unexpected error", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "unexpected error")
	}
}

// --- POST /v1/tournaments ---

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		Name             string     `json:"name"`
		Sport            string     `json:"sport"`
		Format           string     `json:"format"`
		MaxParticipants  int        `json:"max_participants"`
		StartsAt         *time.Time `json:"starts_at"`
		SetsToWin        int        `json:"sets_to_win"`
		AdvantageEnabled bool       `json:"advantage_enabled"`
		PointsToWin      int        `json:"points_to_win"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	t, err := h.svc.CreateTournament(r.Context(), userID, CreateInput{
		Name:             body.Name,
		Sport:            body.Sport,
		Format:           body.Format,
		MaxParticipants:  body.MaxParticipants,
		StartsAt:         body.StartsAt,
		SetsToWin:        body.SetsToWin,
		AdvantageEnabled: body.AdvantageEnabled,
		PointsToWin:      body.PointsToWin,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

// --- GET /v1/tournaments/mine ---

func (h *handler) listMine(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	f := ListFilter{
		Query: r.URL.Query().Get("q"),
		Sport: r.URL.Query().Get("sport"),
	}

	items, total, err := h.svc.ListMine(r.Context(), userID, f, page.PageSize, page.Offset())
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

// --- GET /v1/tournaments/{ref} (ref = slug) ---

func (h *handler) getBySlug(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	view, err := h.svc.GetTournament(r.Context(), userID, r.PathValue("ref"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

// --- DELETE /v1/tournaments/{ref} (ref = id) ---

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	if err := h.svc.Delete(r.Context(), userID, r.PathValue("ref")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- POST /v1/tournaments/{ref}/join (ref = slug) ---

func (h *handler) join(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		JoinCode string `json:"join_code"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	p, err := h.svc.JoinTournament(r.Context(), userID, r.PathValue("ref"), body.JoinCode)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// --- GET /v1/tournaments/{ref}/participants (ref = id) ---

func (h *handler) participants(w http.ResponseWriter, r *http.Request) {
	page := httpx.ParsePage(r)
	items, total, err := h.svc.ListParticipants(r.Context(), r.PathValue("ref"), page.PageSize, page.Offset())
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

// --- DELETE /v1/tournaments/{ref}/participants/me (ref = id) ---

func (h *handler) leave(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	if err := h.svc.Leave(r.Context(), userID, r.PathValue("ref")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- POST /v1/tournaments/{ref}/lock (ref = id) ---

func (h *handler) lock(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	t, err := h.svc.Lock(r.Context(), userID, r.PathValue("ref"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// --- POST /v1/tournaments/{ref}/draw (ref = id) ---

func (h *handler) draw(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		Mode    string   `json:"mode"`
		Seeding []string `json:"seeding"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	b, err := h.svc.Draw(r.Context(), userID, r.PathValue("ref"), body.Mode, body.Seeding)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, b)
}

// --- GET /v1/tournaments/{ref}/bracket (ref = id) ---

func (h *handler) bracket(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.GetBracket(r.Context(), r.PathValue("ref"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, b)
}

// --- POST /v1/tournaments/matches/{matchId}/result ---

func (h *handler) result(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		WinnerParticipantID string `json:"winner_participant_id"`
		ScoreSummary        string `json:"score_summary"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.WinnerParticipantID == "" {
		httpx.Error(w, http.StatusBadRequest, "winner_participant_id required")
		return
	}

	m, err := h.svc.SubmitResult(r.Context(), userID, r.PathValue("matchId"), body.WinnerParticipantID, body.ScoreSummary)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

// --- POST /v1/tournaments/matches/{matchId}/confirm ---

func (h *handler) confirm(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		Approve bool `json:"approve"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	m, err := h.svc.ConfirmResult(r.Context(), userID, r.PathValue("matchId"), body.Approve)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}
