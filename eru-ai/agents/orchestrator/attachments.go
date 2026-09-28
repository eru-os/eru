package orchestrator

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"strings"

	models "github.com/eru-os/eru/eru-ai/models"
	"github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"golang.org/x/image/draw"
)

// An attachment arrives inline and must not travel that way.
//
// The browser sends a screenshot as base64 in the request body, which is the
// right thing for it to do and the wrong thing for everything downstream. The
// planner does not need two megabytes to decide who should look at a picture;
// it needs to know that a picture was attached and roughly what it is of. A
// step request certainly does not need the bytes - a plan that carried them
// would be emitting them as model output, token by token.
//
// So the bytes go to the file store once, on arrival, and what travels
// afterwards is a descriptor. The planner sees a small copy of an image, enough
// to tell a bug screenshot from a logo. The step that has to look at the file is
// handed the descriptor and fetches what it needs.
//
// Which step that is, is the planner's decision, and the only one it has to
// make: every agent decodes the request into the same message shape, so "files"
// is accepted by all of them beside "content". There is nothing for an agent to
// opt into and nothing for the orchestrator to guess at - only the question of
// which step actually needs to look at the thing.

// planningPreviewPixels is the longest side of the copy the planner sees.
//
// It is for recognition, not for reading: enough to tell a chart from a form
// from a photograph, and a small fraction of the original.
const planningPreviewPixels = 512

// planningAttachments is what the planner is shown of the user's files.
func planningAttachments(ctx context.Context, files []models.FileMessage) []models.FileMessage {
	if len(files) == 0 {
		return nil
	}
	out := make([]models.FileMessage, 0, len(files))
	for _, file := range files {
		if file.ImageData == "" {
			// A document's body is not something the planner reads. It knows the
			// file exists and can route it; the agent that opens it reads it.
			file.FileData = ""
			out = append(out, file)
			continue
		}
		preview, previewType, err := downscaleImage(file.ImageData, planningPreviewPixels)
		if err != nil {
			if !errors.Is(err, errAlreadySmall) {
				logs.WithContext(ctx).Info(fmt.Sprintf("could not downscale %s for planning, sending it whole: %v", file.FileName, err))
			}
			out = append(out, file)
			continue
		}
		file.ImageData = preview
		file.FileType = previewType
		out = append(out, file)
	}
	return out
}

// errAlreadySmall says the image needs no scaling, which is not a failure.
var errAlreadySmall = errors.New("image is already within the preview size")

// downscaleImage returns a base64 copy whose longest side is at most max.
func downscaleImage(encoded string, max int) (string, string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("not base64: %w", err)
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", "", fmt.Errorf("not a readable image: %w", err)
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= max && h <= max {
		return "", "", errAlreadySmall
	}
	if w >= h {
		h = h * max / w
		w = max
	} else {
		w = w * max / h
		h = max
	}
	if w < 1 || h < 1 {
		return "", "", fmt.Errorf("image is too small to scale")
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return "", "", err
	}

	// Fewer pixels is not always fewer bytes. A flat or synthetic image can
	// compress far better as the PNG it arrived as than as a JPEG of a quarter
	// the size, and sending the larger of the two would be a straight loss on
	// both counts.
	preview := base64.StdEncoding.EncodeToString(buf.Bytes())
	if len(preview) >= len(encoded) {
		return "", "", errAlreadySmall
	}
	return preview, "image/jpeg", nil
}

