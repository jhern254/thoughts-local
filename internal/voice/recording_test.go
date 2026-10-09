package voice

import "testing"

func TestTranscriptAuthority_ApplyTranscriptUpdate(t *testing.T) {
	recordingKey := RecordingKey{SessionID: "session", DraftID: 1, RecordingID: 2}
	t.Run("accepts only current increasing revisions", func(t *testing.T) {
		authority := NewTranscriptAuthority(recordingKey)
		acceptedText := ""
		apply := func(text string) bool { acceptedText = text; return true }
		if !authority.ApplyTranscriptUpdate(TranscriptUpdate{Recording: recordingKey, Revision: 1, Text: "first"}, apply) {
			t.Fatal("current transcript rejected")
		}
		for _, rejected := range []TranscriptUpdate{
			{Recording: recordingKey, Revision: 1, Text: "duplicate"},
			{Recording: RecordingKey{SessionID: "old", DraftID: 1, RecordingID: 2}, Revision: 2, Text: "old session"},
			{Recording: RecordingKey{SessionID: "session", DraftID: 9, RecordingID: 2}, Revision: 2, Text: "old draft"},
			{Recording: RecordingKey{SessionID: "session", DraftID: 1, RecordingID: 1}, Revision: 2, Text: "old recording"},
		} {
			if authority.ApplyTranscriptUpdate(rejected, apply) {
				t.Fatal("stale transcript accepted")
			}
		}
		if acceptedText != "first" {
			t.Fatalf("got %q, want first", acceptedText)
		}
		authority.Revoke()
		if authority.ApplyTranscriptUpdate(TranscriptUpdate{Recording: recordingKey, Revision: 3, Text: "late"}, apply) {
			t.Fatal("revoked recording changed draft")
		}
	})
	t.Run("failed editor validation does not accept a revision", func(t *testing.T) {
		authority := NewTranscriptAuthority(recordingKey)
		update := TranscriptUpdate{Recording: recordingKey, Revision: 1, Text: "text"}
		if authority.ApplyTranscriptUpdate(update, func(string) bool { return false }) {
			t.Fatal("failed edit accepted")
		}
		if !authority.ApplyTranscriptUpdate(update, func(string) bool { return true }) {
			t.Fatal("unaccepted revision consumed")
		}
	})
}

func TestTranscriptAuthority_Revoke(t *testing.T) {
	t.Run("revocation and synchronous draft mutation share one authority boundary", func(t *testing.T) {
		recording := RecordingKey{SessionID: "session", DraftID: 1, RecordingID: 1}
		authority := NewTranscriptAuthority(recording)
		mutationEntered := make(chan struct{})
		finishMutation := make(chan struct{})
		mutationDone := make(chan bool, 1)
		go func() {
			mutationDone <- authority.ApplyTranscriptUpdate(TranscriptUpdate{Recording: recording, Revision: 1, Text: "before Stop"}, func(string) bool {
				close(mutationEntered)
				<-finishMutation
				return true
			})
		}()
		<-mutationEntered
		revocationStarted := make(chan struct{})
		revocationDone := make(chan struct{})
		go func() { close(revocationStarted); authority.Revoke(); close(revocationDone) }()
		<-revocationStarted
		select {
		case <-revocationDone:
			t.Error("revocation returned while draft mutation still held authority")
		default:
		}
		close(finishMutation)
		if !<-mutationDone {
			t.Error("mutation preceding Stop was rejected")
		}
		<-revocationDone
		if authority.ApplyTranscriptUpdate(TranscriptUpdate{Recording: recording, Revision: 2, Text: "after Stop"}, func(string) bool { t.Error("mutated after Stop"); return true }) {
			t.Error("accepted result after revocation")
		}
	})
}
