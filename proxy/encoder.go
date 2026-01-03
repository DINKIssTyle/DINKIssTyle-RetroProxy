package proxy

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// EncodingOption represents an encoding option for the UI
type EncodingOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Encoder handles character encoding conversion for legacy browsers
type Encoder struct {
	forcedEncoding string // If set, use this encoding instead of auto-detect
}

// NewEncoder creates a new encoder
func NewEncoder() *Encoder {
	return &Encoder{
		forcedEncoding: "", // Empty means auto-detect
	}
}

// SetForcedEncoding sets a forced encoding (empty string for auto-detect)
func (e *Encoder) SetForcedEncoding(encoding string) {
	e.forcedEncoding = encoding
}

// GetForcedEncoding returns the current forced encoding
func (e *Encoder) GetForcedEncoding() string {
	return e.forcedEncoding
}

// GetAvailableEncodings returns the list of available encodings for the UI
func (e *Encoder) GetAvailableEncodings() []EncodingOption {
	return []EncodingOption{
		{Value: "auto", Label: "Auto Detect"},
		{Value: "utf-8", Label: "UTF-8 (Unicode)"},
		{Value: "euc-kr", Label: "Korean (EUC-KR)"},
		{Value: "iso-8859-1", Label: "Western (ISO-8859-1)"},
		{Value: "iso-8859-2", Label: "Central European (ISO-8859-2)"},
		{Value: "windows-1252", Label: "Western (Windows-1252)"},
		{Value: "shift_jis", Label: "Japanese (Shift_JIS)"},
		{Value: "euc-jp", Label: "Japanese (EUC-JP)"},
		{Value: "gb2312", Label: "Chinese Simplified (GB2312)"},
		{Value: "gbk", Label: "Chinese Simplified (GBK)"},
		{Value: "big5", Label: "Chinese Traditional (Big5)"},
		{Value: "iso-8859-7", Label: "Greek (ISO-8859-7)"},
		{Value: "iso-8859-9", Label: "Turkish (ISO-8859-9)"},
		{Value: "koi8-r", Label: "Cyrillic (KOI8-R)"},
		{Value: "windows-1251", Label: "Cyrillic (Windows-1251)"},
	}
}

// LegacyBrowserInfo contains detected browser information
type LegacyBrowserInfo struct {
	IsLegacy bool
	Encoding string
	Name     string
}

// DetectLegacyBrowser detects if the User-Agent belongs to a legacy browser
// If forced encoding is set, returns that instead
func (e *Encoder) DetectLegacyBrowser(userAgent string) LegacyBrowserInfo {
	// If forced encoding is set, use it
	if e.forcedEncoding != "" && e.forcedEncoding != "auto" {
		return LegacyBrowserInfo{
			IsLegacy: e.forcedEncoding != "utf-8",
			Encoding: e.forcedEncoding,
			Name:     "Manual",
		}
	}

	ua := strings.ToLower(userAgent)

	// Netscape Navigator 2-4
	if strings.Contains(ua, "mozilla/") && !strings.Contains(ua, "gecko") && !strings.Contains(ua, "msie") && !strings.Contains(ua, "opera") {
		if strings.Contains(ua, "mozilla/2") || strings.Contains(ua, "mozilla/3") {
			return LegacyBrowserInfo{IsLegacy: true, Encoding: "euc-kr", Name: "Netscape 2-3"}
		}
		if strings.Contains(ua, "mozilla/4") {
			// NC4 supports UTF-8, but we name it for UI logic
			return LegacyBrowserInfo{IsLegacy: false, Encoding: "utf-8", Name: "Netscape 4"}
		}
	}

	// Mosaic
	if strings.Contains(ua, "mosaic") || strings.Contains(ua, "ncsa mosaic") {
		return LegacyBrowserInfo{IsLegacy: true, Encoding: "euc-kr", Name: "Mosaic"}
	}

	// Internet Explorer 1, 3, 4 (IE5+ supports UTF-8)
	if strings.Contains(ua, "msie 1") || strings.Contains(ua, "msie 3") || strings.Contains(ua, "msie 4") {
		return LegacyBrowserInfo{IsLegacy: true, Encoding: "euc-kr", Name: "Internet Explorer 1-4"}
	}

	// Opera 3-6 (Opera 7+ has better UTF-8)
	if strings.Contains(ua, "opera/3") || strings.Contains(ua, "opera/4") ||
		strings.Contains(ua, "opera/5") || strings.Contains(ua, "opera/6") {
		return LegacyBrowserInfo{IsLegacy: true, Encoding: "euc-kr", Name: "Opera 3-6"}
	}

	// Default: modern browser, use UTF-8
	return LegacyBrowserInfo{IsLegacy: false, Encoding: "utf-8", Name: "Modern"}
}

