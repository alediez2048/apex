package vendor

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// EnsureStubImages writes minimal placeholder PNGs under baseDir for demo/tests (TICKET-004).
// Not tied to a specific transfer ID yet; pipeline will write per-transfer images later.
func EnsureStubImages(baseDir string) error {
	if baseDir == "" {
		baseDir = "data/images"
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}
	names := []string{"stub_front.png", "stub_back.png"}
	for _, name := range names {
		path := filepath.Join(baseDir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		img := image.NewRGBA(image.Rect(0, 0, 64, 32))
		for y := 0; y < 32; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, color.RGBA{R: 240, G: 240, B: 240, A: 255})
			}
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}
