package dgmedia

import (
	"errors"
	"math"
	"unsafe"
)

const (
	samplesPerFrame int = 2
	bitsPerSample   int = 16
	bytesPerSample      = bitsPerSample / 8
)

func ResampleSingleChannel(input []byte, inputSample int, outputSample int) []byte {
	inputSize := len(input)
	ratio := inputSample / outputSample

	output := make([]byte, 0, inputSize/ratio)
	step := ratio * 2
	for i := 0; i < inputSize; i += step {
		//var v int16
		//v = int16(input[i+1])
		//v = v << 8
		//v += int16(input[i])
		//v /= 2
		//output = append(output, byte(v))
		//output = append(output, byte(v>>8))
		output = append(output, input[i])
		output = append(output, input[i+1])
	}
	return output
}

func ResampleDualChannel(input []byte, inputSample int, outputSample int) []byte {
	inputSize := len(input)
	ratio := inputSample / outputSample

	output := make([]byte, 0, inputSize/ratio)
	step := ratio * 4
	for i := 0; i < inputSize; i += step {
		output = append(output, input[i])
		output = append(output, input[i+1])
		output = append(output, input[i+2])
		output = append(output, input[i+3])
	}
	return output
}

// SimpleF32ToS16le f32 48000  to s16le 16000
func SimpleF32ToS16le(input []byte) ([]byte, error) {
	if len(input)%4 != 0 {
		return nil, errors.New("invalid f32 input")
	}
	nSamples := len(input) / 4
	output := make([]byte, 0, nSamples*2/3)

	for i := 0; i < nSamples; i++ {
		if i%3 == 0 {
			one := readFloat32Le(input, i*4)
			v := float2int16(one)
			output = append(output, byte(v))
			output = append(output, byte(v>>8))
		}
	}
	return output, nil
}

func F32ToS16le(input []byte) ([]byte, error) {
	if len(input)%4 != 0 {
		return nil, errors.New("invalid f32 input")
	}
	nSamples := len(input) / 4
	output := make([]byte, 0, nSamples*2)

	for i := 0; i < nSamples; i++ {
		one := readFloat32Le(input, i*4)
		v := float2int16(one)
		output = append(output, byte(v))
		output = append(output, byte(v>>8))
	}
	return output, nil
}

func float2int16(fv float32) int16 {
	scaledValue := fv * 32768.0
	if scaledValue >= 32767.0 {
		return 32767
	} else if scaledValue <= -32768.0 {
		return -32768
	} else {
		return int16(math.Round(float64(scaledValue)))
	}
}

func readFloat32Le(buf []byte, idx int) float32 {
	return *(*float32)(unsafe.Pointer(&buf[idx]))
}
