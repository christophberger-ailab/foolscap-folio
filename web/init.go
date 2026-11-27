package web

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/initialize"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// initView offers to start a project in an open folder that has no
// folio.yml, as `folio init` does; Demo offers the demo project too,
// which only an empty folder can take.
type initView struct {
	Demo  bool
	Error string
}

// initOffer is what the open folder is offered, or nil for a project.
// The caller must hold s.mu.
func (s *server) initOffer() *initView {
	if s.root == "" || config.Find(s.root) != "" {
		return nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return &initView{}
		}
	}
	return &initView{Demo: true}
}

// initHandler starts a project in the open folder -- with the demo
// project if the form says demo -- and shows it. It works on the open
// folder only, never on a path the request names. A refusal is shown in
// the offer.
func (s *server) initHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initOffer() == nil {
		http.Error(w, "no folder without a folio.yml is open", http.StatusBadRequest)
		return
	}
	_, err := initialize.Run(s.root, initialize.Options{
		Demo:      r.FormValue("demo") != "",
		UserBrand: userBrandIn(s.configFile),
	})
	if err == nil {
		err = s.openProject(w, s.root)
	}
	if err == nil {
		return
	}
	st, lerr := s.load()
	if lerr != nil {
		http.Error(w, errors.Join(err, lerr).Error(), http.StatusInternalServerError)
		return
	}
	if st.Init == nil {
		st.Init = &initView{}
	}
	st.Init.Error = err.Error()
	s.render(w, "main", st)
}
