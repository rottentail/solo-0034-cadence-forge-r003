package encoding

import (
	"cadenceforge/internal/domain"
	"cadenceforge/internal/media"
	"fmt"
	"strings"
)

func seconds(ms int64) string { return fmt.Sprintf("%.3f", float64(ms)/1000) }
func Arguments(render domain.Render, paths map[string]string, output string) ([]string, error) {
	r := render.Recipe
	args := []string{"-nostdin", "-y", "-hide_banner", "-loglevel", "error", "-filter_complex_threads", "1"}
	filters := []string{}
	joins := []string{}
	for i, p := range render.Plan.Placements {
		path, ok := paths[p.SourceID]
		if !ok {
			return nil, fmt.Errorf("source object missing")
		}
		args = append(args, "-protocol_whitelist", "file,pipe", "-format_whitelist", media.Formats, "-i", path)
		audio := fmt.Sprintf("[%d:a:0]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS,aresample=%d,aformat=sample_fmts=fltp:channel_layouts=stereo,volume=%.3fdB[a%d]", i, seconds(p.SourceInMS), seconds(p.SourceOutMS), r.SampleRate, p.GainDB, i)
		if !render.Sources[p.SourceID].Media.Audio {
			audio = fmt.Sprintf("anullsrc=r=%d:cl=stereo,atrim=duration=%s,asetpts=PTS-STARTPTS[a%d]", r.SampleRate, seconds(p.OutputOutMS-p.OutputInMS), i)
		}
		filters = append(filters, audio)
		if r.Format == "mp4" {
			filters = append(filters, fmt.Sprintf("[%d:v:0]trim=start=%s:end=%s,setpts=PTS-STARTPTS,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%d,format=yuv420p[v%d]", i, seconds(p.SourceInMS), seconds(p.SourceOutMS), r.Width, r.Height, r.Width, r.Height, r.FPS, i))
			joins = append(joins, fmt.Sprintf("[v%d][a%d]", i, i))
		} else {
			joins = append(joins, fmt.Sprintf("[a%d]", i))
		}
	}
	if len(joins) == 0 {
		return nil, fmt.Errorf("empty render")
	}
	voiceLabels := []string{}
	if render.Plan.Voice != nil {
		for j, vp := range render.Plan.Voice.Placements {
			path, ok := paths[vp.SourceID]
			if !ok {
				return nil, fmt.Errorf("voice source object missing")
			}
			args = append(args, "-protocol_whitelist", "file,pipe", "-format_whitelist", media.Formats, "-i", path)
			input := len(render.Plan.Placements) + j
			// Takes play at natural rate: trim, gain, then delay to the
			// resolved output position. No time-stretching anywhere.
			filters = append(filters, fmt.Sprintf("[%d:a:0]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS,aresample=%d,aformat=sample_fmts=fltp:channel_layouts=stereo,volume=%.3fdB,adelay=delays=%d:all=1[vt%d]", input, seconds(vp.SourceInMS), seconds(vp.SourceOutMS), r.SampleRate, vp.GainDB, vp.OutputInMS, j))
			voiceLabels = append(voiceLabels, fmt.Sprintf("[vt%d]", j))
		}
	}
	pictureMS := int64(0)
	for _, p := range render.Plan.Placements {
		pictureMS = max(pictureMS, p.OutputOutMS)
	}
	extension := render.Plan.DurationMS - pictureMS
	if r.Format == "mp4" {
		label := "vout"
		if extension > 0 {
			label = "vcat"
		}
		filters = append(filters, strings.Join(joins, "")+fmt.Sprintf("concat=n=%d:v=1:a=1[%s][acat]", len(joins), label))
		if extension > 0 {
			// Trailing voice beyond the picture holds the last frame rather
			// than cutting the narration or inventing new picture content.
			filters = append(filters, fmt.Sprintf("[vcat]tpad=stop_mode=clone:stop_duration=%s,fps=%d,format=yuv420p[vout]", seconds(extension), r.FPS))
		}
	} else {
		filters = append(filters, strings.Join(joins, "")+fmt.Sprintf("concat=n=%d:v=0:a=1[acat]", len(joins)))
	}
	mixed := "acat"
	if len(voiceLabels) > 0 {
		filters = append(filters, "[acat]"+strings.Join(voiceLabels, "")+fmt.Sprintf("amix=inputs=%d:duration=longest:normalize=0,alimiter=limit=0.891251:level=false[amixed]", len(voiceLabels)+1))
		mixed = "amixed"
	}
	if r.Normalize {
		filters = append(filters, fmt.Sprintf("[%s]loudnorm=I=%.2f:TP=-1.5:LRA=11,aresample=%d[aout]", mixed, r.TargetLUFS, r.SampleRate))
	} else {
		filters = append(filters, fmt.Sprintf("[%s]anull[aout]", mixed))
	}
	args = append(args, "-filter_complex", strings.Join(filters, ";"))
	if r.Format == "mp4" {
		args = append(args, "-map", "[vout]", "-map", "[aout]", "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-threads", "1", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart")
	} else {
		args = append(args, "-map", "[aout]", "-c:a", "pcm_s16le")
	}
	return append(args, "-ar", fmt.Sprint(r.SampleRate), "-t", seconds(render.Plan.DurationMS), "-map_metadata", "-1", output), nil
}
