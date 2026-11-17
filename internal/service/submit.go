package service

import (
	"cadenceforge/internal/caption"
	"cadenceforge/internal/domain"
	"cadenceforge/internal/timeline"
	"cadenceforge/internal/voice"
	"strings"
	"time"
)

type Request struct {
	SheetID        string `json:"sheet_id"`
	SheetRevision  int    `json:"sheet_revision"`
	RecipeID       string `json:"recipe_id"`
	RecipeRevision int    `json:"recipe_revision"`
	VoiceTrackID   string `json:"voice_track_id,omitempty"`
	VoiceRevision  int    `json:"voice_revision,omitempty"`
}

func (s *Service) Submit(key string, request Request) (domain.Render, error) {
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return domain.Render{}, domain.Invalid("Idempotency-Key requires 1..128 bytes")
	}
	if request.SheetRevision < 1 || request.RecipeRevision < 1 {
		return domain.Render{}, domain.Invalid("explicit positive revisions are required")
	}
	if request.VoiceTrackID == "" && request.VoiceRevision != 0 {
		return domain.Render{}, domain.Invalid("voice delivery requires both voice_track_id and voice_revision")
	}
	if request.VoiceTrackID != "" && request.VoiceRevision < 1 {
		return domain.Render{}, domain.Invalid("voice delivery requires a positive voice_revision")
	}
	s.mutation.Lock()
	defer s.mutation.Unlock()
	var out domain.Render
	err := s.Store.Update(func(st *domain.State) error {
		fingerprint := hash(request)
		for _, r := range st.Renders {
			if r.Key == key {
				if r.RequestHash != fingerprint {
					return domain.Conflict("idempotency key has different input")
				}
				out = r
				return nil
			}
		}
		pending := 0
		for _, r := range st.Renders {
			if r.State == "queued" || r.State == "running" {
				pending++
			}
		}
		if pending >= s.Config.QueueLimit {
			return domain.Capacity("render queue is full")
		}
		sheets := st.Sheets[request.SheetID]
		recipes := st.Recipes[request.RecipeID]
		if request.SheetRevision > len(sheets) {
			return domain.Missing("sheet revision", request.SheetID)
		}
		if request.RecipeRevision > len(recipes) {
			return domain.Missing("recipe revision", request.RecipeID)
		}
		sheet := sheets[request.SheetRevision-1]
		recipe := recipes[request.RecipeRevision-1]
		sources := map[string]domain.Source{}
		for _, p := range sheet.Spans {
			sources[p.SourceID] = st.Sources[p.SourceID]
		}
		for _, c := range sheet.Cues {
			sources[c.SourceID] = st.Sources[c.SourceID]
		}
		for _, e := range sheet.Events {
			sources[e.SourceID] = st.Sources[e.SourceID]
		}
		var track *domain.VoiceTrack
		if request.VoiceTrackID != "" {
			versions := st.VoiceTracks[request.VoiceTrackID]
			if request.VoiceRevision > len(versions) {
				return domain.Missing("voice track revision", request.VoiceTrackID)
			}
			frozen := versions[request.VoiceRevision-1]
			track = &frozen
			for _, take := range frozen.Takes {
				sources[take.SourceID] = st.Sources[take.SourceID]
			}
		}
		plan, err := timeline.Build(sheet, recipe, sources)
		if err != nil {
			return err
		}
		if track != nil {
			resolution := voice.Resolve(plan, *track)
			plan.DurationMS = resolution.DurationMS
			plan.Voice = &resolution
		}
		plan.VTT = caption.VTT(caption.Map(sheet, plan))
		if plan.Voice != nil {
			plan.VTT = voice.WithNotes(plan.VTT, *plan.Voice)
		}
		now := time.Now().UTC()
		id := ID()
		out = domain.Render{ID: id, Key: key, RequestHash: fingerprint, Sheet: sheet, Recipe: recipe, VoiceTrack: track, Sources: sources, Plan: plan, State: "queued", Attempts: []domain.Attempt{}, CreatedAt: now, UpdatedAt: now}
		st.Renders[id] = out
		detail := "frozen source and recipe revisions"
		if track != nil {
			detail = "frozen source, recipe and voice track revisions"
		}
		st.Emit(id, "render.queued", detail)
		return nil
	})
	if err == nil {
		s.Notify()
	}
	return out, err
}
