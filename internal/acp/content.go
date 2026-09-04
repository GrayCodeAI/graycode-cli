// ACP wire-content admission and projection owned by the ACP adapter.
//
// Go-native port of DSH `packages/acp/acp/src/content.ts`
// (dsh-v0.1.0-rc.7): it narrows untrusted ACP prompt content blocks to the
// durable attachment vocabulary, strictly decodes inline images, admits an
// ordered batch to the attachment store, and projects committed assistant
// blocks back onto ACP wire content. Content-admission failures carry a
// stable category (`invalid` | `internal`) with no raw binary payload.
package acp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/attachment"
)

// Raster formats shared by ACP image blocks and the core attachment
// vocabulary (DSH `IMAGE_MEDIA_TYPES`).
var acpImageMediaTypes = []attachment.ImageMediaType{
	attachment.MediaTypePNG,
	attachment.MediaTypeJPEG,
	attachment.MediaTypeWebP,
	attachment.MediaTypeGIF,
}

// canonicalBase64 is RFC 4648 standard base64, excluding whitespace and
// URL-safe alphabet variants (DSH `CANONICAL_BASE64`).
var canonicalBase64 = regexp.MustCompile(`^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$`)

// ContentFailureKind is the content-admission failure category used by the
// protocol handler (DSH `AcpContentFailureKind`).
type ContentFailureKind string

const (
	// FailureInvalid reports caller-correctable request content, mapped to
	// ACP invalid-params.
	FailureInvalid ContentFailureKind = "invalid"
	// FailureInternal reports an adapter/storage fault, mapped to ACP
	// internal-error.
	FailureInternal ContentFailureKind = "internal"
)

// ContentError is an ACP content-admission failure with a stable category and
// no inline raw binary payload (DSH `AcpContentError`).
type ContentError struct {
	Kind ContentFailureKind
	Msg  string
	Err  error
}

func (e *ContentError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("acp content: %s: %s: %v", e.Kind, e.Msg, e.Err)
	}
	return fmt.Sprintf("acp content: %s: %s", e.Kind, e.Msg)
}

func (e *ContentError) Unwrap() error { return e.Err }

