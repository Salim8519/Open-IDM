package stream

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// variant is one #EXT-X-STREAM-INF entry of a master playlist.
type variant struct {
	URL       string
	Bandwidth int64
	Width     int
	Height    int
	Audio     string // GROUP-ID of the audio renditions it uses ("" = none)
}

// rendition is one #EXT-X-MEDIA entry.
type rendition struct {
	Type, Group, Name string
	URL               string // "" = carried inside the variant stream
	Default           bool
	AutoSelect        bool
}

// keyInfo is the EXT-X-KEY in effect for a segment (nil = not encrypted).
type keyInfo struct {
	URL string
	IV  []byte // nil = derive from the media sequence number
}

// segment is one media segment to download, in order.
type segment struct {
	Seq      int64 // HLS media sequence number, DASH index
	Res      resource
	Key      *keyInfo
	Init     *resource // EXT-X-MAP / DASH initialization, written once before the first segment using it
	Duration float64   // seconds (0 if unknown)
}

// playlist is a parsed M3U8: a master (Variants) or a media playlist (Segments).
type playlist struct {
	Master     bool
	Variants   []variant
	Renditions []rendition

	Segments []segment
	Target   time.Duration
	Ended    bool // EXT-X-ENDLIST or PLAYLIST-TYPE:VOD
}

// parseM3U8 parses a playlist; every URI is resolved against base and must be http(s).
func parseM3U8(data []byte, base *url.URL) (*playlist, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), maxPlaylistBytes)
	first := true
	pl := &playlist{}
	var (
		seq        int64
		pendingInf *variant
		dur        float64
		key        *keyInfo
		initRes    *resource
		byteRange  *resource // pending EXT-X-BYTERANGE for the next URI
		lastEnd    = map[string]int64{}
	)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			if line != "#EXTM3U" {
				return nil, errors.New("not an HLS playlist (missing #EXTM3U)")
			}
			first = false
			continue
		}
		if !strings.HasPrefix(line, "#") { // a URI line
			u, err := resolve(base, line)
			if err != nil {
				return nil, err
			}
			if pendingInf != nil {
				pendingInf.URL = u.String()
				pl.Variants = append(pl.Variants, *pendingInf)
				pendingInf = nil
				continue
			}
			if len(pl.Segments) >= maxSegments {
				return nil, fmt.Errorf("playlist has more than %d segments", maxSegments)
			}
			res := resource{URL: u.String(), Length: -1}
			if byteRange != nil {
				res.Length = byteRange.Length
				res.Offset = byteRange.Offset
				if res.Offset < 0 { // continues after the previous sub-range of this URI
					res.Offset = lastEnd[res.URL]
				}
				lastEnd[res.URL] = res.Offset + res.Length
				byteRange = nil
			}
			pl.Segments = append(pl.Segments, segment{Seq: seq, Res: res, Key: key, Init: initRes, Duration: dur})
			seq++
			dur = 0
			continue
		}
		tag, value, _ := strings.Cut(line, ":")
		switch tag {
		case "#EXT-X-STREAM-INF":
			pl.Master = true
			a := parseAttrs(value)
			v := &variant{Audio: a["AUDIO"]}
			v.Bandwidth, _ = strconv.ParseInt(a["BANDWIDTH"], 10, 64)
			if w, h, ok := strings.Cut(a["RESOLUTION"], "x"); ok {
				v.Width, _ = strconv.Atoi(w)
				v.Height, _ = strconv.Atoi(h)
			}
			pendingInf = v
		case "#EXT-X-MEDIA":
			a := parseAttrs(value)
			r := rendition{Type: a["TYPE"], Group: a["GROUP-ID"], Name: a["NAME"],
				Default: a["DEFAULT"] == "YES", AutoSelect: a["AUTOSELECT"] == "YES"}
			if a["URI"] != "" {
				u, err := resolve(base, a["URI"])
				if err != nil {
					return nil, err
				}
				r.URL = u.String()
			}
			pl.Renditions = append(pl.Renditions, r)
		case "#EXT-X-TARGETDURATION":
			if n, err := strconv.ParseFloat(value, 64); err == nil && n > 0 && n < 3600 {
				pl.Target = time.Duration(n * float64(time.Second))
			}
		case "#EXT-X-MEDIA-SEQUENCE":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				return nil, errors.New("bad EXT-X-MEDIA-SEQUENCE")
			}
			seq = n
		case "#EXT-X-ENDLIST":
			pl.Ended = true
		case "#EXT-X-PLAYLIST-TYPE":
			if value == "VOD" {
				pl.Ended = true
			}
		case "#EXTINF":
			d, _, _ := strings.Cut(value, ",")
			dur, _ = strconv.ParseFloat(strings.TrimSpace(d), 64)
		case "#EXT-X-BYTERANGE":
			br, err := parseByteRange(value)
			if err != nil {
				return nil, err
			}
			byteRange = &br
		case "#EXT-X-KEY":
			a := parseAttrs(value)
			switch a["METHOD"] {
			case "NONE", "":
				key = nil
			case "AES-128":
				if a["URI"] == "" {
					return nil, errors.New("EXT-X-KEY without URI")
				}
				u, err := resolve(base, a["URI"])
				if err != nil {
					return nil, err
				}
				k := &keyInfo{URL: u.String()}
				if iv := a["IV"]; iv != "" {
					b, err := hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(iv, "0x"), "0X"))
					if err != nil || len(b) != 16 {
						return nil, errors.New("bad EXT-X-KEY IV")
					}
					k.IV = b
				}
				key = k
			default:
				return nil, fmt.Errorf("encryption %q is not supported (DRM-protected streams cannot be downloaded)", a["METHOD"])
			}
		case "#EXT-X-MAP":
			a := parseAttrs(value)
			u, err := resolve(base, a["URI"])
			if err != nil {
				return nil, err
			}
			r := resource{URL: u.String(), Length: -1}
			if v := a["BYTERANGE"]; v != "" {
				br, err := parseByteRange(v)
				if err != nil {
					return nil, err
				}
				r.Length, r.Offset = br.Length, max(br.Offset, 0)
			}
			initRes = &r
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read playlist: %w", err)
	}
	if first {
		return nil, errors.New("empty playlist")
	}
	if pl.Master && len(pl.Variants) == 0 {
		return nil, errors.New("master playlist without variants")
	}
	return pl, nil
}

