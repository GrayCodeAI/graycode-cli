// Package attachment is a Go-native port of DSH's durable attachment seam
// (`attachment/attachment`, dsh-v0.1.0-rc.7) version-one image path. It owns
// the vocabulary for immutable image objects: opaque content-addressed ids,
// media types verified from decoded bytes, admission limits, and a store
// interface whose implementations validate bytes before publishing a
// reference.
package attachment

import (
	"context"
	"errors"
	"fmt"
)

// ID is an opaque content-addressed identifier for one immutable attachment
// object. It is never a filesystem path or bearer URL (DSH `AttachmentId`).
type ID string

// ImageMediaType is a raster image format accepted by the version-one path.
type ImageMediaType string

const (
	MediaTypePNG  ImageMediaType = "image/png"
	MediaTypeJPEG ImageMediaType = "image/jpeg"
	MediaTypeWebP ImageMediaType = "image/webp"
	MediaTypeGIF  ImageMediaType = "image/gif"
)

// AllMediaTypes lists every accepted raster format, in DSH order.
var AllMediaTypes = []ImageMediaType{MediaTypePNG, MediaTypeJPEG, MediaTypeWebP, MediaTypeGIF}

// Ref is durable, serializable metadata for one immutable image object.
type Ref struct {
	// Opaque storage identifier; never a filesystem path or bearer URL.
	AttachmentID ID `json:"attachmentId"`
	// Media type verified from the stored bytes.
	MediaType ImageMediaType `json:"mediaType"`
	// Exact encoded byte length.
	Bytes int `json:"bytes"`
	// Intrinsic encoded width in pixels.
	Width int `json:"width"`
	// Intrinsic encoded height in pixels.
	Height int `json:"height"`
	// Optional display name stripped of local path information.
	Name string `json:"name,omitempty"`
}

// Limits is the deployment-resolved image policy used by authoritative and
// fast-path validation.
type Limits struct {
	MaxImageBytes        int
	MaxImagesPerMessage  int
	MaxMessageImageBytes int
	MaxImagePixels       int
	MediaTypes           []ImageMediaType
}

// DefaultLimits returns a conservative deployment policy: 10 MiB per image,
// 4 images per message, 20 MiB aggregate, 16 megapixels per image, all four
// raster formats.
func DefaultLimits() Limits {
	return Limits{
		MaxImageBytes:        10 << 20,
		MaxImagesPerMessage:  4,
		MaxMessageImageBytes: 20 << 20,
		MaxImagePixels:       16_000_000,
		MediaTypes:           append([]ImageMediaType(nil), AllMediaTypes...),
	}
}

// SaveImage is a request to validate and durably commit one image.
type SaveImage struct {
	// Encoded bytes.
	Data []byte
	// Caller-declared media type, checked against fully decoded bytes.
	MediaType ImageMediaType
	// Optional display name; never interpreted as a path.
	Name string
}

// Stored is the stored image bytes returned after reference and digest
// verification.
type Stored struct {
	Ref  Ref
	Data []byte
}

// Store is the immutable binary attachment service. Implementations validate
// bytes before publishing a reference (DSH `AttachmentStore`).
type Store interface {
	// ImageLimits returns the deployment-resolved image policy.
	ImageLimits() Limits

	// ValidateImage validates one image without persisting it. Batch callers
	// validate every member before saving any member.
	ValidateImage(ctx context.Context, input SaveImage) error

	// SaveImages validates one ordered image batch before committing any
	// member, then returns durable references in the exact input order.
	// Validation failures start no writes; storage failures return no partial
	// references.
	SaveImages(ctx context.Context, inputs []SaveImage) ([]Ref, error)

	// SaveImage validates and durably commits one image before its owning
	// session event is appended.
	SaveImage(ctx context.Context, input SaveImage) (Ref, error)

	// ReadImage reads one image and verifies the bytes still match the
	// recorded reference.
	ReadImage(ctx context.Context, ref Ref) (Stored, error)
}

// Code is a stable machine-routing attachment failure code (DSH
// `AttachmentErrorCode`).
type Code string

const (
	CodeTooManyImages      Code = "TOO_MANY_IMAGES"
	CodeImagesTooLarge     Code = "IMAGES_TOO_LARGE"
	CodeUnsupportedType    Code = "UNSUPPORTED_IMAGE_TYPE"
	CodeInvalidBase64      Code = "INVALID_IMAGE_BASE64"
	CodeInvalidImage       Code = "INVALID_IMAGE"
	CodeImageTypeMismatch  Code = "IMAGE_TYPE_MISMATCH"
	CodeImageTooLarge      Code = "IMAGE_TOO_LARGE"
	CodeImageTooManyPixels Code = "IMAGE_TOO_MANY_PIXELS"
	CodeInvalidRef         Code = "INVALID_ATTACHMENT_REF"
	CodeCorrupt            Code = "ATTACHMENT_CORRUPT"
	CodeWriteFailed        Code = "ATTACHMENT_WRITE_FAILED"
	CodeNotFound           Code = "ATTACHMENT_NOT_FOUND"
	CodeReadFailed         Code = "ATTACHMENT_READ_FAILED"
)

// admissionCodes are the caller-correctable failures raised while admitting
// image input (DSH `ImageAdmissionErrorCode`).
var admissionCodes = map[Code]bool{
	CodeTooManyImages:      true,
	CodeImagesTooLarge:     true,
	CodeUnsupportedType:    true,
	CodeInvalidBase64:      true,
	CodeInvalidImage:       true,
	CodeImageTypeMismatch:  true,
	CodeImageTooLarge:      true,
	CodeImageTooManyPixels: true,
}

// Error is a stable attachment failure suitable for host RPC error mapping.
// Consumers route on Code, never on the prototype chain.
type Error struct {
	Code Code
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("attachment: %s: %s: %v", e.Code, e.Msg, e.Err)
	}
	return fmt.Sprintf("attachment: %s: %s", e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// IsAdmission reports whether err is a caller-correctable image admission
// failure rather than a storage fault (DSH `isImageAdmissionError`).
func IsAdmission(err error) bool {
	var ae *Error
	return errors.As(err, &ae) && admissionCodes[ae.Code]
}

// CodeOf extracts the stable routing code from err, or "" when it is not an
// attachment error.
func CodeOf(err error) Code {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}