// AcpContentBlock is one ACP wire content block (the union subset this
// adapter admits). Includes the ACP text/image/resource_link shapes plus the
// audio/resource tags that are explicitly rejected.
type AcpContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	Name     string `json:"name,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// ContentBlock is one ordered piece of durable core content admitted from an
// ACP prompt or projected from a committed assistant block. Images carry the
// store-verified attachment reference (DSH `ContentBlock`).
type ContentBlock struct {
	Type       string
	Text       string
	Attachment *attachment.Ref
}

// imageMediaType narrows a wire MIME string to the durable raster vocabulary.
func imageMediaType(value string) attachment.ImageMediaType {
	for _, mt := range acpImageMediaTypes {
		if string(mt) == value {
			return mt
		}
	}
	return ""
}

// decodeImage strictly decodes one ACP inline image without accepting base64
// aliases (canonical RFC 4648, media type narrowed to the raster vocabulary).
func decodeImage(block AcpContentBlock) (attachment.SaveImage, error) {
	mediaType := imageMediaType(block.MimeType)
	if mediaType == "" {
		return attachment.SaveImage{}, &ContentError{
			Kind: FailureInvalid,
			Msg:  "image mimeType must be image/png, image/jpeg, image/webp, or image/gif",
		}
	}
	if !canonicalBase64.MatchString(block.Data) {
		return attachment.SaveImage{}, &ContentError{
			Kind: FailureInvalid,
			Msg:  "image data must be canonical base64",
		}
	}
	data, err := base64.StdEncoding.DecodeString(block.Data)
	if err != nil {
		return attachment.SaveImage{}, &ContentError{
			Kind: FailureInvalid,
			Msg:  "image data must be canonical base64",
		}
	}
	// Re-encode equality rejects non-canonical encodings the regexp allows
	// (e.g. wrong padding width) — exactly DSH's double check.
	if base64.StdEncoding.EncodeToString(data) != block.Data {
		return attachment.SaveImage{}, &ContentError{
			Kind: FailureInvalid,
			Msg:  "image data must be canonical base64",
		}
	}
	return attachment.SaveImage{Data: data, MediaType: mediaType}, nil
}

// resourceLinkText renders one baseline resource link into the current text
// vocabulary.
func resourceLinkText(block AcpContentBlock) string {
	name, _ := json.Marshal(block.Name)
	uri, _ := json.Marshal(block.URI)
	return fmt.Sprintf("\n[resource_link name=%s uri=%s]\n", name, uri)
}

// AdmitAcpPrompt validates and admits one ACP prompt into ordered durable
// core content. Every wire block and image is validated before the ordered
// image batch starts writing; cancellation after a successful content-addressed
// write may leave an unreachable object but never queues a late user message.
//
// store may be nil only when the prompt contains no image blocks; any image
// admission requires a mounted attachment store. The caller appends the
// returned content to its session spine only after admission returns, so a
// successful content-addressed write never races a user message.
func AdmitAcpPrompt(
	ctx context.Context,
	store attachment.Store,
	prompt []AcpContentBlock,
	imageEnabled bool,
	signal <-chan struct{},
) ([]ContentBlock, error) {
	images := make([]attachment.SaveImage, 0, len(prompt))
	for _, block := range prompt {
		switch block.Type {
		case "text", "resource_link":
			// validated during reconstruction below.
		case "image":
			if !imageEnabled {
				return nil, &ContentError{
					Kind: FailureInvalid,
					Msg:  "inline image prompts were not advertised by this connection",
				}
			}
			img, err := decodeImage(block)
			if err != nil {
				return nil, err
			}
			images = append(images, img)
		case "audio":
			return nil, &ContentError{Kind: FailureInvalid, Msg: "audio prompt content is not supported"}
		case "resource":
			return nil, &ContentError{Kind: FailureInvalid, Msg: "embedded resource prompt content is not supported"}
		default:
			return nil, &ContentError{Kind: FailureInvalid, Msg: "unsupported ACP prompt content"}
		}
	}

	refs := make([]attachment.Ref, 0, len(images))
	if len(images) > 0 {
		if store == nil {
			return nil, &ContentError{Kind: FailureInvalid, Msg: "no attachment store is mounted"}
		}
		if err := checkAborted(signal); err != nil {
			return nil, err
		}
		saved, err := store.SaveImages(ctx, images)
		if err != nil {
			if attachment.IsAdmission(err) {
				return nil, &ContentError{Kind: FailureInvalid, Msg: err.Error(), Err: err}
			}
			return nil, &ContentError{Kind: FailureInternal, Msg: "unable to persist the prompt image batch", Err: err}
		}
		refs = saved
		if err := checkAborted(signal); err != nil {
			return nil, err
		}
	}

	content := make([]ContentBlock, 0, len(prompt))
	pendingText := ""
	flushText := func() {
		if pendingText == "" {
			return
		}
		content = append(content, ContentBlock{Type: "text", Text: pendingText})
		pendingText = ""
	}
	imageIndex := 0
	for _, block := range prompt {
		switch block.Type {
		case "text":
			pendingText += block.Text
		case "resource_link":
			pendingText += resourceLinkText(block)
		case "image":
			flushText()
			ref := refs[imageIndex]
			imageIndex++
			block := ContentBlock{Type: "image", Attachment: &ref}
			content = append(content, block)
		case "audio", "resource":
			// validated-and-rejected by the first pass above; unreachable.
		}
	}
	flushText()

	nonEmpty := false
	for _, block := range content {
		if block.Type == "image" || (block.Type == "text" && strings.TrimSpace(block.Text) != "") {
			nonEmpty = true
			break
		}
	}
	if !nonEmpty {
		return nil, &ContentError{Kind: FailureInvalid, Msg: "empty prompt"}
	}
	return content, nil
}

func checkAborted(signal <-chan struct{}) error {
	if signal == nil {
		return nil
	}
	select {
	case <-signal:
		return &ContentError{Kind: FailureInternal, Msg: "prompt admission cancelled"}
	default:
		return nil
	}
}

// SupportsAcpImagePrompts reports whether a mounted attachment store can
// admit the ACP raster vocabulary (DSH `supportsAcpImagePrompts`'s storage
// half). Model-route capability is checked by the caller via the store-less
// model gate and conjoined here.
func SupportsAcpImagePrompts(store attachment.Store, modelSupportsImage bool) bool {
	if store == nil || !modelSupportsImage {
		return false
	}
	limits := store.ImageLimits()
	for _, mt := range limits.MediaTypes {
		if imageMediaType(string(mt)) != "" {
			return true
		}
	}
	return false
}

// AssistantBlockToAcp projects one committed assistant block to ACP wire
// content. Text blocks map directly; image attachments are re-read and
// integrity-verified before inline base64 delivery. Returns nil for
// non-output blocks and for empty text.
func AssistantBlockToAcp(ctx context.Context, store attachment.Store, block ContentBlock) (*AcpContentBlock, error) {
	if block.Type == "text" {
		if block.Text == "" {
			return nil, nil
		}
		return &AcpContentBlock{Type: "text", Text: block.Text}, nil
	}
	if block.Type != "image" || block.Attachment == nil {
		return nil, nil
	}
	if store == nil {
		return nil, &ContentError{Kind: FailureInternal, Msg: "cannot deliver assistant image: no attachment store is mounted"}
	}
	stored, err := store.ReadImage(ctx, *block.Attachment)
	if err != nil {
		return nil, &ContentError{
			Kind: FailureInternal,
			Msg:  "cannot deliver assistant image: the attachment is unavailable or corrupt",
			Err:  err,
		}
	}
	return &AcpContentBlock{
		Type:     "image",
		Data:     base64.StdEncoding.EncodeToString(stored.Data),
		MimeType: string(stored.Ref.MediaType),
	}, nil
}

// asContentError unwraps a ContentError from a wrapped error chain, for
// RPC-error-category mapping.
func asContentError(err error) (*ContentError, bool) {
	var ce *ContentError
	if errors.As(err, &ce) {
		return ce, true
	}
	return nil, false
}
