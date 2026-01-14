//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
)

// Metadata represents key-value metadata from a media file.
// Common keys include: title, artist, album, genre, date, track, disc, etc.
type Metadata map[string]string

// GetMetadata returns all metadata from the decoder's format context.
// This includes file-level metadata like title, artist, album, etc.
func (d *Decoder) GetMetadata() Metadata {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.formatCtx == nil {
		return nil
	}

	return getMetadataFromDict(avformat.GetMetadata(d.formatCtx))
}

// GetStreamMetadata returns metadata for a specific stream.
// Stream index 0 is typically video, 1 is audio for most files.
func (d *Decoder) GetStreamMetadata(streamIndex int) Metadata {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.formatCtx == nil {
		return nil
	}

	numStreams := avformat.GetNumStreams(d.formatCtx)
	if streamIndex < 0 || streamIndex >= numStreams {
		return nil
	}

	stream := avformat.GetStream(d.formatCtx, streamIndex)
	if stream == nil {
		return nil
	}

	return getMetadataFromDict(avformat.GetStreamMetadata(stream))
}

// SetMetadata sets metadata on the encoder's output file.
// Must be called before WriteHeader.
func (e *Encoder) SetMetadata(metadata Metadata) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.formatCtx == nil {
		return ErrEncoderClosed
	}
	if e.headerWritten {
		return ErrHeaderAlreadyWritten
	}

	for key, value := range metadata {
		if err := avformat.SetMetadata(e.formatCtx, key, value); err != nil {
			return err
		}
	}

	return nil
}

// SetStreamMetadata sets metadata on a specific stream.
// Stream index 0 is video, 1 is audio (if present).
// Must be called before WriteHeader.
func (e *Encoder) SetStreamMetadata(streamIndex int, metadata Metadata) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.formatCtx == nil {
		return ErrEncoderClosed
	}
	if e.headerWritten {
		return ErrHeaderAlreadyWritten
	}

	var stream avformat.Stream
	switch streamIndex {
	case 0:
		stream = e.videoStream
	case 1:
		stream = e.audioStream
	default:
		return ErrInvalidStream
	}

	if stream == nil {
		return ErrInvalidStream
	}

	for key, value := range metadata {
		if err := avformat.SetStreamMetadata(stream, key, value); err != nil {
			return err
		}
	}

	return nil
}

// Common metadata keys
const (
	MetadataTitle       = "title"
	MetadataArtist      = "artist"
	MetadataAlbum       = "album"
	MetadataAlbumArtist = "album_artist"
	MetadataGenre       = "genre"
	MetadataDate        = "date"
	MetadataTrack       = "track"
	MetadataDisc        = "disc"
	MetadataComposer    = "composer"
	MetadataPublisher   = "publisher"
	MetadataComment     = "comment"
	MetadataDescription = "description"
	MetadataEncoder     = "encoder"
	MetadataLanguage    = "language"
	MetadataCopyright   = "copyright"
)

// Helper to convert AVDictionary to Metadata map
func getMetadataFromDict(dict avutil.Dictionary) Metadata {
	if dict == nil {
		return nil
	}

	result := make(Metadata)

	// Iterate through dictionary entries
	// AVDictionary entries are stored as AVDictionaryEntry structs
	// We use av_dict_get with AV_DICT_IGNORE_SUFFIX to iterate all entries
	var prev unsafe.Pointer
	for {
		entry := avformat.DictGet(dict, "", prev, avformat.AV_DICT_IGNORE_SUFFIX)
		if entry == nil {
			break
		}

		key := avformat.DictEntryKey(entry)
		value := avformat.DictEntryValue(entry)
		if key != "" {
			result[key] = value
		}

		prev = entry
	}

	return result
}

// ErrEncoderClosed is returned when operating on a closed encoder.
var ErrEncoderClosed = errEncoderClosed{}

type errEncoderClosed struct{}

func (e errEncoderClosed) Error() string { return "ffgo: encoder is closed" }

// ErrHeaderAlreadyWritten is returned when trying to modify settings after header was written.
var ErrHeaderAlreadyWritten = errHeaderAlreadyWritten{}

type errHeaderAlreadyWritten struct{}

func (e errHeaderAlreadyWritten) Error() string { return "ffgo: header already written" }

// ErrInvalidStream is returned when accessing an invalid stream index.
var ErrInvalidStream = errInvalidStream{}

type errInvalidStream struct{}

func (e errInvalidStream) Error() string { return "ffgo: invalid stream index" }
