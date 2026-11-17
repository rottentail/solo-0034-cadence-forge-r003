package service

import (
	"cadenceforge/internal/domain"
	"time"
)

// SaveVoiceTrack appends an immutable voice track revision. Voice edits are
// saved independently of picture edits; a render later freezes one revision
// of each against the same composition plan.
func (s *Service) SaveVoiceTrack(id string, expected int, input domain.VoiceTrack) (domain.VoiceTrack, error) {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	if id == "" {
		id = ID()
	}
	var out domain.VoiceTrack
	err := s.Store.Update(func(st *domain.State) error {
		versions := st.VoiceTracks[id]
		if len(versions) != expected {
			return domain.Conflict("voice track revision changed")
		}
		if expected > 0 && len(versions) == 0 {
			return domain.Missing("voice track", id)
		}
		if err := domain.ValidateVoiceTrack(input, st.Sources); err != nil {
			return err
		}
		input.ID = id
		input.Revision = len(versions) + 1
		input.CreatedAt = time.Now().UTC()
		out = input
		st.VoiceTracks[id] = append(versions, input)
		st.Emit(id, "voice.revised", "immutable voice track revision saved")
		return nil
	})
	return out, err
}
