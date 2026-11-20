package equipment

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/entitlement"
	"courtscore/internal/platform/httpx"
)

type svcFacade interface {
	List(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Usage, int64, error)
	Create(ctx context.Context, userID string, in CreateInput) (Usage, error)
	Get(ctx context.Context, userID, id string) (Usage, error)
	Update(ctx context.Context, userID, id string, in UpdateInput) (Usage, error)
	Delete(ctx context.Context, userID, id string) error
	Activate(ctx context.Context, userID, id string) (Usage, error)
	Reset(ctx context.Context, userID, id string) (Usage, error)
	Archive(ctx context.Context, userID, id string) (Usage, error)
	Renew(ctx context.Context, userID, id string, in RenewInput) (Usage, error)
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
	case errors.Is(err, ErrConflict):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden")
	default:
		httpx.Error(w, http.StatusInternalServerError, fallback)
	}
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	q := r.URL.Query()
	items, total, err := h.svc.List(r.Context(), userID, ListFilter{
		View: q.Get("view"), Sport: q.Get("sport"), CategoryID: q.Get("category_id"),
	}, page.PageSize, page.Offset())
	if err != nil {
		writeErr(w, err, "failed to list equipment")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Sport           string            `json:"sport"`
		CategoryID      string            `json:"category_id"`
		BrandID         *string           `json:"brand_id"`
		CustomBrandName *string           `json:"custom_brand_name"`
		ModelID         *string           `json:"model_id"`
		CustomModelName *string           `json:"custom_model_name"`
		Nickname        *string           `json:"nickname"`
		LifespanHours   *int              `json:"lifespan_hours"`
		LifespanDays    *int              `json:"lifespan_days"`
		PurchaseDate    *string           `json:"purchase_date"`
		Attributes      map[string]string `json:"attributes"`
		ParentItemID    *string           `json:"parent_item_id"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.Create(r.Context(), userID, CreateInput{
		Sport: body.Sport, CategoryID: body.CategoryID,
		BrandID: body.BrandID, CustomBrandName: body.CustomBrandName,
		ModelID: body.ModelID, CustomModelName: body.CustomModelName,
		Nickname: body.Nickname, LifespanHours: body.LifespanHours, LifespanDays: body.LifespanDays,
		PurchaseDate: body.PurchaseDate, Attributes: body.Attributes, ParentItemID: body.ParentItemID,
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
		Nickname        *string           `json:"nickname"`
		Name            *string           `json:"name"` // legacy alias of nickname
		BrandID         *string           `json:"brand_id"`
		CustomBrandName *string           `json:"custom_brand_name"`
		ModelID         *string           `json:"model_id"`
		CustomModelName *string           `json:"custom_model_name"`
		LifespanHours   *int              `json:"lifespan_hours"`
		LifespanDays    *int              `json:"lifespan_days"`
		PurchaseDate    *string           `json:"purchase_date"`
		Attributes      map[string]string `json:"attributes"`
		ParentItemID    *string           `json:"parent_item_id"`
		IsActive        *bool             `json:"is_active"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	nick := body.Nickname
	if nick == nil {
		nick = body.Name
	}
	out, err := h.svc.Update(r.Context(), userID, id, UpdateInput{
		Nickname:        nick,
		SetBrand:        body.BrandID != nil || body.CustomBrandName != nil,
		BrandID:         body.BrandID,
		CustomBrandName: body.CustomBrandName,
		SetModel:        body.ModelID != nil || body.CustomModelName != nil,
		ModelID:         body.ModelID,
		CustomModelName: body.CustomModelName,
		LifespanHours:   body.LifespanHours,
		LifespanDays:    body.LifespanDays,
		PurchaseDate:    body.PurchaseDate,
		SetPurchaseDate: body.PurchaseDate != nil,
		Attributes:      body.Attributes,
		SetParent:       body.ParentItemID != nil,
		ParentItemID:    body.ParentItemID,
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

func (h *handler) archive(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	out, err := h.svc.Archive(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err, "failed to archive equipment")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// renew: the body is optional; an empty body copies everything from the old row.
func (h *handler) renew(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	var body struct {
		BrandID         *string           `json:"brand_id"`
		CustomBrandName *string           `json:"custom_brand_name"`
		ModelID         *string           `json:"model_id"`
		CustomModelName *string           `json:"custom_model_name"`
		Attributes      map[string]string `json:"attributes"`
	}
	if err := httpx.Decode(r, &body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.Renew(r.Context(), userID, id, RenewInput{
		BrandID: body.BrandID, CustomBrandName: body.CustomBrandName,
		ModelID: body.ModelID, CustomModelName: body.CustomModelName, Attributes: body.Attributes,
	})
	if err != nil {
		writeErr(w, err, "failed to renew equipment")
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
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
