// Package voice resolves a voice track against a picture plan. The resolver
// never stretches audio and never pins takes to absolute output time:
// anchored takes follow their picture events, unanchored takes flow after
// their predecessor, and every compromise the picture edit forces is
// reported per anchor and per take so reviewers can see the trade-offs.
package voice

import (
	"cadenceforge/internal/domain"
	"fmt"
	"strings"
)

// Resolve places each take at natural rate. The first anchor whose event is
// still covered by the picture places the take; remaining anchors keep
// source-relative offsets and their drift is reported in milliseconds. A
// placement that would overlap the previous take or start before zero is
// shifted to the earliest free position and reported, not hidden.
func Resolve(plan domain.Plan, track domain.VoiceTrack) domain.VoiceResolution {
	events := map[string]int64{}
	for _, e := range plan.Events {
		if e.Covered {
			events[e.ID] = e.OutputMS
		}
	}
	res := domain.VoiceResolution{TrackID: track.ID, Revision: track.Revision, Placements: []domain.VoicePlacement{}, Takes: []domain.TakeOutcome{}}
	var cursor int64
	aligned, drifted, lost := 0, 0, 0
	anchored, shifted, flowed := 0, 0, 0
	for i, take := range track.Takes {
		duration := take.OutMS - take.InMS
		primary := -1
		var desired int64
		for k, a := range take.Anchors {
			if out, ok := events[a.EventID]; ok {
				primary = k
				desired = out - (a.CueMS - take.InMS)
				break
			}
		}
		at := cursor
		decision := "flowed"
		detail := "no covered anchor; take follows the previous take at natural rate"
		if primary >= 0 {
			decision = "anchored"
			detail = fmt.Sprintf("placed by anchor %q on event %q", take.Anchors[primary].ID, take.Anchors[primary].EventID)
			at = desired
			switch {
			case at < 0:
				at = 0
				decision = "shifted"
				detail = fmt.Sprintf("anchor %q would start before the composition; clamped to zero", take.Anchors[primary].ID)
			case at < cursor:
				at = cursor
				decision = "shifted"
				detail = fmt.Sprintf("anchor %q placement overlaps the previous take; shifted to its end", take.Anchors[primary].ID)
			}
		}
		placement := domain.VoicePlacement{SourceID: take.SourceID, SourceInMS: take.InMS, SourceOutMS: take.OutMS, OutputInMS: at, OutputOutMS: at + duration, GainDB: take.GainDB}
		outcome := domain.TakeOutcome{TakeIndex: i, Decision: decision, Detail: detail, Placement: placement, Anchors: []domain.AnchorOutcome{}}
		for _, a := range take.Anchors {
			keyword := at + (a.CueMS - take.InMS)
			ao := domain.AnchorOutcome{TakeIndex: i, AnchorID: a.ID, EventID: a.EventID, KeywordMS: keyword}
			if out, ok := events[a.EventID]; ok {
				eventMS := out
				ao.EventMS = &eventMS
				ao.DriftMS = keyword - out
				if ao.DriftMS == 0 {
					ao.Status = "aligned"
					aligned++
				} else {
					ao.Status = "drifted"
					drifted++
				}
			} else {
				ao.Status = "lost"
				lost++
			}
			outcome.Anchors = append(outcome.Anchors, ao)
		}
		switch decision {
		case "anchored":
			anchored++
		case "shifted":
			shifted++
		default:
			flowed++
		}
		res.Placements = append(res.Placements, placement)
		res.Takes = append(res.Takes, outcome)
		cursor = at + duration
	}
	res.DurationMS = max(plan.DurationMS, cursor)
	res.Conflicts = drifted + lost + shifted
	res.Summary = fmt.Sprintf("voice track %s revision %d: %d takes (%d anchored, %d shifted, %d flowed); anchors %d aligned, %d drifted, %d lost",
		track.ID, track.Revision, len(track.Takes), anchored, shifted, flowed, aligned, drifted, lost)
	if extra := res.DurationMS - plan.DurationMS; extra > 0 {
		res.Summary += fmt.Sprintf("; composition extended %dms beyond the picture for trailing voice", extra)
	}
	if res.Conflicts == 0 {
		res.Summary += "; no conflicts"
	}
	return res
}

// WithNotes prepends WebVTT NOTE blocks describing the adopted voice
// relationship, so delivered captions explain the trade-offs next to the
// cues. Marker ids are validated free of markup and line breaks.
func WithNotes(vtt string, res domain.VoiceResolution) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\nNOTE cadence-forge voice track ")
	b.WriteString(res.TrackID)
	fmt.Fprintf(&b, " revision %d\n", res.Revision)
	b.WriteString("NOTE " + res.Summary + "\n")
	for _, t := range res.Takes {
		fmt.Fprintf(&b, "NOTE take %d: %s at %dms", t.TakeIndex+1, t.Decision, t.Placement.OutputInMS)
		for _, a := range t.Anchors {
			if a.Status == "aligned" {
				fmt.Fprintf(&b, "; anchor %s aligned to event %s at %dms", a.AnchorID, a.EventID, a.KeywordMS)
			} else if a.Status == "drifted" {
				fmt.Fprintf(&b, "; anchor %s drift %+dms from event %s", a.AnchorID, a.DriftMS, a.EventID)
			} else {
				fmt.Fprintf(&b, "; anchor %s lost: event %s not covered by the picture", a.AnchorID, a.EventID)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String() + strings.TrimPrefix(vtt, "WEBVTT\n\n")
}
