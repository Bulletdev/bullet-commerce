package handlers

// ReviewHandler wires the product reviews feature. Routes for the integrator to register in
// cmd/main.go (parsePagination and getAuthenticatedUserID are shared helpers in this package):
//
//	POST   /api/products/{id}/reviews   -> (*ReviewHandler).CreateReview   [AUTH: authenticated user; user_id from JWT context]
//	GET    /api/products/{id}/reviews   -> (*ReviewHandler).ListReviews     [PUBLIC; approved only, paginated ?limit&offset]
//	PATCH  /api/reviews/{id}/moderate   -> (*ReviewHandler).ModerateReview  [AUTH: admin]
//
// Constructor:  NewReviewHandler(reviewRepo reviews.ReviewRepository) *ReviewHandler
//
// The handler recomputes the product's rating aggregate (products.rating_avg / rating_count)
// after every change to the approved set — i.e. on CreateReview and on ModerateReview.

import (
	"bullet-commerce/internal/reviews"
	"bullet-commerce/internal/webutils"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type ReviewHandler struct {
	ReviewRepo reviews.ReviewRepository
}

func NewReviewHandler(reviewRepo reviews.ReviewRepository) *ReviewHandler {
	return &ReviewHandler{ReviewRepo: reviewRepo}
}

type CreateReviewRequest struct {
	Rating int     `json:"rating"`
	Title  *string `json:"title,omitempty"`
	Body   *string `json:"body,omitempty"`
}

type ModerateReviewRequest struct {
	Status string `json:"status"`
}

// validReviewStatus gates the moderation target status.
func validReviewStatus(s string) bool {
	switch s {
	case reviews.ReviewStatusPending, reviews.ReviewStatusApproved, reviews.ReviewStatusRejected:
		return true
	default:
		return false
	}
}

// CreateReview handles POST /api/products/{id}/reviews (authenticated). The author is taken
// from the JWT context, never from the body, so a user can only post reviews as themselves.
func (h *ReviewHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	var req CreateReviewRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		webutils.ErrorJSON(w, errors.New("rating must be between 1 and 5"), http.StatusBadRequest)
		return
	}

	review := &reviews.Review{
		ProductID: productID,
		UserID:    authUserID,
		Rating:    req.Rating,
		Title:     req.Title,
		Body:      req.Body,
	}
	if err := h.ReviewRepo.Create(r.Context(), review); err != nil {
		if errors.Is(err, reviews.ErrDuplicateReview) {
			webutils.ErrorJSON(w, err, http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to create review"), http.StatusInternalServerError)
		}
		return
	}

	// Keep the denormalized product aggregate current. The review already persisted, so a
	// recompute failure is logged rather than surfaced as a 4xx/5xx to the client.
	if err := h.ReviewRepo.RecomputeAggregate(r.Context(), productID); err != nil {
		slog.Error("failed to recompute rating aggregate", "product_id", productID, "error", err)
	}

	webutils.WriteJSON(w, http.StatusCreated, review)
}

// ListReviews handles GET /api/products/{id}/reviews (public). Only approved reviews are
// returned, newest-first and paginated.
func (h *ReviewHandler) ListReviews(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	limit, offset := parsePagination(r)
	list, err := h.ReviewRepo.ListByProduct(r.Context(), productID, true, limit, offset)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve reviews"), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []reviews.Review{}
	}
	webutils.WriteJSON(w, http.StatusOK, list)
}

// ModerateReview handles PATCH /api/reviews/{id}/moderate (admin). Changing a review's status
// changes the approved set, so the affected product's aggregate is recomputed afterwards.
func (h *ReviewHandler) ModerateReview(w http.ResponseWriter, r *http.Request) {
	reviewID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid review ID format"), http.StatusBadRequest)
		return
	}

	var req ModerateReviewRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if !validReviewStatus(req.Status) {
		webutils.ErrorJSON(w, errors.New("invalid review status"), http.StatusBadRequest)
		return
	}

	productID, err := h.ReviewRepo.Moderate(r.Context(), reviewID, req.Status)
	if err != nil {
		if errors.Is(err, reviews.ErrReviewNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to moderate review"), http.StatusInternalServerError)
		}
		return
	}

	if err := h.ReviewRepo.RecomputeAggregate(r.Context(), productID); err != nil {
		slog.Error("failed to recompute rating aggregate", "product_id", productID, "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
}
