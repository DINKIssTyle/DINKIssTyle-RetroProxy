package proxy

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

// ImageFormat represents the output format for images
type ImageFormat string

const (
	ImageFormatOriginal ImageFormat = "original"
	ImageFormatJPEG     ImageFormat = "jpeg"
	ImageFormatGIF      ImageFormat = "gif"
	ImageFormatPNG8     ImageFormat = "png8"
)

// ImageFormatOption represents an image format option for the UI
type ImageFormatOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ImageConverter handles image format conversion
type ImageConverter struct {
	format ImageFormat
	client *http.Client
}

// NewImageConverter creates a new image converter
func NewImageConverter() *ImageConverter {
	return &ImageConverter{
		format: ImageFormatOriginal,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// GetAvailableFormats returns the available image format options
func (c *ImageConverter) GetAvailableFormats() []ImageFormatOption {
	return []ImageFormatOption{
		{Value: "original", Label: "Original (Pass-through)"},
		{Value: "jpeg", Label: "JPEG (Best compatibility)"},
		{Value: "gif", Label: "GIF (256 colors)"},
		{Value: "png8", Label: "PNG (8-bit)"},
	}
}

// SetFormat sets the output format
func (c *ImageConverter) SetFormat(format string) {
	c.format = ImageFormat(format)
}

// GetFormat returns the current format
func (c *ImageConverter) GetFormat() string {
	return string(c.format)
}

// FetchAndConvertImage fetches an image and converts it to the target format
func (c *ImageConverter) FetchAndConvertImage(targetURL string) ([]byte, string, error) {
	// Fetch the image using reused client
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	// Set Referer to bypass hotlink protection
	if u, err := url.Parse(targetURL); err == nil {
		origin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		req.Header.Set("Referer", origin)
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()

	// Read the image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	// If format is original, return as-is
	if c.format == ImageFormatOriginal {
		return imageData, contentType, nil
	}

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		// If we can't decode, return original
		return imageData, contentType, nil
	}

	// Convert to target format
	var buf bytes.Buffer
	var newContentType string

	switch c.format {
	case ImageFormatJPEG:
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75})
		newContentType = "image/jpeg"
	case ImageFormatGIF:
		// Handle GIF transparency manually
		// Create a paletted image with transparency support
		pm := convertToPalettedWithTransparency(img)
		// Encode with transparency index 0
		err = gif.Encode(&buf, pm, &gif.Options{NumColors: 256, Quantizer: nil, Drawer: nil})
		newContentType = "image/gif"
	case ImageFormatPNG8:
		// Handle PNG 8-bit with transparency
		// Reuse the same logic to create a Paletted image (ColorType 3 in PNG)
		pm := convertToPalettedWithTransparency(img)
		err = png.Encode(&buf, pm)
		newContentType = "image/png"
	default:
		return imageData, contentType, nil
	}

	if err != nil {
		// If conversion fails, return original
		return imageData, contentType, nil
	}

	return buf.Bytes(), newContentType, nil
}

// convertToPalettedWithTransparency converts an image to Paletted format preserving transparency.
// [DO NOT CHANGE THIS FUNCTION WITHOUT CAREFUL TESTING]
// Uses GIF round-trip for adaptive quantization + manual transparency enforcement.
func convertToPalettedWithTransparency(m image.Image) *image.Paletted {
	bounds := m.Bounds()

	// Step 1: Use GIF encode/decode round-trip for good adaptive palette
	var buf bytes.Buffer
	err := gif.Encode(&buf, m, &gif.Options{NumColors: 256})
	if err != nil {
		// Fallback to Plan9 if encoding fails
		return convertToPalettedPlan9(m)
	}

	img, err := gif.Decode(&buf)
	if err != nil {
		return convertToPalettedPlan9(m)
	}

	pm, ok := img.(*image.Paletted)
	if !ok {
		return convertToPalettedPlan9(m)
	}

	// Step 2: Enforce transparency at index 0
	// Save original color at index 0 so we can remap pixels that used it
	origColor0 := pm.Palette[0]

	// Create new palette with transparency at index 0
	newPalette := make(color.Palette, len(pm.Palette))
	copy(newPalette, pm.Palette)
	newPalette[0] = color.RGBA{0, 0, 0, 0} // Transparent

	// Create result image
	result := image.NewPaletted(bounds, newPalette)

	// Step 3: Re-map pixels from original image
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			origC := m.At(x, y)
			_, _, _, a := origC.RGBA()

			if a < 0x8000 {
				// Transparent pixel -> index 0
				result.SetColorIndex(x, y, 0)
			} else {
				// Opaque pixel -> use GIF-quantized result, but handle index 0 special case
				gifIdx := pm.ColorIndexAt(x, y)
				if gifIdx == 0 {
					// This pixel was mapped to index 0, but we changed index 0 to transparent.
					// Find the nearest color in the new palette (excluding index 0).
					bestIdx := uint8(1)
					bestDist := colorDistance(origC, newPalette[1])
					for i := 2; i < len(newPalette); i++ {
						d := colorDistance(origC, newPalette[i])
						if d < bestDist {
							bestDist = d
							bestIdx = uint8(i)
						}
					}
					// Or if the original color at index 0 is close enough, we can check
					// Actually, let's add the original color0 back to palette if possible
					// For simplicity, just use the best match found
					_ = origColor0 // unused but kept for potential future use
					result.SetColorIndex(x, y, bestIdx)
				} else {
					result.SetColorIndex(x, y, gifIdx)
				}
			}
		}
	}

	return result
}

// colorDistance calculates squared distance between two colors
func colorDistance(c1, c2 color.Color) int {
	r1, g1, b1, _ := c1.RGBA()
	r2, g2, b2, _ := c2.RGBA()
	dr := int(r1>>8) - int(r2>>8)
	dg := int(g1>>8) - int(g2>>8)
	db := int(b1>>8) - int(b2>>8)
	return dr*dr + dg*dg + db*db
}

// convertToPalettedPlan9 is the fallback using fixed palette
func convertToPalettedPlan9(m image.Image) *image.Paletted {
	var pal color.Palette = make([]color.Color, len(palette.Plan9))
	copy(pal, palette.Plan9)
	pal[0] = color.RGBA{0, 0, 0, 0}

	bounds := m.Bounds()
	pm := image.NewPaletted(bounds, pal)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := m.At(x, y)
			_, _, _, a := c.RGBA()
			if a < 0x8000 {
				pm.SetColorIndex(x, y, 0)
			} else {
				pm.Set(x, y, c)
			}
		}
	}
	return pm
}

// ShouldConvert checks if the URL looks like an image that should be converted
func (c *ImageConverter) ShouldConvert(url string) bool {
	if c.format == ImageFormatOriginal {
		return false
	}

	lowerURL := strings.ToLower(url)
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"}
	for _, ext := range imageExts {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}
	return false
}
