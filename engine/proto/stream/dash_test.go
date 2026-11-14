package stream

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

func TestExpandTemplate(t *testing.T) {
	rep := &repXML{ID: "v1", Bandwidth: 800000}
	for in, want := range map[string]string{
		"$RepresentationID$/seg-$Number$.m4s": "v1/seg-42.m4s",
		"$RepresentationID$/$Number%05d$.m4s": "v1/00042.m4s",
		"t/$Time$.m4s":                        "t/90000.m4s",
		"b$Bandwidth$-$$-$Unknown$":           "b800000-$-$Unknown$",
		"init-$RepresentationID$.mp4":         "init-v1.mp4",
		"dangling$Number":                     "dangling$Number",
	} {
		if got := expandTemplate(in, rep, 42, 90000); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestISODuration(t *testing.T) {
	for in, want := range map[string]float64{"PT1H2M3.5S": 3723.5, "PT10S": 10, "P1DT1S": 86401, "": 0, "PT0.5S": 0.5} {
		got, err := isoDuration(in)
		if err != nil || got != want {
			t.Errorf("%q -> %v %v, want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"1H", "PT", "PTxS", "P1Y"} {
		if _, err := isoDuration(bad); err == nil && bad != "PT" {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseMPDDurationTemplateAndList(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/movie/manifest.mpd?tok=1")
	mpd := `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT9.5S">
  <BaseURL>media/</BaseURL>
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate timescale="1000" duration="2000" startNumber="5" initialization="$RepresentationID$/init.mp4" media="$RepresentationID$/$Number%03d$.m4s"/>
      <Representation id="lo" bandwidth="100000" width="320" height="180"/>
      <Representation id="hi" bandwidth="900000" width="1280" height="720"/>
    </AdaptationSet>
    <AdaptationSet mimeType="audio/mp4">
      <Representation id="a" bandwidth="64000">
        <SegmentList>
          <Initialization sourceURL="a.mp4" range="0-99"/>
          <SegmentURL media="a.mp4" mediaRange="100-199"/>
          <SegmentURL media="a.mp4" mediaRange="200-299"/>
        </SegmentList>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`
	v, a, err := parseMPD([]byte(mpd), base)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "hi" || len(v.segments) != 5 { // ceil(9.5 / 2)
		t.Fatalf("video %s with %d segments", v.ID, len(v.segments))
	}
	if got := v.segments[0].Res.URL; got != "https://cdn.example.com/movie/media/hi/005.m4s" {
		t.Fatalf("first segment %s", got)
	}
	if got := v.segments[4].Init.URL; got != "https://cdn.example.com/movie/media/hi/init.mp4" {
		t.Fatalf("init %s", got)
	}
	if a.ID != "a" || len(a.segments) != 2 || a.segments[1].Res.Offset != 200 || a.segments[1].Res.Length != 100 || a.segments[0].Init.Length != 100 {
		t.Fatalf("audio %+v", a)
	}
	if _, _, err := parseMPD([]byte(`<MPD type="dynamic"><Period/></MPD>`), base); err == nil || !strings.Contains(err.Error(), "live") {
		t.Fatalf("dynamic MPD: %v", err)
	}
	if _, _, err := parseMPD([]byte(`<MPD><Period><AdaptationSet mimeType="video/mp4"><ContentProtection/><Representation id="x" bandwidth="1"><BaseURL>x.mp4</BaseURL></Representation></AdaptationSet></Period></MPD>`), base); err == nil || !strings.Contains(err.Error(), "DRM") {
		t.Fatalf("DRM MPD: %v", err)
	}
}

// dashServer publishes an MPD with a SegmentTimeline ($Time$) video set of two
// representations and, optionally, an audio set using $Number$.
func dashServer(t *testing.T, withAudio bool) (*server, []byte) {
	s := newServer(t)
	var want []byte
	for _, rep := range []string{"v-lo", "v-hi"} {
		size := 700
		if rep == "v-hi" {
			size = 2100
		}
		init := payload(rep+"/init", 300)
		s.set("/dash/"+rep+"/init.mp4", init)
		if rep == "v-hi" {
			want = append(want, init...)
		}
		for _, tm := range []int{0, 2000, 4000, 6000, 7000} { // S d=2000 r=2, then d=1000 r=1
			name := fmt.Sprintf("/dash/%s/t%d.m4s", rep, tm)
			data := payload(name, size)
			s.set(name, data)
			if rep == "v-hi" {
				want = append(want, data...)
			}
		}
	}
	audio := ""
	if withAudio {
		s.set("/dash/a/init.mp4", payload("a-init", 100))
		for n := 1; n <= 4; n++ {
			s.set(fmt.Sprintf("/dash/a/%d.m4s", n), payload(fmt.Sprintf("a%d", n), 500))
		}
		audio = `<AdaptationSet contentType="audio" mimeType="audio/mp4">
      <SegmentTemplate timescale="1" duration="2" initialization="$RepresentationID$/init.mp4" media="$RepresentationID$/$Number$.m4s"/>
      <Representation id="a" bandwidth="128000" codecs="mp4a.40.2"/>
    </AdaptationSet>`
	}
	s.set("/dash/stream.mpd", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT8S" profiles="urn:mpeg:dash:profile:isoff-live:2011">
  <Period id="0">
    <AdaptationSet contentType="video" mimeType="video/mp4">
      <SegmentTemplate timescale="1000" initialization="$RepresentationID$/init.mp4" media="$RepresentationID$/t$Time$.m4s">
        <SegmentTimeline>
          <S t="0" d="2000" r="2"/>
          <S d="1000" r="-1"/>
        </SegmentTimeline>
      </SegmentTemplate>
      <Representation id="v-lo" bandwidth="300000" width="640" height="360"/>
      <Representation id="v-hi" bandwidth="2000000" width="1920" height="1080"/>
    </AdaptationSet>
    `+audio+`
  </Period>
</MPD>`))
	return s, want
}

// TestDASHSegmentTemplate: SegmentTemplate with SegmentTimeline ($Time$, r=-1); the best
// representation is chosen and a single track is saved as .mp4 without ffmpeg.
func TestDASHSegmentTemplate(t *testing.T) {
	s, want := dashServer(t, false)
	m := newManager(t, testProto(NewDASH(Options{}), false), testProto(NewHLS(Options{}), false))
	d, err := m.Add(engine.Request{URL: s.URL + "/dash/stream.mpd"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "dash" {
		t.Fatalf("kind = %q", d.Kind)
	}
	done := waitFor(t, m, d.ID, engine.StatusCompleted, 10*time.Second)
	checkFile(t, done, "stream.mp4", want)
	if s.hitCount("/dash/v-lo/init.mp4") != 0 || s.hitCount("/dash/v-hi/init.mp4") != 1 {
		t.Fatal("wrong representation or init fetched more than once")
	}
}

// TestDASHWithoutFFmpeg: video + audio need ffmpeg; without it the error says so.
func TestDASHWithoutFFmpeg(t *testing.T) {
	s, _ := dashServer(t, true)
	m := newManager(t, testProto(NewDASH(Options{}), false))
	d, err := m.Add(engine.Request{URL: s.URL + "/dash/stream.mpd"})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitFor(t, m, d.ID, engine.StatusError, 10*time.Second)
	if failed.Error != ErrFFmpegMissing.Error() || !strings.Contains(failed.Error, "install ffmpeg") {
		t.Fatalf("error = %q", failed.Error)
	}
	if s.hitCount("/dash/v-hi/init.mp4") != 0 {
		t.Fatal("downloaded segments although they could not be combined")
	}
}

// TestDASHSegmentBaseSingleFile: a representation that is one file (BaseURL +
// SegmentBase) is fetched in byte ranges.
func TestDASHSegmentBaseSingleFile(t *testing.T) {
	s := newServer(t)
	want := payload("whole", 150_000)
	s.set("/sb/audio_only.m4a", want)
	s.set("/sb/a.mpd", []byte(`<MPD type="static" mediaPresentationDuration="PT5S"><Period>
  <AdaptationSet mimeType="audio/mp4"><Representation id="a1" bandwidth="96000">
    <BaseURL>audio_only.m4a</BaseURL><SegmentBase indexRange="0-99"><Initialization range="0-49"/></SegmentBase>
  </Representation></AdaptationSet></Period></MPD>`))
	m := newManager(t, testProto(NewDASH(Options{}), false))
	d, err := m.Add(engine.Request{URL: s.URL + "/sb/a.mpd"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m, d.ID, engine.StatusCompleted, 10*time.Second)
	checkFile(t, done, "a.m4a", want)
}

func TestFFmpegFinder(t *testing.T) {
	if got := findFFmpeg(t.TempDir() + "/nope/ffmpeg"); got != "" {
		t.Fatalf("missing configured path returned %q", got)
	}
	p := NewDASH(Options{FFmpeg: t.TempDir() + "/missing"})
	if p.ffmpeg() != "" {
		t.Fatal("configured missing ffmpeg should mean none")
	}
}
