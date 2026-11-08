package httpapi

import (
	"github.com/arcvpn/arcvpn/internal/store"
	"net/http"
)

func (s *Server) usersBulkCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Names []string `json:"names"`
		Group string   `json:"group"`
	}
	if !decode(w, r, &in) {
		return
	}
	users, err := s.App.CreateUsers(r.Context(), in.Names, in.Group)
	if err != nil {
		userEditError(w, err)
		return
	}
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	state, _ := s.App.Store.AccountingState(r.Context())
	respond(w, 201, map[string]any{"saved": true, "ids": ids, "count": len(ids), "runtime_state": state})
}
func (s *Server) usersBulkEdit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items   []store.UserBatchSelection `json:"items"`
		Changes store.UserBatchChanges     `json:"changes"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.App.BulkEditUsers(r.Context(), in.Items, in.Changes); err != nil {
		userEditError(w, err)
		return
	}
	state, _ := s.App.Store.AccountingState(r.Context())
	respond(w, 200, map[string]any{"saved": true, "count": len(in.Items), "runtime_state": state})
}
