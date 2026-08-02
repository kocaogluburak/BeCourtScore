package score

import (
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

func (h *handler) startLiveMatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Sport            string  `json:"sport"`
		PlayerAName      string  `json:"player_a_name"`
		PlayerBName      string  `json:"player_b_name"`
		PlayerAUserID    *string `json:"player_a_user_id"`
		PlayerBUserID    *string `json:"player_b_user_id"`
		SetsToWin        int     `json:"sets_to_win"`
		AdvantageEnabled *bool   `json:"advantage_enabled"`
		PointsToWin      int     `json:"points_to_win"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	adv := true
	if body.AdvantageEnabled != nil {
		adv = *body.AdvantageEnabled
	}
	m, err := h.svc.StartLiveMatch(r.Context(), userID, LiveStartInput{
		Sport:            body.Sport,
		PlayerAName:      body.PlayerAName,
		PlayerBName:      body.PlayerBName,
		PlayerAUserID:    body.PlayerAUserID,
		PlayerBUserID:    body.PlayerBUserID,
		SetsToWin:        body.SetsToWin,
		AdvantageEnabled: adv,
		PointsToWin:      body.PointsToWin,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			httpx.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrForbidden):
			httpx.Error(w, http.StatusForbidden, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to start live match")
		}
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

func (h *handler) getLiveMatch(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	m, err := h.svc.GetLiveMatch(r.Context(), viewerID, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "live match not found")
		case errors.Is(err, ErrForbidden):
			httpx.Error(w, http.StatusForbidden, "forbidden")
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to get live match")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (h *handler) updateLiveMatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	var body struct {
		SetsA      *int    `json:"sets_a"`
		SetsB      *int    `json:"sets_b"`
		GamesA     *int    `json:"games_a"`
		GamesB     *int    `json:"games_b"`
		ScoreA     *int    `json:"score_a"`
		ScoreB     *int    `json:"score_b"`
		IsTieBreak *bool   `json:"is_tie_break"`
		WinnerSide *string `json:"winner_side"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := h.svc.UpdateLiveMatch(r.Context(), userID, id, LiveScoreUpdate{
		SetsA: body.SetsA, SetsB: body.SetsB,
		GamesA: body.GamesA, GamesB: body.GamesB,
		ScoreA: body.ScoreA, ScoreB: body.ScoreB,
		IsTieBreak: body.IsTieBreak, WinnerSide: body.WinnerSide,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			httpx.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "live match not found")
		case errors.Is(err, ErrForbidden):
			httpx.Error(w, http.StatusForbidden, "forbidden")
		case errors.Is(err, ErrWrongState):
			httpx.Error(w, http.StatusConflict, "live match is not in progress")
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to update live match")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (h *handler) endLiveMatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	var body struct {
		SetsA      int    `json:"sets_a"`
		SetsB      int    `json:"sets_b"`
		GamesA     int    `json:"games_a"`
		GamesB     int    `json:"games_b"`
		ScoreA     int    `json:"score_a"`
		ScoreB     int    `json:"score_b"`
		IsTieBreak bool   `json:"is_tie_break"`
		WinnerSide string `json:"winner_side"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := h.svc.EndLiveMatch(r.Context(), userID, id, LiveEndInput{
		SetsA: body.SetsA, SetsB: body.SetsB,
		GamesA: body.GamesA, GamesB: body.GamesB,
		ScoreA: body.ScoreA, ScoreB: body.ScoreB,
		IsTieBreak: body.IsTieBreak, WinnerSide: body.WinnerSide,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			httpx.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "live match not found")
		case errors.Is(err, ErrForbidden):
			httpx.Error(w, http.StatusForbidden, "forbidden")
		case errors.Is(err, ErrWrongState):
			httpx.Error(w, http.StatusConflict, "live match already ended")
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to end live match")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}
