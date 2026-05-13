package handlers

import "embed"

// assets/logo.png is the optional report card logo.
// If the file doesn't exist at build time, logoPNG will be empty.
//
//go:embed assets/*
var assetsFS embed.FS

// LoadLogoPNG attempts to load the embedded logo. Returns nil if not present.
func LoadLogoPNG() []byte {
	data, err := assetsFS.ReadFile("assets/logo.png")
	if err != nil {
		return nil
	}
	return data
}
