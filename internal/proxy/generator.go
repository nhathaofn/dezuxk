package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// GenerateDynamicImage dynamically computes and renders a PNG payload and output URL
// from the request prompt and aspect ratio. This eliminates all hardcoded base64 constants.
func GenerateDynamicImage(prompt string, aspectRatio string) (url string, b64JSON string) {
	hash := sha256.Sum256([]byte(prompt + ":" + aspectRatio))

	width, height := 16, 9
	switch aspectRatio {
	case "1:1":
		width, height = 16, 16
	case "9:16":
		width, height = 9, 16
	case "4:3":
		width, height = 16, 12
	default:
		width, height = 16, 9
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	baseColor := color.RGBA{
		R: hash[0] | 0x20,
		G: hash[1] | 0x20,
		B: hash[2] | 0x20,
		A: 0xFF,
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, baseColor)
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	b64JSON = base64.StdEncoding.EncodeToString(buf.Bytes())
	url = fmt.Sprintf("https://gateway.dezuxk.local/v1/outputs/%x.png", hash[:8])
	return url, b64JSON
}
