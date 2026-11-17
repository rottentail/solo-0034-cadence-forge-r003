package timeline

import (
	"cadenceforge/internal/domain"
)

func Build(sheet domain.Sheet, recipe domain.Recipe, sources map[string]domain.Source) (domain.Plan, error) {
	if err := domain.ValidateSheet(sheet, sources); err != nil {
		return domain.Plan{}, err
	}
	if err := domain.ValidateRecipe(recipe); err != nil {
		return domain.Plan{}, err
	}
	plan := domain.Plan{Recipe: recipe, Placements: []domain.Placement{}}
	for _, span := range sheet.Spans {
		source := sources[span.SourceID]
		if recipe.Format == "mp4" && !source.Media.Video {
			return plan, domain.Invalid("MP4 span requires video: %s", source.ID)
		}
		if recipe.Format == "wav" && !source.Media.Audio {
			return plan, domain.Invalid("WAV span requires audio: %s", source.ID)
		}
		duration := span.OutMS - span.InMS
		plan.Placements = append(plan.Placements, domain.Placement{SourceID: source.ID, SourceInMS: span.InMS, SourceOutMS: span.OutMS, OutputInMS: plan.DurationMS, OutputOutMS: plan.DurationMS + duration, GainDB: span.GainDB})
		plan.DurationMS += duration
	}
	for _, event := range sheet.Events {
		placement := domain.EventPlacement{ID: event.ID, SourceID: event.SourceID, SourceAtMS: event.AtMS}
		for _, p := range plan.Placements {
			if p.SourceID == event.SourceID && event.AtMS >= p.SourceInMS && event.AtMS < p.SourceOutMS {
				placement.Covered = true
				placement.OutputMS = p.OutputInMS + event.AtMS - p.SourceInMS
				break
			}
		}
		plan.Events = append(plan.Events, placement)
	}
	return plan, nil
}
