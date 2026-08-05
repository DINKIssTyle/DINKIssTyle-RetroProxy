// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package proxy

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
)

// Tile represents a sliced image part
type Tile struct {
	Data []byte
	Y    int // Y offset
	H    int // Height
}

// SliceImage takes image data (PNG or JPEG) and slices it into vertical tiles of max height
func SliceImage(imgData []byte, maxTileHeight int) ([]Tile, int, int, error) {
	// Decode Image (supports JPEG, PNG, etc. automatically)
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, 0, 0, err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	var tiles []Tile

	for y := 0; y < height; y += maxTileHeight {
		h := maxTileHeight
		if y+h > height {
			h = height - y
		}

		// Crop
		// SubImage shares memory, so it's fast.
		// However, we need to encode each tile to JPEG bytes.
		type subImager interface {
			SubImage(r image.Rectangle) image.Image
		}

		var subImg image.Image
		if si, ok := img.(subImager); ok {
			rect := image.Rect(0, y, width, y+h)
			subImg = si.SubImage(rect)
		} else {
			// Fallback (shouldn't happen with png decode)
			continue
		}

		// Encode to JPEG
		var buf bytes.Buffer
		// Quality 75 is decent for web
		if err := jpeg.Encode(&buf, subImg, &jpeg.Options{Quality: 75}); err != nil {
			continue
		}

		tiles = append(tiles, Tile{
			Data: buf.Bytes(),
			Y:    y,
			H:    h,
		})
	}

	return tiles, width, height, nil
}
