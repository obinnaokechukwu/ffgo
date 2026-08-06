//go:build !ios && !android && (amd64 || arm64)

package swresample

import (
	"testing"
	"unsafe"
)

func TestConvertMonoFLTPToStereoS16(t *testing.T) {
	context := AllocSetOpts(nil,
		ChannelLayoutStereo, SampleFormatS16, 48000,
		ChannelLayoutMono, SampleFormatFLTP, 44100)
	if context == nil {
		t.Skip("FFmpeg or the legacy swr_alloc_set_opts API is unavailable")
	}
	defer Free(&context)

	if err := InitContext(context); err != nil {
		t.Fatalf("initialize resampler: %v", err)
	}

	const inputSamples = 1024
	input := make([]float32, inputSamples)
	for i := range input {
		input[i] = 0.25
	}
	inputPlane := unsafe.Pointer(&input[0])
	inputPlanes := []unsafe.Pointer{inputPlane}

	outputSamples := GetOutSamples(context, inputSamples)
	if outputSamples <= 0 {
		t.Fatalf("output capacity = %d, want > 0", outputSamples)
	}
	output := make([]int16, outputSamples*2)
	outputPlane := unsafe.Pointer(&output[0])
	outputPlanes := []unsafe.Pointer{outputPlane}

	converted, err := Convert(
		context,
		unsafe.Pointer(&outputPlanes[0]), int32(outputSamples),
		unsafe.Pointer(&inputPlanes[0]), inputSamples,
	)
	if err != nil {
		t.Fatalf("convert samples: %v", err)
	}
	if converted <= 0 {
		t.Fatalf("converted samples = %d, want > 0", converted)
	}

	for _, sample := range output[:converted*2] {
		if sample != 0 {
			return
		}
	}
	t.Fatal("converted audio is silent")
}