// parseByteRange parses "<length>[@<offset>]"; Offset is -1 when absent.
func parseByteRange(v string) (resource, error) {
	ls, offs, hasOff := strings.Cut(strings.Trim(v, `"`), "@")
	n, err := strconv.ParseInt(ls, 10, 64)
	if err != nil || n <= 0 || n > maxSegmentBytes {
		return resource{}, errors.New("bad byte range")
	}
	off := int64(-1)
	if hasOff {
		off, err = strconv.ParseInt(offs, 10, 64)
		if err != nil || off < 0 {
			return resource{}, errors.New("bad byte range")
		}
	}
	return resource{Length: n, Offset: off}, nil
}

// parseAttrs parses an HLS attribute list: KEY=VALUE,KEY="quoted, value",...
func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for len(s) > 0 {
		k, rest, ok := strings.Cut(s, "=")
		if !ok {
			break
		}
		k = strings.TrimSpace(k)
		var v string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				v, rest = rest[1:], ""
			} else {
				v, rest = rest[1:1+end], rest[2+end:]
			}
			_, rest, _ = strings.Cut(rest, ",")
		} else {
			v, rest, _ = strings.Cut(rest, ",")
		}
		out[k] = strings.TrimSpace(v)
		s = rest
	}
	return out
}

// pickVariant chooses the highest-bandwidth variant (ties: the larger picture). When
// avoidSplit is set (no ffmpeg to mux), variants whose audio is a separate rendition are
// skipped if any other exists.
func pickVariant(pl *playlist, avoidSplit bool) (variant, *rendition) {
	best := -1
	for pass := 0; pass < 2 && best < 0; pass++ {
		for i, v := range pl.Variants {
			if pass == 0 && avoidSplit && audioRendition(pl, v) != nil {
				continue
			}
			if best < 0 || v.Bandwidth > pl.Variants[best].Bandwidth ||
				(v.Bandwidth == pl.Variants[best].Bandwidth && v.Width*v.Height > pl.Variants[best].Width*pl.Variants[best].Height) {
				best = i
			}
		}
	}
	v := pl.Variants[best]
	return v, audioRendition(pl, v)
}

// audioRendition returns the separate audio playlist a variant needs, or nil when its
// audio is muxed in (or it has none): DEFAULT=YES first, then AUTOSELECT, then the first.
func audioRendition(pl *playlist, v variant) *rendition {
	if v.Audio == "" {
		return nil
	}
	var pick *rendition
	score := -1
	for i := range pl.Renditions {
		r := &pl.Renditions[i]
		if r.Type != "AUDIO" || r.Group != v.Audio {
			continue
		}
		s := 0
		if r.Default {
			s = 2
		} else if r.AutoSelect {
			s = 1
		}
		if s > score {
			pick, score = r, s
		}
	}
	if pick == nil || pick.URL == "" {
		return nil
	}
	return pick
}
