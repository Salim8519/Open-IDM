package stream

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// The subset of the DASH MPD schema we read. encoding/xml matches local names, so the
// urn:mpeg:dash:schema:mpd:2011 namespace needs no special handling.
type mpdXML struct {
	Type     string      `xml:"type,attr"`
	Duration string      `xml:"mediaPresentationDuration,attr"`
	BaseURL  []string    `xml:"BaseURL"`
	Periods  []periodXML `xml:"Period"`
}

type periodXML struct {
	Duration string     `xml:"duration,attr"`
	BaseURL  []string   `xml:"BaseURL"`
	Sets     []adaptXML `xml:"AdaptationSet"`
}

type adaptXML struct {
	MimeType    string       `xml:"mimeType,attr"`
	ContentType string       `xml:"contentType,attr"`
	Codecs      string       `xml:"codecs,attr"`
	BaseURL     []string     `xml:"BaseURL"`
	Template    *templateXML `xml:"SegmentTemplate"`
	List        *listXML     `xml:"SegmentList"`
	Base        *baseXML     `xml:"SegmentBase"`
	Protection  []struct{}   `xml:"ContentProtection"`
	Reps        []repXML     `xml:"Representation"`
}

type repXML struct {
	ID         string       `xml:"id,attr"`
	Bandwidth  int64        `xml:"bandwidth,attr"`
	Width      int          `xml:"width,attr"`
	Height     int          `xml:"height,attr"`
	MimeType   string       `xml:"mimeType,attr"`
	Codecs     string       `xml:"codecs,attr"`
	BaseURL    []string     `xml:"BaseURL"`
	Template   *templateXML `xml:"SegmentTemplate"`
	List       *listXML     `xml:"SegmentList"`
	Base       *baseXML     `xml:"SegmentBase"`
	Protection []struct{}   `xml:"ContentProtection"`
}

type templateXML struct {
	Media          string       `xml:"media,attr"`
	Initialization string       `xml:"initialization,attr"`
	StartNumber    *int64       `xml:"startNumber,attr"`
	Timescale      *int64       `xml:"timescale,attr"`
	Duration       *int64       `xml:"duration,attr"`
	PTO            *int64       `xml:"presentationTimeOffset,attr"`
	Timeline       *timelineXML `xml:"SegmentTimeline"`
}

type timelineXML struct {
	S []struct {
		T *int64 `xml:"t,attr"`
		D int64  `xml:"d,attr"`
		R int64  `xml:"r,attr"`
	} `xml:"S"`
}

type listXML struct {
	Init *urlXML `xml:"Initialization"`
	URLs []struct {
		Media string `xml:"media,attr"`
		Range string `xml:"mediaRange,attr"`
	} `xml:"SegmentURL"`
}

type baseXML struct {
	Init *urlXML `xml:"Initialization"`
}

type urlXML struct {
	SourceURL string `xml:"sourceURL,attr"`
	Range     string `xml:"range,attr"`
}

// track is one representation chosen for download.
type track struct {
	ID        string
	Kind      string // "video" or "audio"
	Bandwidth int64
	Height    int
	Ext       string // ".mp4", ".m4a", ".webm" (output when it is the only track)
	Duration  float64

	segments []segment // nil with single != "" (fetched in byte-range chunks)
	single   string    // URL of a whole-file representation (SegmentBase / BaseURL only)
}

