//go:build !ios && !android && (amd64 || arm64)

package avutil

import (
	"errors"
	"fmt"
	"syscall"
)

// Common FFmpeg error codes (AVERROR values)
const (
	AVERROR_EOF               int32 = -541478725             // End of file
	AVERROR_EAGAIN            int32 = -int32(syscall.EAGAIN) // Resource temporarily unavailable
	AVERROR_EINVAL            int32 = -int32(syscall.EINVAL) // Invalid argument
	AVERROR_ENOMEM            int32 = -int32(syscall.ENOMEM) // Out of memory
	AVERROR_DECODER_NOT_FOUND int32 = -1128613112            // Decoder not found
	AVERROR_ENCODER_NOT_FOUND int32 = -1129203192            // Encoder not found
	AVERROR_DEMUXER_NOT_FOUND int32 = -1296385272            // Demuxer not found
	AVERROR_MUXER_NOT_FOUND   int32 = -1381258232            // Muxer not found
	AVERROR_STREAM_NOT_FOUND  int32 = -1381258232            // Stream not found
	AVERROR_INVALIDDATA       int32 = -1094995529            // Invalid data
	AVERROR_BUG               int32 = -558323010             // Bug detected
	AVERROR_UNKNOWN           int32 = -1313558101            // Unknown error
)

// Error represents an FFmpeg error.
type Error struct {
	Code    int32  // Raw FFmpeg error code
	Message string // Human-readable message
	Op      string // Operation that failed
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("ffmpeg %s: %s (code %d)", e.Op, e.Message, e.Code)
}

// NewError creates a new FFmpeg error from an error code.
func NewError(code int32, op string) error {
	if code >= 0 {
		return nil
	}
	return &Error{
		Code:    code,
		Message: ErrorString(code),
		Op:      op,
	}
}

// IsEOF returns true if the error indicates end of file.
func IsEOF(err error) bool {
	var ffErr *Error
	if errors.As(err, &ffErr) {
		return ffErr.Code == AVERROR_EOF
	}
	return false
}

// IsAgain returns true if the error indicates to try again (EAGAIN).
// This is common during decoding when more data is needed.
func IsAgain(err error) bool {
	var ffErr *Error
	if errors.As(err, &ffErr) {
		return ffErr.Code == AVERROR_EAGAIN
	}
	return false
}

// IsInvalidData returns true if the error indicates invalid data.
func IsInvalidData(err error) bool {
	var ffErr *Error
	if errors.As(err, &ffErr) {
		return ffErr.Code == AVERROR_INVALIDDATA
	}
	return false
}

// Code returns the FFmpeg error code from an error, or 0 if not an FFmpeg error.
func Code(err error) int32 {
	var ffErr *Error
	if errors.As(err, &ffErr) {
		return ffErr.Code
	}
	return 0
}
