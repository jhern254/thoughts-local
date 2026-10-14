package visual

import (
	"encoding/binary"
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

// Only IFD0's inline SHORT orientation is read. GPS, thumbnails and the other
// TIFF directories are intentionally not traversed or retained.
func readOrientation(tiff []byte) (int, error) {
	if len(tiff) < 8 {
		return 0, ErrImage
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II": // TIFF little-endian byte order (Intel).
		order = binary.LittleEndian
	case "MM": // TIFF big-endian byte order (Motorola).
		order = binary.BigEndian
	default:
		return 0, ErrImage
	}
	if order.Uint16(tiff[2:4]) != 42 { // Classic TIFF magic; BigTIFF is outside this EXIF subset.
		return 0, ErrImage
	}
	// Bytes 4–7 point to IFD0 relative to the TIFF header, not the JPEG/PNG file.
	offset := uint64(order.Uint32(tiff[4:8]))
	if offset < 8 || offset+2 > uint64(len(tiff)) {
		return 0, ErrImage
	}
	// An IFD has a uint16 entry count, twelve-byte records, and a
	// trailing uint32 next-directory pointer. Orientation fits inline.
	count := uint64(order.Uint16(tiff[offset : offset+2]))
	if offset+2+count*12+4 > uint64(len(tiff)) {
		return 0, ErrImage
	}
	orientation := 1
	found := false
	for index := uint64(0); index < count; index++ {
		entry := tiff[offset+2+index*12 : offset+2+(index+1)*12]
		if order.Uint16(entry[:2]) != 0x112 { // TIFF Orientation tag (274).
			continue
		}
		// Type 3 is SHORT (uint16); count 1 fits in the four-byte inline value
		// field at bytes 8–11. Values 1–8 describe the standard display transforms.
		value := int(order.Uint16(entry[8:10]))
		if found || order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 || value < 1 || value > 8 {
			return 0, ErrImage
		}
		found = true
		orientation = value
	}
	return orientation, nil
}

// Applying orientation to pixels removes the need to preserve private EXIF and
// makes stored dimensions agree with the browser's display-oriented preview.
func normalizeOrientation(source image.Image, orientation int) image.Image {
	if orientation == 1 {
		return source
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if orientation >= 5 { // Transpose/90° transforms exchange displayed width and height.
		width, height = height, width
	}
	// Draw into a concrete buffer once, then copy whole pixels. This avoids
	// allocating a color interface for every pixel during rotation/mirroring.
	var input, output []byte
	var sourceStride, destinationStride, pixelBytes int
	var normalized image.Image
	switch source.ColorModel() {
	case color.GrayModel:
		buffer, concrete := source.(*image.Gray)
		if !concrete {
			buffer = image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
			draw.Draw(buffer, buffer.Bounds(), source, bounds.Min, draw.Src)
		}
		destination := image.NewGray(image.Rect(0, 0, width, height))
		input, output = buffer.Pix, destination.Pix
		sourceStride, destinationStride, pixelBytes = buffer.Stride, destination.Stride, 1
		normalized = destination
	case color.Gray16Model:
		buffer, concrete := source.(*image.Gray16)
		if !concrete {
			buffer = image.NewGray16(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
			draw.Draw(buffer, buffer.Bounds(), source, bounds.Min, draw.Src)
		}
		destination := image.NewGray16(image.Rect(0, 0, width, height))
		input, output = buffer.Pix, destination.Pix
		sourceStride, destinationStride, pixelBytes = buffer.Stride, destination.Stride, 2
		normalized = destination
	case color.RGBA64Model, color.NRGBA64Model:
		buffer, concrete := source.(*image.NRGBA64)
		if !concrete {
			buffer = image.NewNRGBA64(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
			draw.Draw(buffer, buffer.Bounds(), source, bounds.Min, draw.Src)
		}
		destination := image.NewNRGBA64(image.Rect(0, 0, width, height))
		input, output = buffer.Pix, destination.Pix
		sourceStride, destinationStride, pixelBytes = buffer.Stride, destination.Stride, 8
		normalized = destination
	default:
		buffer, concrete := source.(*image.NRGBA)
		if !concrete {
			buffer = image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
			draw.Draw(buffer, buffer.Bounds(), source, bounds.Min, draw.Src)
		}
		destination := image.NewNRGBA(image.Rect(0, 0, width, height))
		input, output = buffer.Pix, destination.Pix
		sourceStride, destinationStride, pixelBytes = buffer.Stride, destination.Stride, 4
		normalized = destination
	}
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			destinationX, destinationY := x, y
			// EXIF orientation maps encoded coordinates to upright display:
			// 1 unchanged; 2 horizontal mirror; 3 rotate 180°; 4 vertical mirror;
			// 5 transpose; 6 rotate 90° clockwise; 7 transverse; 8 rotate 90° counterclockwise.
			switch orientation {
			case 2:
				destinationX = bounds.Dx() - 1 - x
			case 3:
				destinationX, destinationY = bounds.Dx()-1-x, bounds.Dy()-1-y
			case 4:
				destinationY = bounds.Dy() - 1 - y
			case 5:
				destinationX, destinationY = y, x
			case 6:
				destinationX, destinationY = bounds.Dy()-1-y, x
			case 7:
				destinationX, destinationY = bounds.Dy()-1-y, bounds.Dx()-1-x
			case 8:
				destinationX, destinationY = y, bounds.Dx()-1-x
			}
			// Strides are bytes per row; pixelBytes is 1/2 for gray8/16,
			// or 4/8 for RGBA8/16. Copy complete pixels to retain alpha/depth.
			from := y*sourceStride + x*pixelBytes
			to := destinationY*destinationStride + destinationX*pixelBytes
			copy(output[to:to+pixelBytes], input[from:from+pixelBytes])
		}
	}
	return normalized
}

func resizeBackground(source image.Image) image.Image {
	width, height := backgroundDimensions(source.Bounds().Dx(), source.Bounds().Dy())
	if width == source.Bounds().Dx() && height == source.Bounds().Dy() {
		return source
	}
	bounds := image.Rect(0, 0, width, height)
	var destination draw.Image
	switch source.ColorModel() {
	case color.GrayModel:
		destination = image.NewGray(bounds)
	case color.Gray16Model:
		destination = image.NewGray16(bounds)
	case color.RGBA64Model, color.NRGBA64Model:
		destination = image.NewRGBA64(bounds)
	default:
		destination = image.NewRGBA(bounds)
	}
	// Interpolation stays in the source's encoded component space; there is no
	// ICC conversion. The matching display profile must accompany the output.
	draw.ApproxBiLinear.Scale(destination, bounds, source, source.Bounds(), draw.Src, nil)
	return destination
}
