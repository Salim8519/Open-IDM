package stream

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

func (p *Protocol) plan(ctx context.Context, r *runner, sources []string, haveFFmpeg bool) (*plan, error) {
	if p.kind == "dash" {
		return planDASH(ctx, r, sources)
	}
	return planHLS(ctx, r, sources, haveFFmpeg)
}

// fetchPlaylist downloads and parses an M3U8.
func fetchPlaylist(ctx context.Context, c *client, rawURL string) (*playlist, error) {
	base, err := resolve(nil, rawURL)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, resource{URL: base.String(), Length: -1}, maxPlaylistBytes, "playlist", false)
	if err != nil {
		var tb *tooBigErr
		if errors.As(err, &tb) {
			return nil, fmt.Errorf("playlist is larger than %d MB", maxPlaylistBytes>>20)
		}
		return nil, err
	}
	return parseM3U8(body, base)
}

func planHLS(ctx context.Context, r *runner, sources []string, haveFFmpeg bool) (*plan, error) {
	media := sources
	var firstList *playlist
	estimate := int64(-1)
	var bandwidth int64
	if len(media) == 0 {
		pl, err := fetchPlaylist(ctx, r.c, r.j.URL)
		if err != nil {
			return nil, err
		}
		if pl.Master {
			v, audio := pickVariant(pl, !haveFFmpeg)
			media = []string{v.URL}
			if audio != nil {
				media = append(media, audio.URL)
			}
			bandwidth = v.Bandwidth
		} else {
			media, firstList = []string{r.j.URL}, pl
		}
	}
	out := &plan{ids: media, needFFmpeg: len(media) > 1}
	names := []string{"video", "audio"}
	for i, m := range media {
		pl := firstList
		if i > 0 || pl == nil {
			var err error
			if pl, err = fetchPlaylist(ctx, r.c, m); err != nil {
				return nil, err
			}
		}
		if pl.Master {
			return nil, errors.New("variant playlist is a master playlist")
		}
		if pl.Ended && len(pl.Segments) == 0 {
			return nil, errors.New("playlist has no segments")
		}
		live := !pl.Ended
		if i == 0 {
			out.live = live
			out.ext = hlsExt(pl.Segments)
			if bandwidth > 0 && !live {
				var dur float64
				for _, s := range pl.Segments {
					dur += s.Duration
				}
				estimate = int64(float64(bandwidth) / 8 * dur)
			}
		}
		mediaURL := m
		src := &source{name: names[min(i, 1)], live: live, estimate: -1,
			first: &list{segs: pl.Segments, ended: pl.Ended, target: pl.Target}}
		if i == 0 {
			src.estimate = estimate // the variant's bandwidth covers its audio too
		} else if estimate >= 0 {
			src.estimate = 0
		}
		src.reload = func(ctx context.Context) (*list, error) {
			pl, err := fetchPlaylist(ctx, r.c, mediaURL)
			if err != nil {
				return nil, err
			}
			return &list{segs: pl.Segments, ended: pl.Ended, target: pl.Target}, nil
		}
		out.sources = append(out.sources, src)
	}
	return out, nil
}

// hlsExt is the container of concatenated segments: fragmented MP4 (with an EXT-X-MAP
// or .m4s segments), packed audio, or MPEG-TS.
func hlsExt(segs []segment) string {
	if len(segs) == 0 {
		return ".ts"
	}
	ext := func(raw string) string {
		if u, err := url.Parse(raw); err == nil {
			return strings.ToLower(path.Ext(u.Path))
		}
		return ""
	}
	if segs[0].Init != nil && ext(segs[0].Init.URL) != ".ts" {
		return ".mp4"
	}
	switch ext(segs[0].Res.URL) {
	case ".m4s", ".mp4", ".m4v", ".cmfv", ".fmp4":
		return ".mp4"
	case ".aac":
		return ".aac"
	case ".m4a":
		return ".m4a"
	case ".mp3":
		return ".mp3"
	}
	return ".ts"
}

func planDASH(ctx context.Context, r *runner, sources []string) (*plan, error) {
	base, err := resolve(nil, r.j.URL)
	if err != nil {
		return nil, err
	}
	body, err := r.c.get(ctx, resource{URL: base.String(), Length: -1}, maxPlaylistBytes, "manifest", false)
	if err != nil {
		return nil, err
	}
	video, audio, err := parseMPD(body, base)
	if err != nil {
		return nil, err
	}
	var tracks []*track
	for _, t := range []*track{video, audio} {
		if t != nil {
			tracks = append(tracks, t)
		}
	}
	out := &plan{needFFmpeg: len(tracks) > 1, ext: tracks[0].Ext}
	for i, t := range tracks {
		id := t.Kind + ":" + t.ID
		if len(sources) > 0 && (i >= len(sources) || sources[i] != id) {
			return nil, errors.New("the stream changed since the download started; restart it")
		}
		out.ids = append(out.ids, id)
		segs := t.segments
		estimate := int64(-1)
		if t.Bandwidth > 0 && t.Duration > 0 {
			estimate = int64(float64(t.Bandwidth) / 8 * t.Duration)
		}
		if t.single != "" {
			n, ranged, err := r.c.size(ctx, t.single)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", t.Kind, err)
			}
			segs = chunked(t.single, n, ranged)
			if n > 0 {
				estimate = n
			}
		}
		out.sources = append(out.sources, &source{name: t.Kind, estimate: estimate,
			first: &list{segs: segs, ended: true}})
	}
	return out, nil
}

// chunked splits a whole-file representation into byte-range segments.
func chunked(rawURL string, size int64, ranged bool) []segment {
	if !ranged || size <= 0 {
		return []segment{{Res: resource{URL: rawURL, Length: -1}}}
	}
	var segs []segment
	for off := int64(0); off < size; off += rangeChunk {
		segs = append(segs, segment{Seq: int64(len(segs)), Res: resource{URL: rawURL, Offset: off, Length: min(rangeChunk, size-off)}})
	}
	return segs
}
