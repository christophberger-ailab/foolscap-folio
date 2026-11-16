package svgimage

import (
	"os"
	"testing"
	"time"
)

// BenchmarkPNG draws the built-in placeholder as a document embeds it.
func BenchmarkPNG(b *testing.B) {
	if _, err := PNG(builtIn, 4); err != nil { // the module's start is not drawing
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := PNG(builtIn, 4); err != nil {
			b.Fatal(err)
		}
	}
}

// TestStartupTime reports how long the renderer takes to start, with
// -v; FOLIO_SVG_STARTUP sets a limit to fail on.
func TestStartupTime(t *testing.T) {
	renderer.mu.Lock()
	if renderer.rt != nil {
		renderer.rt.Close(t.Context())
		renderer.rt, renderer.mod = nil, nil
	}
	renderer.mu.Unlock()
	start := time.Now()
	if _, err := Rasterize([]byte(square), 1); err != nil {
		t.Fatal(err)
	}
	d := time.Since(start)
	t.Logf("renderer start and first image: %v", d)
	if limit, err := time.ParseDuration(os.Getenv("FOLIO_SVG_STARTUP")); err == nil && d > limit {
		t.Errorf("start took %v, over %v", d, limit)
	}
}

func BenchmarkRasterize(b *testing.B) {
	if _, err := Rasterize(builtIn, 4); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := Rasterize(builtIn, 4); err != nil {
			b.Fatal(err)
		}
	}
}
