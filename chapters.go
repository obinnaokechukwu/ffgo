//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"time"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/shim"
)

// Chapter represents a chapter marker in a media file.
type Chapter struct {
	ID       int64         // Chapter ID
	Start    time.Duration // Start time
	End      time.Duration // End time
	Title    string        // Chapter title (from metadata)
	Metadata Metadata      // Full chapter metadata
}

// GetChapters returns all chapters from the media file.
func (d *Decoder) GetChapters() []Chapter {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.formatCtx == nil {
		return nil
	}

	numChapters := avformat.GetNumChapters(d.formatCtx)
	if numChapters == 0 {
		return nil
	}

	chapters := make([]Chapter, 0, numChapters)

	for i := 0; i < numChapters; i++ {
		ch := avformat.GetChapter(d.formatCtx, i)
		if ch == nil {
			continue
		}

		// Get time base for conversion
		tbNum, tbDen := avformat.GetChapterTimeBase(ch)
		if tbDen == 0 {
			tbDen = 1
		}

		// Convert start and end to time.Duration
		startPTS := avformat.GetChapterStart(ch)
		endPTS := avformat.GetChapterEnd(ch)

		// Convert to microseconds: pts * num / den * 1000000
		startUS := startPTS * int64(tbNum) * 1000000 / int64(tbDen)
		endUS := endPTS * int64(tbNum) * 1000000 / int64(tbDen)

		// Get metadata
		meta := getChapterMetadata(ch)

		// Extract title from metadata
		title := ""
		if meta != nil {
			title = meta["title"]
		}

		chapters = append(chapters, Chapter{
			ID:       avformat.GetChapterID(ch),
			Start:    time.Duration(startUS) * time.Microsecond,
			End:      time.Duration(endUS) * time.Microsecond,
			Title:    title,
			Metadata: meta,
		})
	}

	return chapters
}

// getChapterMetadata extracts metadata from a chapter as a Metadata map.
func getChapterMetadata(ch avformat.Chapter) Metadata {
	dict := avformat.GetChapterMetadata(ch)
	if dict == nil {
		return nil
	}

	meta := make(Metadata)
	var prev unsafe.Pointer
	for {
		entry := avformat.DictGet(dict, "", prev, avformat.AV_DICT_IGNORE_SUFFIX)
		if entry == nil {
			break
		}
		key := avformat.DictEntryKey(entry)
		value := avformat.DictEntryValue(entry)
		if key != "" {
			meta[key] = value
		}
		prev = entry
	}

	return meta
}

// SetChapters sets chapters for the output file.
// Must be called before WriteHeader.
func (e *Encoder) SetChapters(chapters []Chapter) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("ffgo: encoder is closed")
	}
	if e.headerWritten {
		return errors.New("ffgo: SetChapters must be called before WriteHeader")
	}
	if e.formatCtx == nil {
		return errors.New("ffgo: encoder not initialized")
	}

	// Check if shim is loaded (required for chapter writing)
	if !shim.IsLoaded() {
		return errors.New("ffgo: shim not loaded, chapter writing not available")
	}

	// Use millisecond time base for chapters (1/1000)
	const tbNum int32 = 1
	const tbDen int32 = 1000

	for i, ch := range chapters {
		// Convert time.Duration to PTS in milliseconds
		startPTS := int64(ch.Start / time.Millisecond)
		endPTS := int64(ch.End / time.Millisecond)

		// Use provided ID or generate one
		id := ch.ID
		if id == 0 {
			id = int64(i)
		}

		// Create metadata dictionary for title (errors are non-fatal for metadata)
		var metadata avutil.Dictionary
		if ch.Title != "" {
			_ = avutil.DictSet(&metadata, "title", ch.Title, 0)
			// Add any additional metadata
			for k, v := range ch.Metadata {
				if k != "title" { // Don't duplicate title
					_ = avutil.DictSet(&metadata, k, v, 0)
				}
			}
		}

		// Create chapter using shim
		_, err := shim.NewChapter(
			unsafe.Pointer(e.formatCtx),
			id,
			tbNum, tbDen,
			startPTS, endPTS,
			metadata,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
