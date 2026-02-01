//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"

	"github.com/obinnaokechukwu/ffgo/avcodec"
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

// AddAttachment adds an attachment to the output file.
// Attachments are commonly used in MKV files for fonts, cover art, etc.
// Must be called before WriteHeader.
func (e *Encoder) AddAttachment(att Attachment) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("ffgo: encoder is closed")
	}
	if e.headerWritten {
		return errors.New("ffgo: AddAttachment must be called before WriteHeader")
	}
	if e.formatCtx == nil {
		return errors.New("ffgo: encoder not initialized")
	}
	if len(att.Data) == 0 {
		return errors.New("ffgo: attachment data is empty")
	}

	// Create a new stream for the attachment
	stream := avformat.NewStream(e.formatCtx, nil)
	if stream == nil {
		return errors.New("ffgo: failed to create attachment stream")
	}

	// Get codec parameters for the stream
	codecPar := avformat.GetStreamCodecPar(stream)
	if codecPar == nil {
		return errors.New("ffgo: failed to get codec parameters")
	}

	// Set media type to attachment
	avformat.SetCodecParType(codecPar, avutil.MediaTypeAttachment)

	// Determine codec ID based on MIME type or filename
	codecID := guessAttachmentCodecID(att.MimeType, att.Filename)
	avformat.SetCodecParCodecID(codecPar, codecID)

	// Set extradata to the attachment content
	avformat.SetCodecParExtradata(codecPar, att.Data)

	// Set metadata on the stream (errors are non-fatal for metadata)
	if att.Filename != "" {
		_ = avformat.SetStreamMetadata(stream, "filename", att.Filename)
	}
	if att.MimeType != "" {
		_ = avformat.SetStreamMetadata(stream, "mimetype", att.MimeType)
	}
	if att.Description != "" {
		_ = avformat.SetStreamMetadata(stream, "title", att.Description)
	}

	return nil
}

// guessAttachmentCodecID guesses the codec ID based on MIME type or filename.
func guessAttachmentCodecID(mimeType, filename string) avcodec.CodecID {
	// Check MIME type first
	switch mimeType {
	case "application/x-truetype-font", "font/ttf", "font/otf",
		"application/x-font-ttf", "application/x-font-opentype":
		return avcodec.CodecIDTTF
	case "image/png":
		return avcodec.CodecIDPNG
	case "image/jpeg":
		return avcodec.CodecIDMJPEG
	case "image/gif":
		return avcodec.CodecIDGIF
	case "image/webp":
		return avcodec.CodecIDWEBP
	}

	// Check filename extension
	ext := ""
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			ext = filename[i+1:]
			break
		}
	}

	switch ext {
	case "ttf", "TTF", "otf", "OTF":
		return avcodec.CodecIDTTF
	case "png", "PNG":
		return avcodec.CodecIDPNG
	case "jpg", "jpeg", "JPG", "JPEG":
		return avcodec.CodecIDMJPEG
	case "gif", "GIF":
		return avcodec.CodecIDGIF
	case "webp", "WEBP":
		return avcodec.CodecIDWEBP
	}

	// Default to binary data
	return avcodec.CodecIDBinData
}
