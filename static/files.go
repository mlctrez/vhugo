package static

import "embed"

//go:embed *.html *.js *.ico
var Files embed.FS
