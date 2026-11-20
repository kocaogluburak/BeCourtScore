package equipment

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/entitlement"
	"courtscore/internal/platform/httpx"
)

type svcFacade interface {
	List(ctx context.Context, userID string, limit, offset int) ([]Usage, int64, error)
	Create(ctx context.Context, userID string, in CreateInput) (Usage, error)
	Get(ctx context.Context, userID, id string) (Usage, error)
	Update(ctx context.Context, userID, id string, in UpdateInput) (Usage, error)
	Delete(ctx context.Context, userID, id string) error
	Activate(ctx context.Context, userID, id string) (Usage, error)
	Reset(ctx context.Context, userID, id string) (Usage, error)
	GetCatalog(ctx context.Context, sport string) ([]CatalogView, error)
}

type handler struct {
	svc svcFacade
}

func writeErr(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, entitlement.ErrProRequired):
		httpx.ProRequired(w)
	case errors.Is(err, ErrInvalid):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "equipment not found")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden")
	default:
		httpx.Error(w, http.StatusInternalServerError, fallback)
	}
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	items, total, err := h.svc.List(r.Context(), userID, page.PageSize, page.Offset())
	if err != nil {
		writeErr(w, err, "failed to list equipment")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Type          string  `json:"type"`
		Name          string  `json:"name"`
		LifespanHours int     `json:"lifespan_hours"`
		PurchaseDate  *string `json:"purchase_date"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.Create(r.Context(), userID, CreateInput{
		Type: body.Type, Name: body.Name, LifespanHours: body.LifespanHours, PurchaseDate: body.PurchaseDate,
	})
	if err != nil {
		writeErr(w, err, "failed to create equipment")
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	out, err := h.svc.Get(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err, "failed to get equipment")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	var body struct {
		Name          *string `json:"name"`
		LifespanHours *int    `json:"lifespan_hours"`
		PurchaseDate  *string `json:"purchase_date"`
		IsActive      *bool   `json:"is_active"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.Update(r.Context(), userID, id, UpdateInput{
		Name:            body.Name,
		LifespanHours:   body.LifespanHours,
		PurchaseDate:    body.PurchaseDate,
		SetPurchaseDate: body.PurchaseDate != nil,
		IsActive:        body.IsActive,
	})
	if err != nil {
		writeErr(w, err, "failed to update equipment")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) del(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	if err := h.svc.Delete(r.Context(), userID, id); err != nil {
		writeErr(w, err, "failed to delete equipment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) activate(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	out, err := h.svc.Activate(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err, "failed to activate equipment")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) reset(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	out, err := h.svc.Reset(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err, "failed to reset equipment")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// catalog serves GET /v1/equipment/catalog?sport=. With sport: one {sport, categories}
// object; without: array of all five sports. Intentionally not paginated (02-technical §2.2).
func (h *handler) catalog(w http.ResponseWriter, r *http.Request) {
	sport := r.URL.Query().Get("sport")
	views, err := h.svc.GetCatalog(r.Context(), sport)
	if err != nil {
		writeErr(w, err, "failed to load equipment catalog")
		return
	}
	if strings.TrimSpace(sport) != "" && len(views) == 1 {
		httpx.JSON(w, http.StatusOK, views[0])
		return
	}
	httpx.JSON(w, http.StatusOK, views)
}
