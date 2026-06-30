package readings

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bloodpressure/backend/internal/auth"
	"bloodpressure/backend/internal/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type readingRequest struct {
	Systolic   int     `json:"systolic"`
	Diastolic  int     `json:"diastolic"`
	Pulse      *int    `json:"pulse"`
	MeasuredAt string  `json:"measuredAt"`
	Note       *string `json:"note"`
	UpdatedAt  string  `json:"updatedAt"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireMethod(w, r, http.MethodGet) {
		return
	}
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var since *time.Time
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid since parameter")
			return
		}
		since = &parsed
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	readings, err := h.service.List(r.Context(), userID, since, limit)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if readings == nil {
		readings = []Reading{}
	}
	httputil.WriteJSON(w, http.StatusOK, readingsToJSON(readings))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireMethod(w, r, http.MethodGet) {
		return
	}
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/readings/")
	if id == "" || strings.Contains(id, "/") {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	reading, err := h.service.Get(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, readingToJSON(reading))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireMethod(w, r, http.MethodPost) {
		return
	}
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	input, err := parseReadingRequest(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	reading, err := h.service.Create(r.Context(), userID, input)
	if err != nil {
		if errors.Is(err, ErrInvalidReading) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid reading")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, readingToJSON(reading))
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireMethod(w, r, http.MethodPut) {
		return
	}
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/readings/")
	if id == "" || strings.Contains(id, "/") {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	req, err := decodeReadingRequest(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	measuredAt, err := time.Parse(time.RFC3339, req.MeasuredAt)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid measuredAt")
		return
	}
	updatedAt := time.Now().UTC()
	if req.UpdatedAt != "" {
		updatedAt, err = time.Parse(time.RFC3339, req.UpdatedAt)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid updatedAt")
			return
		}
	}

	reading, err := h.service.Update(r.Context(), userID, id, UpdateInput{
		Systolic:      req.Systolic,
		Diastolic:     req.Diastolic,
		Pulse:         req.Pulse,
		MeasuredAt:    measuredAt,
		Note:          req.Note,
		ClientUpdated: updatedAt,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httputil.WriteError(w, http.StatusNotFound, "not found")
		case errors.Is(err, ErrConflict):
			httputil.WriteError(w, http.StatusConflict, "conflict")
		case errors.Is(err, ErrInvalidReading):
			httputil.WriteError(w, http.StatusBadRequest, "invalid reading")
		default:
			httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	httputil.WriteJSON(w, http.StatusOK, readingToJSON(reading))
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireMethod(w, r, http.MethodDelete) {
		return
	}
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/readings/")
	if id == "" || strings.Contains(id, "/") {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.service.Delete(r.Context(), userID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireMethod(w, r, http.MethodPost) {
		return
	}
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req SyncRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	response, err := h.service.Sync(r.Context(), userID, req)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, syncResponseToJSON(response))
}

func parseReadingRequest(r *http.Request) (CreateInput, error) {
	req, err := decodeReadingRequest(r)
	if err != nil {
		return CreateInput{}, err
	}
	measuredAt, err := time.Parse(time.RFC3339, req.MeasuredAt)
	if err != nil {
		return CreateInput{}, fmt.Errorf("invalid measuredAt")
	}
	return CreateInput{
		Systolic:   req.Systolic,
		Diastolic:  req.Diastolic,
		Pulse:      req.Pulse,
		MeasuredAt: measuredAt,
		Note:       req.Note,
	}, nil
}

func decodeReadingRequest(r *http.Request) (readingRequest, error) {
	var req readingRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		return readingRequest{}, fmt.Errorf("invalid request body")
	}
	return req, nil
}
