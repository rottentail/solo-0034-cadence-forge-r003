package domain

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

func ValidateRecipe(r Recipe) error {
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 160 {
		return Invalid("recipe name must contain 1..160 bytes")
	}
	if r.Format != "mp4" && r.Format != "wav" {
		return Invalid("format must be mp4 or wav")
	}
	if r.SampleRate != 48000 && r.SampleRate != 44100 {
		return Invalid("sample_rate must be 44100 or 48000")
	}
	if r.Format == "mp4" {
		if r.Width < 64 || r.Height < 64 || r.Width > 1920 || r.Height > 1080 || r.Width%2 != 0 || r.Height%2 != 0 {
			return Invalid("invalid even video dimensions")
		}
		if r.FPS < 1 || r.FPS > 60 {
			return Invalid("fps must be 1..60")
		}
	}
	if r.Normalize && (math.IsNaN(r.TargetLUFS) || math.IsInf(r.TargetLUFS, 0) || r.TargetLUFS < -36 || r.TargetLUFS > -9) {
		return Invalid("normalization target must be -36..-9 LUFS")
	}
	return nil
}
func ValidateSheet(s Sheet, sources map[string]Source) error {
	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 160 {
		return Invalid("sheet name must contain 1..160 bytes")
	}
	if len(s.Spans) == 0 || len(s.Spans) > 32 {
		return Invalid("a sheet requires 1..32 spans")
	}
	var duration int64
	for i, p := range s.Spans {
		source, ok := sources[p.SourceID]
		if !ok {
			return Missing("source", p.SourceID)
		}
		if p.InMS < 0 || p.OutMS <= p.InMS || p.OutMS > source.Media.DurationMS {
			return Invalid("span %d is outside source duration", i)
		}
		if p.OutMS-p.InMS < 100 {
			return Invalid("span %d is shorter than 100ms", i)
		}
		if math.IsNaN(p.GainDB) || math.IsInf(p.GainDB, 0) || p.GainDB < -60 || p.GainDB > 12 {
			return Invalid("span %d has invalid gain", i)
		}
		duration += p.OutMS - p.InMS
	}
	if duration > 120000 {
		return Invalid("sheet exceeds 120 seconds")
	}
	if len(s.Cues) > 500 {
		return Invalid("sheet exceeds 500 cues")
	}
	for i, c := range s.Cues {
		source, ok := sources[c.SourceID]
		if !ok {
			return Missing("cue source", c.SourceID)
		}
		if c.StartMS < 0 || c.EndMS <= c.StartMS || c.EndMS > source.Media.DurationMS {
			return Invalid("cue %d has invalid interval", i)
		}
		if !utf8.ValidString(c.Text) || strings.TrimSpace(c.Text) == "" || len(c.Text) > 2000 || strings.Contains(c.Text, "-->") || strings.ContainsAny(c.Text, "\x00\r") {
			return Invalid("cue %d has invalid text", i)
		}
	}
	if len(s.Events) > 100 {
		return Invalid("sheet exceeds 100 events")
	}
	seen := map[string]bool{}
	for i, e := range s.Events {
		if err := MarkerID(e.ID); err != nil {
			return Invalid("event %d: %s", i, err)
		}
		if seen[e.ID] {
			return Invalid("event %d duplicates an event id", i)
		}
		seen[e.ID] = true
		source, ok := sources[e.SourceID]
		if !ok {
			return Missing("event source", e.SourceID)
		}
		if e.AtMS < 0 || e.AtMS > source.Media.DurationMS {
			return Invalid("event %d is outside source duration", i)
		}
	}
	return nil
}

