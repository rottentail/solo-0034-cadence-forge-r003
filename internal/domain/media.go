package domain

import "time"

type Media struct {
	DurationMS int64  `json:"duration_ms"`
	Video      bool   `json:"video"`
	Audio      bool   `json:"audio"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec"`
}
type Source struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Digest    string    `json:"digest"`
	Bytes     int64     `json:"bytes"`
	Media     Media     `json:"media"`
	CreatedAt time.Time `json:"created_at"`
}
type Span struct {
	SourceID string  `json:"source_id"`
	InMS     int64   `json:"in_ms"`
	OutMS    int64   `json:"out_ms"`
	GainDB   float64 `json:"gain_db"`
}
type Cue struct {
	SourceID string `json:"source_id"`
	StartMS  int64  `json:"start_ms"`
	EndMS    int64  `json:"end_ms"`
	Text     string `json:"text"`
}

// PictureEvent names a source moment that voice anchors can align to. Events
// are versioned with the sheet, so a picture re-edit moves or drops them.
type PictureEvent struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	AtMS     int64  `json:"at_ms"`
}
type Sheet struct {
	ID        string         `json:"id"`
	Revision  int            `json:"revision"`
	Name      string         `json:"name"`
	Spans     []Span         `json:"spans"`
	Cues      []Cue          `json:"cues"`
	Events    []PictureEvent `json:"events,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
type Recipe struct {
	ID         string  `json:"id"`
	Revision   int     `json:"revision"`
	Name       string  `json:"name"`
	Format     string  `json:"format"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        int     `json:"fps"`
	SampleRate int     `json:"sample_rate"`
	Normalize  bool    `json:"normalize"`
	TargetLUFS float64 `json:"target_lufs"`
}
type Placement struct {
	SourceID    string  `json:"source_id"`
	SourceInMS  int64   `json:"source_in_ms"`
	SourceOutMS int64   `json:"source_out_ms"`
	OutputInMS  int64   `json:"output_in_ms"`
	OutputOutMS int64   `json:"output_out_ms"`
	GainDB      float64 `json:"gain_db"`
}

// EventPlacement records where a picture event lands in the composition. With
// repeated source footage the first covering span wins, deterministically.
type EventPlacement struct {
	ID         string `json:"id"`
	SourceID   string `json:"source_id"`
	SourceAtMS int64  `json:"source_at_ms"`
	Covered    bool   `json:"covered"`
	OutputMS   int64  `json:"output_ms"`
}

// VoiceAnchor ties a keyword moment inside a voice take (CueMS, source time)
// to a picture event. Anchors never stretch audio and never pin takes to
// absolute output time; they express intent that Resolve honours or reports.
type VoiceAnchor struct {
	ID      string `json:"id"`
	EventID string `json:"event_id"`
	CueMS   int64  `json:"cue_ms"`
}
type VoiceTake struct {
	SourceID string        `json:"source_id"`
	InMS     int64         `json:"in_ms"`
	OutMS    int64         `json:"out_ms"`
	GainDB   float64       `json:"gain_db"`
	Anchors  []VoiceAnchor `json:"anchors"`
}

// VoiceTrack is an independently versioned voiceover edit. It is saved apart
// from the sheet, but a render always freezes one sheet revision and one
// voice track revision together against the same composition plan.
type VoiceTrack struct {
	ID        string      `json:"id"`
	Revision  int         `json:"revision"`
	Name      string      `json:"name"`
	Takes     []VoiceTake `json:"takes"`
	CreatedAt time.Time   `json:"created_at"`
}

// AnchorOutcome reports how one anchor survived the picture edit: aligned
// (drift 0), drifted (keyword lands DriftMS away from its event), or lost
// (the event is no longer covered by the picture).
type AnchorOutcome struct {
	TakeIndex int    `json:"take_index"`
	AnchorID  string `json:"anchor_id"`
	EventID   string `json:"event_id"`
	Status    string `json:"status"`
	KeywordMS int64  `json:"keyword_ms"`
	EventMS   *int64 `json:"event_ms,omitempty"`
	DriftMS   int64  `json:"drift_ms"`
}
type VoicePlacement struct {
	SourceID    string  `json:"source_id"`
	SourceInMS  int64   `json:"source_in_ms"`
	SourceOutMS int64   `json:"source_out_ms"`
	OutputInMS  int64   `json:"output_in_ms"`
	OutputOutMS int64   `json:"output_out_ms"`
	GainDB      float64 `json:"gain_db"`
}

// TakeOutcome explains one take's placement decision: anchored (primary
// anchor placed it), shifted (anchor placement clamped or moved to avoid
// overlapping the previous take), or flowed (no covered anchor, so it
// follows its predecessor at natural rate).
type TakeOutcome struct {
	TakeIndex int             `json:"take_index"`
	Decision  string          `json:"decision"`
	Detail    string          `json:"detail,omitempty"`
	Placement VoicePlacement  `json:"placement"`
	Anchors   []AnchorOutcome `json:"anchors"`
}

// VoiceResolution is the frozen, reviewable voiceover relationship adopted
// by a preview or a render. Conflicts counts every reported compromise
// (drifted or lost anchors, shifted takes); none of them is hidden by
// stretching audio or pinning takes to absolute time.
type VoiceResolution struct {
	TrackID    string           `json:"track_id"`
	Revision   int              `json:"revision"`
	DurationMS int64            `json:"duration_ms"`
	Placements []VoicePlacement `json:"placements"`
	Takes      []TakeOutcome    `json:"takes"`
	Conflicts  int              `json:"conflicts"`
	Summary    string           `json:"summary"`
}
type Plan struct {
	DurationMS int64            `json:"duration_ms"`
	Placements []Placement      `json:"placements"`
	Recipe     Recipe           `json:"recipe"`
	VTT        string           `json:"vtt"`
	Events     []EventPlacement `json:"events,omitempty"`
	Voice      *VoiceResolution `json:"voice,omitempty"`
}
