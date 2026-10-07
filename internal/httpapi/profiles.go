package httpapi

import (
	"errors"
	"net/http"

	"github.com/wimmme/shellylanman/internal/service"
)

// profileRoutes: the profiles a new Shelly gets (DECISIONS §29).
func (s *server) profileRoutes(mux router, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/profiles", h(s.listProfiles))
	mux.HandleFunc("POST /api/v1/profiles", h(s.createProfile))
	mux.HandleFunc("PUT /api/v1/profiles/{id}", h(s.updateProfile))
	mux.HandleFunc("DELETE /api/v1/profiles/{id}", h(s.deleteProfile))
	mux.HandleFunc("GET /api/v1/profiles/{id}/plan", h(s.profilePlan))
	mux.HandleFunc("POST /api/v1/profiles/{id}/apply", h(s.applyProfile))
}

func profileErrorCode(err error) int {
	switch {
	case errors.Is(err, service.ErrNoProfile):
		return http.StatusNotFound
	case errors.Is(err, service.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrConfirm):
		return http.StatusPreconditionRequired
	case errors.Is(err, service.ErrBadCommand):
		return http.StatusConflict
	}
	return deviceErrorCode(err)
}

func (s *server) listProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.Devices.Profiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) createProfile(w http.ResponseWriter, r *http.Request) {
	var in service.ProfileInput
	if !readJSON(w, r, &in) {
		return
	}
	in.ID = "" // a new one
	v, err := s.Devices.SaveProfile(in)
	if err != nil {
		writeError(w, profileErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *server) updateProfile(w http.ResponseWriter, r *http.Request) {
	var in service.ProfileInput
	if !readJSON(w, r, &in) {
		return
	}
	in.ID = r.PathValue("id")
	v, err := s.Devices.SaveProfile(in)
	if err != nil {
		writeError(w, profileErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if err := s.Devices.DeleteProfile(r.PathValue("id")); err != nil {
		writeError(w, profileErrorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// profilePlan: what the profile would do to ?device=.
func (s *server) profilePlan(w http.ResponseWriter, r *http.Request) {
	plan, err := s.Devices.ProfilePlan(r.PathValue("id"), r.URL.Query().Get("device"))
	if err != nil {
		writeError(w, profileErrorCode(err), err.Error())
		return
	}
	if plan == nil {
		plan = []service.PlanStep{}
	}
	writeJSON(w, http.StatusOK, plan)
}

// applyProfile sets the profile on a device that is on the network; it changes the device, so it needs confirm=true.
func (s *server) applyProfile(w http.ResponseWriter, r *http.Request) {
	var body applyProfileBody
	if !readJSON(w, r, &body) {
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusPreconditionRequired, service.ErrConfirm.Error())
		return
	}
	steps, err := s.Devices.ApplyProfile(r.Context(), r.PathValue("id"), body.Device)
	if err != nil {
		writeError(w, profileErrorCode(err), err.Error())
		return
	}
	if steps == nil {
		steps = []service.ProfileStep{}
	}
	writeJSON(w, http.StatusOK, profileResult{Steps: steps})
}
