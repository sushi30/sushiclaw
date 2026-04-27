package email

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	gomail "github.com/emersion/go-message/mail"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/media"
)

type emailMediaHandler struct {
	store media.MediaStore
}

func newEmailMediaHandler(store media.MediaStore) emailMediaHandler {
	return emailMediaHandler{store: store}
}

func (h emailMediaHandler) outboundAttachments(parts []bus.MediaPart) ([]attachmentInfo, error) {
	if h.store == nil {
		return nil, fmt.Errorf("no media store available: %w", channels.ErrSendFailed)
	}

	attachments := make([]attachmentInfo, 0, len(parts))
	for _, part := range parts {
		localPath, err := h.store.Resolve(part.Ref)
		if err != nil {
			return nil, fmt.Errorf("resolve media ref %q: %w: %w", part.Ref, err, channels.ErrSendFailed)
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w: %w", localPath, err, channels.ErrSendFailed)
		}
		ct := part.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		filename := part.Filename
		if filename == "" {
			filename = filepath.Base(localPath)
		}
		attachments = append(attachments, attachmentInfo{
			filename:    filename,
			contentType: ct,
			data:        data,
		})
	}

	return attachments, nil
}

func (h emailMediaHandler) storeInboundAttachments(fromAddr, messageID string, attachments []attachmentInfo) []string {
	if h.store == nil || len(attachments) == 0 {
		return nil
	}

	if err := os.MkdirAll(media.TempDir(), 0o700); err != nil {
		logger.WarnCF("email", "Failed to create media temp dir", map[string]any{
			"err": err.Error(),
			"dir": media.TempDir(),
		})
		return nil
	}

	scope := channels.BuildMediaScope("email", fromAddr, messageID)
	mediaPaths := make([]string, 0, len(attachments))
	for _, att := range attachments {
		localPath := filepath.Join(media.TempDir(), uuid.New().String()[:8]+"_"+att.filename)
		if err := os.WriteFile(localPath, att.data, 0o600); err != nil {
			logger.WarnCF("email", "Failed to write attachment", map[string]any{
				"err":      err.Error(),
				"filename": att.filename,
			})
			continue
		}
		ref, storeErr := h.store.Store(localPath, media.MediaMeta{
			Filename:      att.filename,
			ContentType:   att.contentType,
			Source:        "email",
			CleanupPolicy: media.CleanupPolicyDeleteOnCleanup,
		}, scope)
		if storeErr != nil {
			logger.WarnCF("email", "Failed to store attachment", map[string]any{
				"err":      storeErr.Error(),
				"filename": att.filename,
			})
			continue
		}
		mediaPaths = append(mediaPaths, ref)
	}

	return mediaPaths
}

// attachmentInfo holds a parsed email attachment before storage.
type attachmentInfo struct {
	filename    string
	contentType string
	data        []byte
}

// extractAttachments walks the MIME message and returns all attachment parts.
func extractAttachments(r io.Reader) []attachmentInfo {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil
	}

	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}

	var attachments []attachmentInfo

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		attachHeader, ok := p.Header.(*gomail.AttachmentHeader)
		if !ok {
			continue
		}
		filename, _ := attachHeader.Filename()
		if filename == "" {
			ct, _, _ := attachHeader.ContentType()
			filename = fallbackFilename(ct)
		}
		data, _ := io.ReadAll(p.Body)
		if len(data) == 0 {
			continue
		}
		ct, _, _ := attachHeader.ContentType()
		if ct == "" {
			ct = "application/octet-stream"
		}
		attachments = append(attachments, attachmentInfo{
			filename:    filename,
			contentType: ct,
			data:        data,
		})
	}

	return attachments
}

// fallbackFilename generates a filename from a MIME type when none is provided.
func fallbackFilename(ct string) string {
	ext := "bin"
	switch ct {
	case "text/plain":
		ext = "txt"
	case "text/html":
		ext = "html"
	case "image/jpeg", "image/jpg":
		ext = "jpg"
	case "image/png":
		ext = "png"
	case "image/gif":
		ext = "gif"
	case "image/webp":
		ext = "webp"
	case "application/pdf":
		ext = "pdf"
	case "application/zip":
		ext = "zip"
	case "application/json":
		ext = "json"
	}
	return fmt.Sprintf("attachment.%s", ext)
}
