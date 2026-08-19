package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FSStore is a filesystem-backed Store: one immutable object per
// content-addressed id under root, verified on read. It is the Go-native port
// of DSH's filesystem attachment backend ("content-addressed objects").
type FSStore struct {
	root   string
	limits Limits
}

// NewFSStore creates a store rooted at dir with [DefaultLimits].
func NewFSStore(root string) *FSStore {
	return &FSStore{root: root, limits: DefaultLimits()}
}

// NewFSStoreWithLimits creates a store rooted at dir with an explicit policy.
func NewFSStoreWithLimits(root string, limits Limits) *FSStore {
	return &FSStore{root: root, limits: limits}
}

// ImageLimits returns the deployment-resolved image policy.
func (s *FSStore) ImageLimits() Limits { return s.limits }

// ValidateImage validates one image without persisting it.
func (s *FSStore) ValidateImage(_ context.Context, input SaveImage) error {
	return validateImage(s.limits, input)
}

// SaveImages validates the whole ordered batch before committing any member,
// then returns durable references in input order. Validation failures start
// no writes; storage failures return no partial references.
func (s *FSStore) SaveImages(ctx context.Context, inputs []SaveImage) ([]Ref, error) {
	if len(inputs) > s.limits.MaxImagesPerMessage {
		return nil, &Error{Code: CodeTooManyImages, Msg: fmt.Sprintf("image batch of %d exceeds the configured count limit of %d", len(inputs), s.limits.MaxImagesPerMessage)}
	}
	total := 0
	for _, in := range inputs {
		total += len(in.Data)
	}
	if total > s.limits.MaxMessageImageBytes {
		return nil, &Error{Code: CodeImagesTooLarge, Msg: fmt.Sprintf("image batch of %d bytes exceeds the configured aggregate limit of %d", total, s.limits.MaxMessageImageBytes)}
	}
	// Validate every member before saving any member.
	for _, in := range inputs {
		if err := s.ValidateImage(ctx, in); err != nil {
			return nil, err
		}
	}
	refs := make([]Ref, 0, len(inputs))
	for _, in := range inputs {
		ref, err := s.saveImage(ctx, in)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// SaveImage validates and durably commits one image.
func (s *FSStore) SaveImage(ctx context.Context, input SaveImage) (Ref, error) {
	if err := s.ValidateImage(ctx, input); err != nil {
		return Ref{}, err
	}
	return s.saveImage(ctx, input)
}

func (s *FSStore) saveImage(_ context.Context, input SaveImage) (Ref, error) {
	id := contentID(input.Data)
	path := filepath.Join(s.root, string(id))

	// Content-addressed objects are immutable: duplicate writes are rejected.
	if _, err := os.Stat(path); err == nil {
		return Ref{}, &Error{Code: CodeWriteFailed, Msg: "duplicate attachment write rejected (object already exists)"}
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return Ref{}, &Error{Code: CodeWriteFailed, Msg: "cannot create attachment root", Err: err}
	}
	if err := os.WriteFile(path, input.Data, 0o644); err != nil {
		return Ref{}, &Error{Code: CodeWriteFailed, Msg: "cannot persist attachment", Err: err}
	}
	width, height, _, err := decodeInfo(input.Data)
	if err != nil {
		// Validated above; unreachable unless the file was mutated underneath.
		return Ref{}, &Error{Code: CodeCorrupt, Msg: "validated image no longer decodes", Err: err}
	}
	return Ref{
		AttachmentID: id,
		MediaType:    input.MediaType,
		Bytes:        len(input.Data),
		Width:        width,
		Height:       height,
		Name:         input.Name,
	}, nil
}

// ReadImage reads one image and verifies the bytes still match the recorded
// reference: digest, byte length, and decodability/media type agreement.
func (s *FSStore) ReadImage(_ context.Context, ref Ref) (Stored, error) {
	if err := validateRef(ref); err != nil {
		return Stored{}, err
	}
	path := filepath.Join(s.root, string(ref.AttachmentID))
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Stored{}, &Error{Code: CodeNotFound, Msg: "attachment object not found", Err: err}
		}
		return Stored{}, &Error{Code: CodeReadFailed, Msg: "cannot read attachment object", Err: err}
	}
	if contentID(data) != ref.AttachmentID {
		return Stored{}, &Error{Code: CodeCorrupt, Msg: "attachment digest no longer matches its reference"}
	}
	if len(data) != ref.Bytes {
		return Stored{}, &Error{Code: CodeCorrupt, Msg: fmt.Sprintf("attachment byte length %d no longer matches reference %d", len(data), ref.Bytes)}
	}
	if err := validateImage(s.limits, SaveImage{Data: data, MediaType: ref.MediaType}); err != nil {
		return Stored{}, &Error{Code: CodeCorrupt, Msg: "attachment media type no longer matches its reference", Err: err}
	}
	return Stored{Ref: ref, Data: data}, nil
}

// validateRef rejects structurally invalid references before any I/O.
func validateRef(ref Ref) error {
	if ref.AttachmentID == "" {
		return &Error{Code: CodeInvalidRef, Msg: "attachment id is empty"}
	}
	if formatFor(ref.MediaType) == "" {
		return &Error{Code: CodeInvalidRef, Msg: fmt.Sprintf("attachment media type %q is not accepted", ref.MediaType)}
	}
	if ref.Bytes <= 0 {
		return &Error{Code: CodeInvalidRef, Msg: "attachment byte length is not positive"}
	}
	return nil
}

// contentID returns the opaque content-addressed id for one immutable object:
// hex-encoded sha256 of the encoded bytes.
func contentID(data []byte) ID {
	sum := sha256.Sum256(data)
	return ID(hex.EncodeToString(sum[:]))
}
