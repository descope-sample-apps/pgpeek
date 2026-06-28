package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/descope-sample-apps/pgpeek/internal/guard"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

func (s *Server) handleListQueries(w http.ResponseWriter, r *http.Request) {
	qs, err := s.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list saved queries")
		s.log.Error("list saved queries", "err", err)
		return
	}
	if qs == nil {
		qs = []store.SavedQuery{}
	}
	writeJSON(w, http.StatusOK, qs)
}

type savedQueryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SQL         string `json:"sql"`
}

func (s *Server) handleCreateQuery(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeSavedQuery(w, r)
	if !ok {
		return
	}
	q, err := s.store.Create(r.Context(), req.Name, req.Description, req.SQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save query")
		s.log.Error("create saved query", "err", err)
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

func (s *Server) handleUpdateQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	req, ok := decodeSavedQuery(w, r)
	if !ok {
		return
	}
	q, err := s.store.Update(r.Context(), id, req.Name, req.Description, req.SQL)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved query not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update query")
		s.log.Error("update saved query", "err", err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) handleDeleteQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.store.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved query not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete query")
		s.log.Error("delete saved query", "err", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeSavedQuery(w http.ResponseWriter, r *http.Request) (savedQueryRequest, bool) {
	var req savedQueryRequest
	if !decodeBody(w, r, &req) {
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	req.SQL = strings.TrimSpace(req.SQL)
	if req.Name == "" || req.SQL == "" {
		writeError(w, http.StatusBadRequest, "name and sql are required")
		return req, false
	}
	if err := guard.Validate(req.SQL); err != nil {
		writeError(w, http.StatusBadRequest, "saved query must be read-only: "+err.Error())
		return req, false
	}
	return req, true
}
