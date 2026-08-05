// Created by DINKIssTyle on 2025. Copyright (C) 2025 DINKI'ssTyle. All rights reserved.

package proxy

import "fmt"

// UIOption represents a generic dropdown option
type UIOption struct {
	Value string
	Label string
}

// AvailableHTMLModes lists all supported proxy modes
var AvailableHTMLModes = []UIOption{
	{"modern", "Modern (No SSL)"},
	{"3.2", "HTML 3.2 (Legacy)"},
	{"3.2new", "HTML 3.2 New (Layout)"},
	{"4.01", "HTML 4.01 (Standard)"},
	{"text", "Text Only"},
	{"image", "Image Map"},
}

// AvailableEncodings lists supported text encodings
var AvailableEncodings = []UIOption{
	{"auto", "Auto Detect"},
	{"utf-8", "UTF-8"},
	{"euc-kr", "EUC-KR"},
	{"cp949", "CP949"},
	{"shift_jis", "Shift_JIS"},
	{"iso-8859-1", "ISO-8859-1"},
}

// AvailableImageFormats lists supported image formats
var AvailableImageFormats = []UIOption{
	{"original", "Original (Pass-through)"},
	{"jpeg", "JPEG (Best compatibility)"},
	{"gif", "GIF (256 colors)"},
	{"png8", "PNG (8-bit)"},
}

// InternalPages lists built-in pages for the debug index
var InternalPages = []UIOption{
	{"/debug", "Debug Home (URL Launcher)"},
	{"/_drp/settings", "Server Settings"},
	//{"/_drp/images", "Image Cache Viewer (Internal)"},
	{"/_drp/test/retry", "Retry Page (Test)"},
}

// Helper to generate <option> tags
func GenerateOptionsHTML(options []UIOption, selectedValue string) string {
	html := ""
	for _, opt := range options {
		selected := ""
		if opt.Value == selectedValue {
			selected = " selected"
		}
		html += fmt.Sprintf(`<option value="%s"%s>%s</option>`, opt.Value, selected, opt.Label)
	}
	return html
}

// Helper to generate <li> links
func GenerateLinkListHTML(options []UIOption) string {
	html := "<ul>"
	for _, opt := range options {
		html += fmt.Sprintf(`<li><a href="%s">%s</a> <small>(%s)</small></li>`, opt.Value, opt.Label, opt.Value)
	}
	html += "</ul>"
	return html
}
