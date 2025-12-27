package proxy

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	_ "image/png"
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
		err = gif.Encode(&buf, img, nil)
		newContentType = "image/gif"
	default:
		return imageData, contentType, nil
	}

	if err != nil {
		// If conversion fails, return original
		return imageData, contentType, nil
	}

	return buf.Bytes(), newContentType, nil
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
