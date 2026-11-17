package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"

	"github.com/christophberger-ailab/foolscap-folio/internal/mermaid"
)

// maxDiagram bounds the Mermaid source /diagram takes.
const maxDiagram = 256 << 10

// diagram draws the Mermaid source in the request body as SVG, the way a
// render draws a ```{mermaid} block, for the preview to show. A diagram
// that cannot be drawn is answered with the reason and 422. The ids in the
// SVG derive from the source, so two different diagrams on a page do not
// share them.
func (s *server) diagram(w http.ResponseWriter, r *http.Request) {
	src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDiagram))
	if err != nil {
		code := http.StatusBadRequest
		if errors.As(err, new(*http.MaxBytesError)) {
			code = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), code)
		return
	}
	sum := sha256.Sum256(src)
	svg, err := mermaid.Render(string(src), "mermaid-"+hex.EncodeToString(sum[:6]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.WriteString(w, svg)
}