// parseMPD returns the best video and best audio track of the first period.
func parseMPD(data []byte, base *url.URL) (video, audio *track, err error) {
	var m mpdXML
	dec := xml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m); err != nil {
		return nil, nil, fmt.Errorf("parse MPD: %w", err)
	}
	if m.Type == "dynamic" {
		return nil, nil, errors.New("live DASH streams are not supported yet")
	}
	if len(m.Periods) == 0 {
		return nil, nil, errors.New("MPD has no Period")
	}
	b, err := withBase(base, m.BaseURL)
	if err != nil {
		return nil, nil, err
	}
	p := m.Periods[0] // multi-period (e.g. ad breaks): only the first period is downloaded
	if b, err = withBase(b, p.BaseURL); err != nil {
		return nil, nil, err
	}
	dur, err := isoDuration(p.Duration)
	if err != nil || dur == 0 {
		if dur, err = isoDuration(m.Duration); err != nil {
			return nil, nil, err
		}
	}
	type cand struct {
		set  *adaptXML
		rep  *repXML
		kind string
	}
	var bestV, bestA *cand
	for i := range p.Sets {
		set := &p.Sets[i]
		for j := range set.Reps {
			rep := &set.Reps[j]
			kind := mediaKind(set, rep)
			c := &cand{set, rep, kind}
			switch kind {
			case "video":
				if bestV == nil || rep.Bandwidth > bestV.rep.Bandwidth ||
					(rep.Bandwidth == bestV.rep.Bandwidth && rep.Height > bestV.rep.Height) {
					bestV = c
				}
			case "audio":
				if bestA == nil || rep.Bandwidth > bestA.rep.Bandwidth {
					bestA = c
				}
			}
		}
	}
	if bestV == nil && bestA == nil {
		return nil, nil, errors.New("MPD has no video or audio representation")
	}
	build := func(c *cand) (*track, error) {
		if c == nil {
			return nil, nil
		}
		if len(c.set.Protection) > 0 || len(c.rep.Protection) > 0 {
			return nil, errors.New("this DASH stream is DRM-protected and cannot be downloaded")
		}
		return buildTrack(b, dur, c.set, c.rep, c.kind)
	}
	if video, err = build(bestV); err != nil {
		return nil, nil, err
	}
	if audio, err = build(bestA); err != nil {
		return nil, nil, err
	}
	return video, audio, nil
}

func withBase(base *url.URL, refs []string) (*url.URL, error) {
	if len(refs) == 0 || strings.TrimSpace(refs[0]) == "" {
		return base, nil
	}
	return resolve(base, refs[0])
}

func mediaKind(set *adaptXML, rep *repXML) string {
	mt := rep.MimeType
	if mt == "" {
		mt = set.MimeType
	}
	switch {
	case strings.HasPrefix(mt, "video/"), set.ContentType == "video":
		return "video"
	case strings.HasPrefix(mt, "audio/"), set.ContentType == "audio":
		return "audio"
	case rep.Width > 0 || rep.Height > 0:
		return "video"
	}
	codecs := rep.Codecs + set.Codecs
	for _, c := range []string{"mp4a", "opus", "ac-3", "ec-3", "vorbis", "flac"} {
		if strings.HasPrefix(codecs, c) {
			return "audio"
		}
	}
	return ""
}

func buildTrack(base *url.URL, dur float64, set *adaptXML, rep *repXML, kind string) (*track, error) {
	t := &track{ID: rep.ID, Kind: kind, Bandwidth: rep.Bandwidth, Height: rep.Height, Duration: dur}
	mt := rep.MimeType
	if mt == "" {
		mt = set.MimeType
	}
	switch {
	case strings.HasSuffix(mt, "/webm"):
		t.Ext = ".webm"
	case kind == "audio":
		t.Ext = ".m4a"
	default:
		t.Ext = ".mp4"
	}
	b, err := withBase(base, set.BaseURL)
	if err != nil {
		return nil, err
	}
	if b, err = withBase(b, rep.BaseURL); err != nil {
		return nil, err
	}
	tmpl := mergeTemplate(set.Template, rep.Template)
	list := rep.List
	if list == nil {
		list = set.List
	}
	switch {
	case tmpl != nil && tmpl.Media != "":
		t.segments, err = templateSegments(b, dur, tmpl, rep)
	case list != nil:
		t.segments, err = listSegments(b, list)
	default:
		if len(rep.BaseURL) == 0 && len(set.BaseURL) == 0 {
			return nil, fmt.Errorf("representation %q has no segments", rep.ID)
		}
		t.single = b.String()
	}
	if err != nil {
		return nil, err
	}
	if t.single == "" && len(t.segments) == 0 {
		return nil, fmt.Errorf("representation %q has no segments", rep.ID)
	}
	return t, nil
}

