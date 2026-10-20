package browserterm

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestVideoQuality_Routes(t *testing.T) {
	for _, tier := range []visual.VideoQuality{visual.VideoQualityEfficient, visual.VideoQualityHigh, visual.VideoQualityMaximum} {
		t.Run(string(tier), func(t *testing.T) {
			for _, tool := range []string{"ffmpeg", "ffprobe"} {
				if _, err := exec.LookPath(tool); err != nil {
					t.Skip("video tools not installed")
				}
			}
			body, err := os.ReadFile("client/fixtures/visual-video.mp4")
			if err != nil {
				t.Fatal(err)
			}
			store := &visualStore{}
			s := &server{
				videoEditor: newVideoEditor(t.Context()),
				visual:      visual.NewService(store, "user", t.TempDir()),
				authority:   "127.0.0.1:7777",
			}
			id, _ := s.videoEditor.setOpen(true)
			defer s.videoEditor.Close()
			staged := videoEditRequest(t, s, id, "POST", "/visual/video/source", body)
			var source struct {
				SourceID string `json:"sourceID"`
			}
			if staged.Code != 200 {
				t.Fatalf("stage=%d", staged.Code)
			}
			if err := json.Unmarshal(staged.Body.Bytes(), &source); err != nil {
				t.Fatal(err)
			}
			controls := func(quality visual.VideoQuality) []byte {
				b, _ := json.Marshal(map[string]any{"sourceID": source.SourceID, "trim": visual.VideoTrim{EndMS: 1000}, "videoQuality": quality})
				return b
			}
			prepared := videoEditRequest(t, s, id, "POST", "/visual/video/preview", controls(tier))
			if prepared.Code != 200 {
				t.Fatalf("prepare=%d %s", prepared.Code, prepared.Body.String())
			}
			var candidate struct {
				PreviewID string `json:"previewID"`
				visual.PreparedVideoInfo
			}
			if err := json.Unmarshal(prepared.Body.Bytes(), &candidate); err != nil {
				t.Fatal(err)
			}
			if candidate.VideoQuality != tier || candidate.FrameRate.String() != "24/1" || candidate.Width != 96 || candidate.Height != 64 {
				t.Fatalf("metadata=%+v", candidate)
			}
			accept, _ := json.Marshal(map[string]string{"previewID": candidate.PreviewID})
			if w := videoEditRequest(t, s, id, "PUT", "/visual/video/preview", accept); w.Code != 200 {
				t.Fatalf("ack=%d", w.Code)
			}
			video := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+candidate.PreviewID+"&part=video", nil)
			poster := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+candidate.PreviewID+"&part=poster", nil)
			changed := visual.VideoQualityMaximum
			if tier == changed {
				changed = visual.VideoQualityEfficient
			}
			next := videoEditRequest(t, s, id, "POST", "/visual/video/preview", controls(changed))
			if next.Code != 200 {
				t.Fatalf("changed tier=%d %s", next.Code, next.Body.String())
			}
			var unaccepted struct {
				PreviewID string `json:"previewID"`
			}
			if err := json.Unmarshal(next.Body.Bytes(), &unaccepted); err != nil {
				t.Fatal(err)
			}
			if unaccepted.PreviewID == candidate.PreviewID {
				t.Fatal("different quality reused the accepted candidate identity")
			}
			invalid := videoEditRequest(t, s, id, "POST", "/visual/video/preview", controls(visual.VideoQuality("invalid")))
			if invalid.Code != 400 {
				t.Fatalf("invalid tier=%d", invalid.Code)
			}
			t.Setenv("PATH", t.TempDir())
			apply, _ := json.Marshal(map[string]any{"previewID": candidate.PreviewID, "darkness": 42, "framing": data.DefaultBackgroundFraming(), "speedPercent": 50, "userPaused": true})
			saved := videoEditRequest(t, s, id, "POST", "/visual/video", apply)
			if saved.Code != 200 {
				t.Fatalf("Apply=%d %s", saved.Code, saved.Body.String())
			}
			if store.settings.VideoQuality != string(tier) {
				t.Fatalf("saved quality=%q, want %q", store.settings.VideoQuality, tier)
			}
			file, _, err := s.visual.OpenBackground(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := io.ReadAll(file)
			file.Close()
			if !bytes.Equal(actual, video.Body.Bytes()) {
				t.Fatal("Apply changed accepted video bytes")
			}
			file, _, err = s.visual.OpenPoster(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			actual, _ = io.ReadAll(file)
			file.Close()
			if !bytes.Equal(actual, poster.Body.Bytes()) {
				t.Fatal("Apply changed accepted poster bytes")
			}
		})
	}
}
