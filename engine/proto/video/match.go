package video

import (
	"net/url"
	"strings"
)

// site is one well-known video host. Subdomains match too (m.youtube.com,
// music.youtube.com, clips.twitch.tv). When paths is set, only URLs whose path starts with
// one of them match, for hosts that also serve plain files or non-video pages.
type site struct {
	host  string
	paths []string
}

// sites is the built-in list of hosts whose page URLs go to yt-dlp without the user picking
// "video". Any other page yt-dlp supports (about 1800 sites) works with Kind "video".
var sites = []site{
	{host: "youtube.com"},
	{host: "youtu.be"},
	{host: "youtube-nocookie.com"},
	{host: "vimeo.com"},
	{host: "x.com"},
	{host: "twitter.com"},
	{host: "tiktok.com"},
	{host: "instagram.com"},
	{host: "facebook.com"},
	{host: "fb.watch"},
	{host: "twitch.tv"},
	{host: "dailymotion.com"},
	{host: "dai.ly"},
	{host: "soundcloud.com"},
	{host: "bilibili.com"},
	{host: "b23.tv"},
	{host: "v.redd.it"},
	{host: "reddit.com", paths: []string{"/r/", "/user/", "/u/"}},
	{host: "streamable.com"},
	{host: "rumble.com"},
	{host: "odysee.com"},
	{host: "bitchute.com"},
	{host: "kick.com"},
	{host: "bandcamp.com"},
	{host: "mixcloud.com"},
	{host: "threads.net"},
	{host: "threads.com"},
	{host: "loom.com", paths: []string{"/share/", "/embed/"}},
	{host: "ted.com", paths: []string{"/talks/"}},
	{host: "nicovideo.jp"},
	{host: "vk.com", paths: []string{"/video", "/clip"}},
	{host: "vkvideo.ru"},
	{host: "ok.ru", paths: []string{"/video"}},
}

// Match reports whether rawURL is a page on a well-known video site. It never does I/O.
func (*Protocol) Match(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return false
	}
	for _, s := range sites {
		if host != s.host && !strings.HasSuffix(host, "."+s.host) {
			continue
		}
		if len(s.paths) == 0 {
			return true
		}
		for _, p := range s.paths {
			if strings.HasPrefix(u.Path, p) {
				return true
			}
		}
		return false
	}
	return false
}