func mergeTemplate(set, rep *templateXML) *templateXML {
	if set == nil {
		return rep
	}
	if rep == nil {
		return set
	}
	m := *set
	if rep.Media != "" {
		m.Media = rep.Media
	}
	if rep.Initialization != "" {
		m.Initialization = rep.Initialization
	}
	if rep.StartNumber != nil {
		m.StartNumber = rep.StartNumber
	}
	if rep.Timescale != nil {
		m.Timescale = rep.Timescale
	}
	if rep.Duration != nil {
		m.Duration = rep.Duration
	}
	if rep.PTO != nil {
		m.PTO = rep.PTO
	}
	if rep.Timeline != nil {
		m.Timeline = rep.Timeline
	}
	return &m
}

func templateSegments(base *url.URL, dur float64, t *templateXML, rep *repXML) ([]segment, error) {
	num := int64(1)
	if t.StartNumber != nil {
		num = *t.StartNumber
	}
	scale := int64(1)
	if t.Timescale != nil && *t.Timescale > 0 {
		scale = *t.Timescale
	}
	var initRes *resource
	if t.Initialization != "" {
		u, err := resolve(base, expandTemplate(t.Initialization, rep, 0, 0))
		if err != nil {
			return nil, err
		}
		initRes = &resource{URL: u.String(), Length: -1}
	}
	var segs []segment
	add := func(number, time, d int64) error {
		if len(segs) >= maxSegments {
			return fmt.Errorf("representation has more than %d segments", maxSegments)
		}
		u, err := resolve(base, expandTemplate(t.Media, rep, number, time))
		if err != nil {
			return err
		}
		segs = append(segs, segment{Seq: int64(len(segs)), Res: resource{URL: u.String(), Length: -1},
			Init: initRes, Duration: float64(d) / float64(scale)})
		return nil
	}
	if t.Timeline != nil {
		pto := int64(0)
		if t.PTO != nil {
			pto = *t.PTO
		}
		end := pto + int64(dur*float64(scale)) // period end in timescale units, for r = -1
		var tm int64
		for i, s := range t.Timeline.S {
			if s.T != nil {
				tm = *s.T
			}
			if s.D <= 0 {
				return nil, errors.New("bad SegmentTimeline duration")
			}
			repeat := s.R
			if repeat < 0 {
				next := end
				if i+1 < len(t.Timeline.S) && t.Timeline.S[i+1].T != nil {
					next = *t.Timeline.S[i+1].T
				}
				repeat = int64(math.Ceil(float64(next-tm)/float64(s.D))) - 1
			}
			if repeat > maxSegments {
				return nil, fmt.Errorf("representation has more than %d segments", maxSegments)
			}
			for k := int64(0); k <= repeat; k++ {
				if err := add(num, tm, s.D); err != nil {
					return nil, err
				}
				num++
				tm += s.D
			}
		}
		return segs, nil
	}
	if t.Duration == nil || *t.Duration <= 0 {
		return nil, errors.New("SegmentTemplate without duration or SegmentTimeline")
	}
	if dur <= 0 {
		return nil, errors.New("MPD without a duration")
	}
	d := *t.Duration
	count := int64(math.Ceil(dur * float64(scale) / float64(d)))
	if count > maxSegments {
		return nil, fmt.Errorf("representation has more than %d segments", maxSegments)
	}
	for k := int64(0); k < count; k++ {
		if err := add(num+k, k*d, d); err != nil {
			return nil, err
		}
	}
	return segs, nil
}

