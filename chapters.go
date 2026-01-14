//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"time"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avformat"
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
