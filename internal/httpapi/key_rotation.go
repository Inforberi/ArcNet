package httpapi

import "net/http"

func (s *Server) rotationPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	plan, err := s.App.PreviewKeyRotation(r.Context(), r.PathValue("kind"))
	if err != nil {
		connectionEditFailure(w, err)
		return
	}
	respond(w, 200, plan)
}
func (s *Server) rotationApply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision     string `json:"revision"`
		Confirmation string `json:"confirmation"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Revision) != 64 || len(in.Confirmation) != 64 {
		fail(w, 422, "Обновите план смены ключей перед подтверждением")
		return
	}
	if err := s.App.RotateConnectionKeys(r.Context(), r.PathValue("kind"), in.Revision, in.Confirmation); err != nil {
		connectionEditFailure(w, err)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
