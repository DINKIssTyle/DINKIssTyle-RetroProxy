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

// convertToPalettedWithTransparency converts an image to Paletted format preserving transparency
// convertToPalettedWithTransparency converts an image to Paletted format preserving transparency
// It uses GIF encoding to generate an optimized palette for better color quality.
func convertToPalettedWithTransparency(m image.Image) *image.Paletted {
	// If already paletted, return as is
	if pm, ok := m.(*image.Paletted); ok {
		return pm
	}

	// 1. Use GIF encoding to generate an optimized palette
	// This uses the standard library's quantizer which is better than fixed Plan9
	var buf bytes.Buffer
	err := gif.Encode(&buf, m, &gif.Options{NumColors: 256, Quantizer: nil, Drawer: nil})
	if err != nil {
		// Fallback to basic conversion if encoding fails
		return convertToPalettedPlan9(m)
	}

	// 2. Decode back to get the paletted image
	img, err := gif.Decode(&buf)
	if err != nil {
		return convertToPalettedPlan9(m)
	}

	pm, ok := img.(*image.Paletted)
	if !ok {
		return convertToPalettedPlan9(m)
	}

	return pm
}

// convertToPalettedPlan9 is a fallback using fixed Plan9 palette
func convertToPalettedPlan9(m image.Image) *image.Paletted {
	var pal color.Palette = make([]color.Color, len(palette.Plan9))
	copy(pal, palette.Plan9)
	pal[0] = color.RGBA{0, 0, 0, 0} // Reserve index 0 for transparency

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