// offloadAttachments moves inline payloads into the file store and leaves
// descriptors in their place, so what travels onward is an id.
//
// It degrades rather than fails. A tenant with no file store attached keeps
// today's behaviour - the payload travels inline - because losing the
// attachment altogether would be worse than carrying it.
func (oa *OrchestratorAgent) offloadAttachments(ctx context.Context, files []models.FileMessage, projectId, tenantId string) []models.FileMessage {
	if len(files) == 0 {
		return files
	}
	store := oa.fileStore(ctx)
	if store == nil {
		logs.WithContext(ctx).Info("no file store is attached to this orchestrator - attachments travel inline")
		return files
	}

	out := make([]models.FileMessage, 0, len(files))
	for _, file := range files {
		payload := file.ImageData
		if payload == "" {
			payload = file.FileData
		}
		if payload == "" || file.FileId != "" {
			// Nothing to move, or it is already in the store.
			out = append(out, file)
			continue
		}
		id, err := uploadAttachment(ctx, store, file, payload, projectId, tenantId)
		if err != nil || id == "" {
			logs.WithContext(ctx).Info(fmt.Sprintf("could not store %s, it travels inline: %v", file.FileName, err))
			out = append(out, file)
			continue
		}
		file.FileId = id
		file.FileData = ""

		// An image keeps a thumbnail in place of its payload. The full
		// resolution is in the store for whoever needs to read it; everything
		// that merely needs to SEE what was attached - the planner deciding who
		// should handle it, a later turn recalling it - is better served by a
		// picture it can afford than by an id it cannot look at.
		if file.ImageData != "" {
			preview, previewType, scaleErr := downscaleImage(file.ImageData, planningPreviewPixels)
			switch {
			case scaleErr == nil:
				file.ImageData = preview
				file.FileType = previewType
			case errors.Is(scaleErr, errAlreadySmall):
				// It is its own thumbnail.
			default:
				file.ImageData = ""
			}
		}
		logs.WithContext(ctx).Info(fmt.Sprintf("attachment %s stored as %s", file.FileName, id))
		out = append(out, file)
	}
	return out
}

// fileStore finds an attached eru-files tool to put attachments in.
func (oa *OrchestratorAgent) fileStore(ctx context.Context) tools.Tooling {
	for _, attached := range oa.AgentTools {
		if attached.Tool == nil {
			continue
		}
		toolType, err := attached.Tool.GetAttribute(ctx, "tool_type")
		if err != nil {
			continue
		}
		if name, ok := toolType.(string); ok && strings.EqualFold(name, "ERUFILES") {
			return attached.Tool
		}
	}
	return nil
}

// attachmentSummary names what the user attached, for a planner that is given
// descriptors rather than contents.
func attachmentSummary(files []models.FileMessage) string {
	if len(files) == 0 {
		return ""
	}
	lines := make([]string, 0, len(files))
	for i, file := range files {
		name := file.FileName
		if name == "" {
			name = fmt.Sprintf("attachment %d", i)
		}
		detail := name
		if file.FileType != "" {
			detail += " (" + file.FileType + ")"
		}
		if file.FileId != "" {
			detail += " id=" + file.FileId
		}
		lines = append(lines, fmt.Sprintf("  user.files.%d - %s", i, detail))
	}
	return "The user attached these files. Any step that needs to look at one takes them as the top-level " +
		"\"files\" key of its request - \"files\": {\"from\": \"user.files\"} - and only the steps that need them should.\n" +
		strings.Join(lines, "\n")
}

// uploadAttachment puts one payload in the store and returns the id it was
// given.
func uploadAttachment(ctx context.Context, store tools.Tooling, file models.FileMessage, payload, projectId, tenantId string) (string, error) {
	name := file.FileName
	if name == "" {
		name = "attachment"
	}
	result, _, err := store.Execute(ctx, projectId, tenantId, erufilesUploadB64, map[string]interface{}{
		"file_name":    name,
		"content_type": file.FileType,
		"data":         payload,
	})
	if err != nil {
		return "", err
	}
	return storedFileId(result), nil
}

// erufilesUploadB64 is the eru-files action that takes a base64 payload.
const erufilesUploadB64 = "upload_b64"

// storedFileId digs the id out of whatever shape the store answered with. The
// providers behind eru-files do not agree on what to call it.
func storedFileId(result map[string]interface{}) string {
	for _, key := range []string{"file_id", "id", "doc_id", "fileId"} {
		if value, ok := result[key].(string); ok && value != "" {
			return value
		}
	}
	for _, nested := range result {
		if inner, ok := nested.(map[string]interface{}); ok {
			if id := storedFileId(inner); id != "" {
				return id
			}
		}
	}
	return ""
}

// attachmentNames lists what the user attached, for the plan checks that need to
// know whether anything was.
func attachmentNames(files []models.FileMessage) []string {
	if len(files) == 0 {
		return nil
	}
	names := make([]string, 0, len(files))
	for i, file := range files {
		name := file.FileName
		if name == "" {
			name = fmt.Sprintf("attachment %d", i)
		}
		names = append(names, name)
	}
	return names
}
