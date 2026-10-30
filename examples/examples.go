// Package examples holds the sample CV shown in the README. cvmaker init
// writes these files as the starting point for a new CV.
package examples

import _ "embed"

var (
	//go:embed cv.md
	CV []byte
	//go:embed config.yml
	Config []byte
)