// ConvertToEncoding converts UTF-8 HTML to the specified encoding
func (e *Encoder) ConvertToEncoding(html string, encodingName string) ([]byte, error) {
	enc := e.getEncoder(encodingName)
	if enc == nil {
		return []byte(html), nil
	}

	// Update charset meta tag
	html = e.updateCharsetMeta(html, encodingName)

	// Convert encoding with best effort (replace invalid chars with ?)
	encoder := enc.NewEncoder()

	// Pre-allocate buffer (estimate size)
	var buf bytes.Buffer
	buf.Grow(len(html) + 512)

	src := []byte(html)
	srcIdx := 0
	dst := make([]byte, 4096)

	for srcIdx < len(src) {
		nDst, nSrc, err := encoder.Transform(dst, src[srcIdx:], true)
		buf.Write(dst[:nDst])
		srcIdx += nSrc

		if err == nil {
			break
		}

		if err == transform.ErrShortDst {
			// Output buffer too small, writing handled above, just continue
			continue
		}

		if err == transform.ErrShortSrc {
			// Input incomplete
			break
		}

		// If error is unrelated to buffer size, it's likely an invalid/unsupported char
		// Skip one rune and write '?'
		// We need to decode the rune to know how many bytes to skip
		// (Assume input is UTF-8 as per function contract)
		_, size := utf8.DecodeRune(src[srcIdx:])
		if size == 0 {
			break // Should not happen
		}

		// Skip the problematic rune and insert placeholder
		srcIdx += size
		buf.WriteByte('?') // Replacement char
	}

	return buf.Bytes(), nil
}

// getEncoder returns the encoding for the given name
func (e *Encoder) getEncoder(encodingName string) encoding.Encoding {
	switch strings.ToLower(encodingName) {
	case "utf-8":
		return nil // No conversion needed
	case "euc-kr":
		return korean.EUCKR
	case "iso-8859-1":
		return charmap.ISO8859_1
	case "iso-8859-2":
		return charmap.ISO8859_2
	case "windows-1252":
		return charmap.Windows1252
	case "shift_jis":
		return japanese.ShiftJIS
	case "euc-jp":
		return japanese.EUCJP
	case "gb2312", "gbk":
		return simplifiedchinese.GBK
	case "big5":
		return traditionalchinese.Big5
	case "iso-8859-7":
		return charmap.ISO8859_7
	case "iso-8859-9":
		return charmap.ISO8859_9
	case "koi8-r":
		return charmap.KOI8R
	case "windows-1251":
		return charmap.Windows1251
	default:
		return nil
	}
}

// updateCharsetMeta updates the charset meta tag in HTML
func (e *Encoder) updateCharsetMeta(html string, charset string) string {
	// Remove existing charset meta tags
	charsetRegex := regexp.MustCompile(`(?i)<meta[^>]*charset[^>]*>`)
	html = charsetRegex.ReplaceAllString(html, "")

	// Remove content-type meta tags
	contentTypeRegex := regexp.MustCompile(`(?i)<meta[^>]*http-equiv\s*=\s*["']?content-type["']?[^>]*>`)
	html = contentTypeRegex.ReplaceAllString(html, "")

	// Find the head tag and insert new charset
	headRegex := regexp.MustCompile(`(?i)(<head[^>]*>)`)
	newMeta := `$1<meta http-equiv="Content-Type" content="text/html; charset=` + charset + `">`
	html = headRegex.ReplaceAllString(html, newMeta)

	return html
}
