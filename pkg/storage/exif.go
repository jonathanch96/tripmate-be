package storage

import (
	"bytes"
	"encoding/binary"
)

// StripMetadata removes EXIF and XMP blocks — which is where a phone puts GPS coordinates — before
// an image is stored (ST-5).
//
// This edits the container rather than decoding and re-encoding the picture: re-encoding a JPEG
// would throw away image quality on every upload, and Go has no WebP encoder at all. Anything not
// recognised is returned untouched; the sniffing in DetectImageType has already established that
// the bytes are one of the three types handled here.
func StripMetadata(data []byte) []byte {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8}):
		return stripJPEG(data)
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return stripPNG(data)
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return stripWebP(data)
	}
	return data
}

// stripJPEG drops every APPn marker segment. EXIF lives in APP1, XMP in APP1 too, and the various
// vendor blobs occupy the rest; none of them are needed to render the picture.
func stripJPEG(data []byte) []byte {
	out := make([]byte, 0, len(data))
	out = append(out, 0xFF, 0xD8)
	index := 2
	for index+3 < len(data) {
		if data[index] != 0xFF {
			break
		}
		marker := data[index+1]
		// Start of scan: the entropy-coded image data runs to the end, so copy the rest verbatim.
		if marker == 0xDA {
			out = append(out, data[index:]...)
			return out
		}
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD9) {
			out = append(out, data[index], data[index+1])
			index += 2
			continue
		}
		length := int(binary.BigEndian.Uint16(data[index+2 : index+4]))
		if length < 2 || index+2+length > len(data) {
			break
		}
		segment := data[index : index+2+length]
		if marker >= 0xE0 && marker <= 0xEF {
			index += 2 + length
			continue
		}
		out = append(out, segment...)
		index += 2 + length
	}
	if index < len(data) {
		out = append(out, data[index:]...)
	}
	return out
}

// stripPNG drops the metadata chunks: eXIf holds EXIF, and the text chunks can carry anything.
func stripPNG(data []byte) []byte {
	const headerLen = 8
	dropped := map[string]bool{"eXIf": true, "tEXt": true, "iTXt": true, "zTXt": true}
	out := make([]byte, 0, len(data))
	out = append(out, data[:headerLen]...)
	index := headerLen
	for index+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[index : index+4]))
		if length < 0 || index+12+length > len(data) {
			break
		}
		kind := string(data[index+4 : index+8])
		chunk := data[index : index+12+length]
		if !dropped[kind] {
			out = append(out, chunk...)
		}
		index += 12 + length
		if kind == "IEND" {
			return out
		}
	}
	if index < len(data) {
		out = append(out, data[index:]...)
	}
	return out
}

// stripWebP drops the EXIF and XMP chunks from the RIFF container and fixes up the file size field.
func stripWebP(data []byte) []byte {
	const headerLen = 12
	if len(data) < headerLen {
		return data
	}
	dropped := map[string]bool{"EXIF": true, "XMP ": true}
	body := make([]byte, 0, len(data))
	index := headerLen
	for index+8 <= len(data) {
		size := int(binary.LittleEndian.Uint32(data[index+4 : index+8]))
		padded := size + size%2
		if size < 0 || index+8+padded > len(data) {
			break
		}
		if !dropped[string(data[index:index+4])] {
			body = append(body, data[index:index+8+padded]...)
		}
		index += 8 + padded
	}
	out := make([]byte, 0, headerLen+len(body))
	out = append(out, data[:headerLen]...)
	out = append(out, body...)
	// RIFF size counts everything after the first eight bytes.
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}
