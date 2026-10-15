package handlers

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
)

const (
	MaxImagesPerAsset = 5
	maxImageBytes     = 2 << 20 // 2 MiB per photo, same as the Laravel rule
	imgMaxW           = 480
	imgMaxH           = 360
)

// ImageStore saves resized asset photos below Dir.
type ImageStore struct{ Dir string }

func (s ImageStore) path(name string) string {
	return filepath.Join(s.Dir, "assets", filepath.Base(name))
}

// Save validates, resizes (never upscales) and stores one uploaded photo.
// It returns the generated file name.
func (s ImageStore) Save(fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxImageBytes {
		return "", errors.New("ukuran foto maksimal 2MB")
	}
	f, err := fh.Open()
	if err != nil {
		return "", errors.New("foto tidak dapat dibaca")
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := f.Read(head)
	var ext string
	switch http.DetectContentType(head[:n]) {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	default:
		return "", errors.New("foto harus berformat jpeg, png, atau gif")
	}
	if _, err := f.Seek(0, 0); err != nil {
		return "", errors.New("foto tidak dapat dibaca")
	}

	img, err := imaging.Decode(f, imaging.AutoOrientation(true))
	if err != nil {
		return "", errors.New("file bukan gambar yang valid")
	}
	img = imaging.Fit(img, imgMaxW, imgMaxH, imaging.Lanczos)

	if err := os.MkdirAll(filepath.Join(s.Dir, "assets"), 0o755); err != nil {
		return "", err
	}
	name := uuid.NewString() + ext
	if err := imaging.Save(img, s.path(name), imaging.JPEGQuality(85)); err != nil {
		return "", fmt.Errorf("simpan foto: %w", err)
	}
	return name, nil
}

func (s ImageStore) Remove(names ...string) {
	for _, n := range names {
		if n == "" || strings.ContainsAny(n, `/\`) {
			continue
		}
		_ = os.Remove(s.path(n))
	}
}

func (s ImageStore) Serve(w http.ResponseWriter, r *http.Request, name string) {
	p := s.path(name)
	if _, err := os.Stat(p); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, p)
}