// MarkerID constrains event and anchor identifiers so they can be quoted
// safely inside delivered WebVTT NOTE blocks.
func MarkerID(id string) error {
	if len(id) < 1 || len(id) > 64 || strings.Contains(id, "-->") || strings.ContainsAny(id, "\x00\r\n") {
		return fmt.Errorf("ids must be 1..64 bytes without markup or line breaks")
	}
	return nil
}
func ValidateVoiceTrack(t VoiceTrack, sources map[string]Source) error {
	if strings.TrimSpace(t.Name) == "" || len(t.Name) > 160 {
		return Invalid("voice track name must contain 1..160 bytes")
	}
	if len(t.Takes) == 0 || len(t.Takes) > 32 {
		return Invalid("a voice track requires 1..32 takes")
	}
	for i, take := range t.Takes {
		source, ok := sources[take.SourceID]
		if !ok {
			return Missing("voice source", take.SourceID)
		}
		if !source.Media.Audio {
			return Invalid("take %d requires a source with audio", i)
		}
		if take.InMS < 0 || take.OutMS <= take.InMS || take.OutMS > source.Media.DurationMS {
			return Invalid("take %d is outside source duration", i)
		}
		if take.OutMS-take.InMS < 100 {
			return Invalid("take %d is shorter than 100ms", i)
		}
		if math.IsNaN(take.GainDB) || math.IsInf(take.GainDB, 0) || take.GainDB < -60 || take.GainDB > 12 {
			return Invalid("take %d has invalid gain", i)
		}
		if len(take.Anchors) > 8 {
			return Invalid("take %d exceeds 8 anchors", i)
		}
		seen := map[string]bool{}
		for k, a := range take.Anchors {
			if err := MarkerID(a.ID); err != nil {
				return Invalid("take %d anchor %d: %s", i, k, err)
			}
			if seen[a.ID] {
				return Invalid("take %d duplicates an anchor id", i)
			}
			seen[a.ID] = true
			if err := MarkerID(a.EventID); err != nil {
				return Invalid("take %d anchor %d event: %s", i, k, err)
			}
			if a.CueMS < take.InMS || a.CueMS > take.OutMS {
				return Invalid("take %d anchor %d cue is outside the take", i, k)
			}
		}
	}
	return nil
}
func ValidateState(s State) error {
	if s.Schema != 1 || s.Sources == nil || s.Sheets == nil || s.Recipes == nil || s.VoiceTracks == nil || s.Renders == nil {
		return fmt.Errorf("unsupported or incomplete state schema")
	}
	for id, source := range s.Sources {
		if source.ID != id || len(source.Digest) != 64 || source.Bytes <= 0 {
			return fmt.Errorf("invalid source %s", id)
		}
	}
	for id, versions := range s.Sheets {
		if len(versions) == 0 {
			return fmt.Errorf("empty sheet history")
		}
		for i, v := range versions {
			if v.ID != id || v.Revision != i+1 {
				return fmt.Errorf("invalid sheet history")
			}
			if err := ValidateSheet(v, s.Sources); err != nil {
				return err
			}
		}
	}
	for id, versions := range s.Recipes {
		if len(versions) == 0 {
			return fmt.Errorf("empty recipe history")
		}
		for i, v := range versions {
			if v.ID != id || v.Revision != i+1 {
				return fmt.Errorf("invalid recipe history")
			}
			if err := ValidateRecipe(v); err != nil {
				return err
			}
		}
	}
	for id, versions := range s.VoiceTracks {
		if len(versions) == 0 {
			return fmt.Errorf("empty voice track history")
		}
		for i, v := range versions {
			if v.ID != id || v.Revision != i+1 {
				return fmt.Errorf("invalid voice track history")
			}
			if err := ValidateVoiceTrack(v, s.Sources); err != nil {
				return err
			}
		}
	}
	keys := map[string]bool{}
	for id, r := range s.Renders {
		if id != r.ID || keys[r.Key] {
			return fmt.Errorf("invalid render identity")
		}
		keys[r.Key] = true
		switch r.State {
		case "queued", "running", "succeeded", "failed", "cancelled":
		default:
			return fmt.Errorf("invalid render state")
		}
		if (r.State == "succeeded") != (r.Artifact != nil) {
			return fmt.Errorf("artifact state mismatch")
		}
		if r.Sources == nil {
			return fmt.Errorf("missing render source snapshot")
		}
		if err := ValidateSheet(r.Sheet, r.Sources); err != nil {
			return err
		}
		if err := ValidateRecipe(r.Recipe); err != nil {
			return err
		}
		if (r.Plan.Voice == nil) != (r.VoiceTrack == nil) {
			return fmt.Errorf("voice snapshot mismatch")
		}
		if r.VoiceTrack != nil {
			if r.Plan.Voice.TrackID != r.VoiceTrack.ID || r.Plan.Voice.Revision != r.VoiceTrack.Revision {
				return fmt.Errorf("frozen voice revision mismatch")
			}
			if len(r.Plan.Voice.Placements) != len(r.VoiceTrack.Takes) {
				return fmt.Errorf("voice placement mismatch")
			}
			if err := ValidateVoiceTrack(*r.VoiceTrack, r.Sources); err != nil {
				return err
			}
		}
	}
	var previous uint64
	for _, e := range s.Events {
		if e.Sequence <= previous || e.Sequence > s.Sequence {
			return fmt.Errorf("event sequence is invalid")
		}
		previous = e.Sequence
	}
	return nil
}
