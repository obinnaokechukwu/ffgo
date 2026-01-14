//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
)

// Attachment represents an embedded file in a media container.
// Attachments are commonly used in MKV files to embed fonts,
// cover art, or other supplementary files.
type Attachment struct {
	Filename    string // Original filename of the attachment
	MimeType    string // MIME type (e.g., "application/x-truetype-font")
	Description string // Optional description
	Data        []byte // The attachment data
}

// GetAttachments returns all attachments from the media file.
// Returns an empty slice if there are no attachments.
func (d *Decoder) GetAttachments() []Attachment {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.formatCtx == nil {
		return nil
	}

	numStreams := avformat.GetNumStreams(d.formatCtx)
	if numStreams == 0 {
		return nil
	}

	var attachments []Attachment

	for i := 0; i < numStreams; i++ {
		stream := avformat.GetStream(d.formatCtx, i)
		if stream == nil {
			continue
		}

		codecPar := avformat.GetStreamCodecPar(stream)
		if codecPar == nil {
			continue
		}

		// Check if this is an attachment stream
		mediaType := avformat.GetCodecParType(codecPar)
		if mediaType != avutil.MediaTypeAttachment {
			continue
		}

		// Get attachment data from extradata
		data := avformat.GetCodecParExtradata(codecPar)
		if len(data) == 0 {
			continue
		}

		// Get metadata for filename, mimetype, description
		streamMeta := avformat.GetStreamMetadata(stream)
		filename := getMetadataValue(streamMeta, "filename")
		mimeType := getMetadataValue(streamMeta, "mimetype")
		description := getMetadataValue(streamMeta, "title") // Often stored as title

		attachments = append(attachments, Attachment{
			Filename:    filename,
			MimeType:    mimeType,
			Description: description,
			Data:        data,
		})
	}

	return attachments
}

// getMetadataValue retrieves a value from a metadata dictionary.
func getMetadataValue(dict avutil.Dictionary, key string) string {
	if dict == nil {
		return ""
	}
	entry := avformat.DictGet(dict, key, nil, 0)
	if entry == nil {
		return ""
	}
	return avformat.DictEntryValue(entry)
}

// HasAttachments returns true if the file has any attachment streams.
func (d *Decoder) HasAttachments() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.formatCtx == nil {
		return false
	}

	numStreams := avformat.GetNumStreams(d.formatCtx)
	for i := 0; i < numStreams; i++ {
		stream := avformat.GetStream(d.formatCtx, i)
		if stream == nil {
			continue
		}

		codecPar := avformat.GetStreamCodecPar(stream)
		if codecPar == nil {
			continue
		}

		if avformat.GetCodecParType(codecPar) == avutil.MediaTypeAttachment {
			return true
		}
	}

	return false
}