func listSegments(base *url.URL, l *listXML) ([]segment, error) {
	var initRes *resource
	if l.Init != nil {
		r, err := dashResource(base, l.Init.SourceURL, l.Init.Range)
		if err != nil {
			return nil, err
		}
		initRes = &r
	}
	if len(l.URLs) > maxSegments {
		return nil, fmt.Errorf("representation has more than %d segments", maxSegments)
	}
	segs := make([]segment, 0, len(l.URLs))
	for i, su := range l.URLs {
		r, err := dashResource(base, su.Media, su.Range)
		if err != nil {
			return nil, err
		}
		segs = append(segs, segment{Seq: int64(i), Res: r, Init: initRes})
	}
	return segs, nil
}

// dashResource resolves a sourceURL/media (empty = the base URL) with an "a-b" range.
func dashResource(base *url.URL, ref, rng string) (resource, error) {
	u := base
	if ref != "" {
		var err error
		if u, err = resolve(base, ref); err != nil {
			return resource{}, err
		}
	}
	r := resource{URL: u.String(), Length: -1}
	if rng != "" {
		a, b, ok := strings.Cut(rng, "-")
		from, err1 := strconv.ParseInt(a, 10, 64)
		to, err2 := strconv.ParseInt(b, 10, 64)
		if !ok || err1 != nil || err2 != nil || from < 0 || to < from || to-from >= maxSegmentBytes {
			return resource{}, errors.New("bad byte range in MPD")
		}
		r.Offset, r.Length = from, to-from+1
	}
	return r, nil
}

// expandTemplate substitutes $RepresentationID$, $Number$, $Time$, $Bandwidth$ (with an
// optional %0Nd width) and $$.
func expandTemplate(s string, rep *repXML, number, time int64) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '$')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		s = s[i+1:]
		j := strings.IndexByte(s, '$')
		if j < 0 { // unmatched: keep literally
			b.WriteByte('$')
			b.WriteString(s)
			return b.String()
		}
		id, format := s[:j], ""
		s = s[j+1:]
		if k := strings.IndexByte(id, '%'); k >= 0 {
			id, format = id[:k], id[k:]
		}
		width := 0
		if format != "" {
			f := strings.TrimSuffix(strings.TrimPrefix(format, "%0"), "d")
			if w, err := strconv.Atoi(f); err == nil && w > 0 && w < 32 {
				width = w
			}
		}
		pad := func(n int64) string {
			v := strconv.FormatInt(n, 10)
			for len(v) < width {
				v = "0" + v
			}
			return v
		}
		switch id {
		case "":
			b.WriteByte('$')
		case "RepresentationID":
			b.WriteString(rep.ID)
		case "Number":
			b.WriteString(pad(number))
		case "Time":
			b.WriteString(pad(time))
		case "Bandwidth":
			b.WriteString(pad(rep.Bandwidth))
		default:
			b.WriteString("$" + id + format + "$")
		}
	}
}

// isoDuration parses an xs:duration such as PT1H2M3.5S or P1DT2H ("" = 0).
func isoDuration(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	s = s[1:]
	total, inTime := 0.0, false
	for len(s) > 0 {
		if s[0] == 'T' {
			inTime, s = true, s[1:]
			continue
		}
		i := strings.IndexAny(s, "YMWDHS")
		if i <= 0 {
			return 0, fmt.Errorf("bad duration %q", s)
		}
		v, err := strconv.ParseFloat(s[:i], 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("bad duration %q", s)
		}
		switch {
		case s[i] == 'D' && !inTime:
			total += v * 86400
		case s[i] == 'W' && !inTime:
			total += v * 7 * 86400
		case s[i] == 'H' && inTime:
			total += v * 3600
		case s[i] == 'M' && inTime:
			total += v * 60
		case s[i] == 'S' && inTime:
			total += v
		default:
			return 0, fmt.Errorf("unsupported duration %q", s)
		}
		s = s[i+1:]
	}
	return total, nil
}
