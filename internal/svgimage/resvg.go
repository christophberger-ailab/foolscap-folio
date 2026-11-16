package svgimage

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
)

// The images are drawn by resvg (https://github.com/linebender/resvg),
// compiled to WebAssembly from ./resvg and run by wazero: a complete SVG
// renderer without cgo. The module imports nothing, so an image can reach
// neither files nor the network.
//
//go:generate sh -c "cd resvg && cargo build --release --target wasm32-unknown-unknown && gzip -9n -c target/wasm32-unknown-unknown/release/folio_resvg.wasm > ../resvg.wasm.gz"
//go:embed resvg.wasm.gz
var resvgWasm []byte

// maxPages caps the module's memory at 1 GiB (64 KiB pages), which an
// image of some 15000×15000 pixels fills.
const maxPages = 16384

// fonts are what the images' text is set in, the first standing in for
// any family not among them.
var fonts = [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF, gomono.TTF, gomonobold.TTF}

// renderer is the one instance of the module; mu serializes its use.
var renderer struct {
	mu  sync.Mutex
	rt  wazero.Runtime
	mod api.Module
}

// draw renders an SVG image into a w×h image.
func draw(svg []byte, w, h int) (*image.RGBA, error) {
	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	ctx := context.Background()
	if renderer.mod == nil {
		rt := wazero.NewRuntimeWithConfig(ctx, runtimeConfig())
		mod, err := instantiate(ctx, rt)
		if err != nil {
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("svg renderer: %w", err)
		}
		renderer.rt, renderer.mod = rt, mod
	}
	pix, err := call(ctx, renderer.mod, svg, w, h)
	if err != nil {
		var failed renderError
		if !errors.As(err, &failed) {
			// A trap, such as running out of memory, may leave the
			// module broken: start afresh next time.
			_ = renderer.rt.Close(ctx)
			renderer.rt, renderer.mod = nil, nil
		}
		return nil, err
	}
	return &image.RGBA{Pix: pix, Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}, nil
}

// renderError is resvg's own refusal of an image, as opposed to a failure
// of the module.
type renderError string

func (e renderError) Error() string { return "svg: " + string(e) }

// runtimeConfig caps the memory and keeps the module's machine code in
// the user's cache directory: compiling it takes a second or so, loading
// it a fraction of that.
func runtimeConfig() wazero.RuntimeConfig {
	cfg := wazero.NewRuntimeConfig().WithMemoryLimitPages(maxPages)
	if dir, err := os.UserCacheDir(); err == nil {
		if cache, err := wazero.NewCompilationCacheWithDir(filepath.Join(dir, "folio", "wazero")); err == nil {
			cfg = cfg.WithCompilationCache(cache)
		}
	}
	return cfg
}

func instantiate(ctx context.Context, rt wazero.Runtime) (api.Module, error) {
	zr, err := gzip.NewReader(bytes.NewReader(resvgWasm))
	if err != nil {
		return nil, err
	}
	code, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	mod, err := rt.InstantiateWithConfig(ctx, code, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, err
	}
	for _, f := range fonts {
		ptr, err := put(ctx, mod, f)
		if err != nil {
			return nil, err
		}
		if _, err := mod.ExportedFunction("add_font").Call(ctx, ptr, uint64(len(f))); err != nil {
			return nil, err
		}
	}
	return mod, nil
}

// put copies data into memory the module allocates for it.
func put(ctx context.Context, mod api.Module, data []byte) (uint64, error) {
	res, err := mod.ExportedFunction("alloc").Call(ctx, uint64(len(data)))
	if err != nil {
		return 0, err
	}
	if !mod.Memory().Write(uint32(res[0]), data) {
		return 0, errors.New("alloc returned memory out of range")
	}
	return res[0], nil
}

func call(ctx context.Context, mod api.Module, svg []byte, w, h int) ([]byte, error) {
	ptr, err := put(ctx, mod, svg)
	if err != nil {
		return nil, err
	}
	defer mod.ExportedFunction("dealloc").Call(ctx, ptr, uint64(len(svg)))
	status, err := mod.ExportedFunction("render").Call(ctx, ptr, uint64(len(svg)), uint64(w), uint64(h))
	if err != nil {
		return nil, err
	}
	at, err := mod.ExportedFunction("result_ptr").Call(ctx)
	if err != nil {
		return nil, err
	}
	n, err := mod.ExportedFunction("result_len").Call(ctx)
	if err != nil {
		return nil, err
	}
	out, ok := mod.Memory().Read(uint32(at[0]), uint32(n[0]))
	if !ok {
		return nil, errors.New("svg renderer: result out of range")
	}
	out = bytes.Clone(out) // the view is the module's memory, reused by the next call
	if status[0] != 0 {
		return nil, renderError(out)
	}
	if len(out) != 4*w*h {
		return nil, fmt.Errorf("svg renderer: %d bytes of pixels for %d×%d", len(out), w, h)
	}
	return out, nil
}
