package visual

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestPrepareImage(t *testing.T) {
	t.Run("resizing normalizes orientation and discards EXIF for JPEG and PNG", func(t *testing.T) {
		// Little-endian TIFF with one orientation tag: 6 means clockwise 90°.
		tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
		for _, format := range []string{"jpeg", "png"} {
			var source bytes.Buffer
			img := image.NewNRGBA(image.Rect(0, 0, 4096, 256))
			var metadata []byte
			var offset int
			if format == "jpeg" {
				if err := jpeg.Encode(&source, img, nil); err != nil {
					t.Fatal(err)
				}
				metadata = append([]byte{0xff, 0xe1, 0, 34, 'E', 'x', 'i', 'f', 0, 0}, tiff...)
				offset = 2
			} else {
				if err := png.Encode(&source, img); err != nil {
					t.Fatal(err)
				}
				metadata = make([]byte, 12+len(tiff))
				binary.BigEndian.PutUint32(metadata[:4], uint32(len(tiff)))
				copy(metadata[4:8], "eXIf")
				copy(metadata[8:], tiff)
				binary.BigEndian.PutUint32(metadata[len(metadata)-4:], crc32.ChecksumIEEE(metadata[4:len(metadata)-4]))
				offset = 33
			}
			body := append(append(append([]byte{}, source.Bytes()[:offset]...), metadata...), source.Bytes()[offset:]...)
			prepared, _, err := prepareImage(t.Context(), body)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(prepared, tiff) {
				t.Fatalf("%s retained source EXIF", format)
			}
			if config, _, err := image.DecodeConfig(bytes.NewReader(prepared)); err != nil || config.Width != 192 || config.Height != 3072 {
				t.Fatalf("%s stored config = %+v, %v", format, config, err)
			}
		}
	})
	for _, format := range []string{"jpeg", "png"} {
		for _, size := range []image.Point{{12, 8}, {6000, 4000}, {91, 4093}, {4093, 91}, {8192, 1}} {
			t.Run(format+"/"+size.String(), func(t *testing.T) {
				img := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
				for i := 0; i < len(img.Pix); i += 4 {
					copy(img.Pix[i:i+4], []byte{80, 160, 200, 128})
				}
				var source bytes.Buffer
				if format == "jpeg" {
					if err := jpeg.Encode(&source, img, nil); err != nil {
						t.Fatal(err)
					}
				} else if err := png.Encode(&source, img); err != nil {
					t.Fatal(err)
				}
				body, gotFormat, err := prepareImage(t.Context(), source.Bytes())
				if err != nil || gotFormat != format {
					t.Fatalf("format = %q, err %v; want %s", gotFormat, err, format)
				}
				decoded, _, err := image.Decode(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				got := decoded.Bounds().Size()
				if got.X > 3072 || got.Y > 3072 || got.X*got.Y > 4_000_000 || got.X < 1 || got.Y < 1 {
					t.Fatalf("stored size = %v, exceeds budget", got)
				}
				if size == (image.Point{12, 8}) && got != size {
					t.Fatalf("small image size = %v, want %v", got, size)
				}
				if size == (image.Point{12, 8}) {
					original, _, err := image.Decode(bytes.NewReader(source.Bytes()))
					if err != nil {
						t.Fatal(err)
					}
					want := color.NRGBAModel.Convert(original.At(0, 0)).(color.NRGBA)
					pixel := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA)
					for index, channel := range []uint8{pixel.R, pixel.G, pixel.B, pixel.A} {
						reference := []uint8{want.R, want.G, want.B, want.A}[index]
						difference := int(channel) - int(reference)
						if difference < -3 || difference > 3 {
							t.Fatalf("small image pixel=%v, want approximately %v", pixel, want)
						}
					}
				}
				if size == (image.Point{6000, 4000}) && got != (image.Point{2449, 1632}) {
					t.Fatalf("size = %v, want (2449,1632)", got)
				}
				if size == (image.Point{91, 4093}) && got != (image.Point{68, 3072}) {
					t.Fatalf("size = %v, want (68,3072)", got)
				}
				if format == "png" {
					if alpha := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA).A; alpha != 128 {
						t.Fatalf("alpha = %d, want 128", alpha)
					}
				}
			})
		}
	}
	t.Run("cancelled import does not prepare an image", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, _, err := prepareImage(ctx, samplePNG(t)); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want cancellation", err)
		}
	})
}

func TestBackgroundDimensions(t *testing.T) {
	for _, size := range []image.Point{{2000, 2000}, {3072, 1}, {1, 3072}, {1, 1}} {
		t.Run(size.String()+" stays unchanged at the budget", func(t *testing.T) {
			width, height := backgroundDimensions(size.X, size.Y)
			if got := (image.Point{width, height}); got != size {
				t.Fatalf("size = %v, want %v", got, size)
			}
		})
	}
}

func BenchmarkPrepareImage(b *testing.B) {
	for _, size := range []image.Point{{1267, 719}, {6000, 4000}} {
		img := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x + y), 255})
			}
		}
		var source bytes.Buffer
		if err := png.Encode(&source, img); err != nil {
			b.Fatal(err)
		}
		b.Run(size.String(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := prepareImage(b.Context(), source.Bytes()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
