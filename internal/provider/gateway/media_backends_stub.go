//go:build !media_engine

package gateway

import graycoderouterengine "github.com/GrayCodeAI/graycode-router/engine"

// wireOptionalBackends is a no-op when the media_engine build tag is disabled.
// The full implementation (media_backends.go) is only compiled with
// -tags media_engine and requires a graycode-router checkout that exposes the
// GenerateImage/Transcribe engine facade (not present in the published v0.0.1).
func wireOptionalBackends(eng *graycoderouterengine.Engine) {}
