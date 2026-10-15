package handlers

import (
	"bytes"
	"image/png"
	"net/http"
	"net/url"
	"strconv"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/qr"
)

// BaseURL is the address encoded into QR stickers: PUBLIC_BASE_URL when set,
// otherwise the address the browser used to reach the app.
func BaseURL(configured string, r *http.Request) string {
	if configured != "" {
		return configured
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// AssetURL is what a phone opens after scanning the sticker of an asset.
func AssetURL(base, tag string) string { return base + "/a/" + url.PathEscape(tag) }

func sizeParam(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("size"))
	if err != nil {
		return def
	}
	if n < 100 {
		return 100
	}
	if n > 1000 {
		return 1000
	}
	return n
}

// QRPNG renders content as a QR code with error correction level M.
func QRPNG(content string, size int) ([]byte, error) {
	bc, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return nil, err
	}
	bc, err = barcode.Scale(bc, size, size)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = png.Encode(&buf, bc)
	return buf.Bytes(), err
}

// BarcodePNG renders the tag as a Code 128 linear barcode (handheld scanners).
func BarcodePNG(tag string, width int) ([]byte, error) {
	bc, err := code128.Encode(tag)
	if err != nil {
		return nil, err
	}
	if w := bc.Bounds().Dx(); width < w {
		width = w
	}
	scaled, err := barcode.Scale(bc, width, width/5)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = png.Encode(&buf, scaled)
	return buf.Bytes(), err
}
