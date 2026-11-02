// Package embedded provides embedded fallback provider manifests.
package embedded

import "embed"

// ManifestsFS embeds the default provider manifests for zero-configuration out-of-the-box operation.
//
//go:embed manifests/*.json
var ManifestsFS embed.FS
