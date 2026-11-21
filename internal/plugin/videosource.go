package plugin

import (
	"net/url"

	"mygo-music/internal/video"
)

// VideoSource is a plugin as a source of videos. Its answers name their items
// without their source, which is the plugin, as the music ones do.

// VideoSource is what a plugin's video exports answer through.
type VideoSource struct{ p *Plugin }

func (v *VideoSource) Info() video.Info {
	return video.Info{ID: v.p.manifest.ID, Name: v.p.t(v.p.manifest.Name), Version: v.p.manifest.Version}
}

// Can reports whether the plugin exports the video function named whole, such
// as videoSearch or videoHome.
func (v *VideoSource) Can(what string) bool { return v.p.Can(what) }

// stamp marks everything a video answer brought as the plugin's, so the video
// window knows whom to ask about it.
func (v *VideoSource) stamp(items []video.Item) {
	for i := range items {
		items[i].Source = v.p.manifest.ID
	}
}

func (v *VideoSource) stampBrowse(b *video.Browse) {
	for i := range b.Rows {
		v.stamp(b.Rows[i].Items)
	}
}

func (v *VideoSource) Home() (*video.Browse, error) {
	var b video.Browse
	if err := v.p.call("videoHome", nil, &b); err != nil {
		return nil, err
	}
	v.stampBrowse(&b)
	return &b, nil
}

func (v *VideoSource) Browse(page string) (*video.Browse, error) {
	var b video.Browse
	if err := v.p.call("videoBrowse", map[string]string{"page": page}, &b); err != nil {
		return nil, err
	}
	v.stampBrowse(&b)
	return &b, nil
}

func (v *VideoSource) Search(query string, offset int) (*video.VideoResult, error) {
	var r video.VideoResult
	if err := v.p.call("videoSearch", map[string]any{"query": query, "offset": offset}, &r); err != nil {
		return nil, err
	}
	v.stamp(r.Items)
	return &r, nil
}

func (v *VideoSource) Video(id string) (*video.Video, error) {
	var out video.Video
	if err := v.p.call("videoItem", map[string]string{"id": id}, &out); err != nil {
		return nil, err
	}
	out.Source = v.p.manifest.ID
	return &out, nil
}

func (v *VideoSource) Play(id, ep string) (*video.Stream, error) {
	var s video.Stream
	if err := v.p.call("videoPlay", map[string]string{"id": id, "episode": ep}, &s); err != nil {
		return nil, err
	}
	// The plugin's requestHeaders for video go with it, under what the
	// stream says itself.
	if u, err := url.Parse(s.URL); err == nil {
		for k, h := range v.p.HeadersFor(u, KindVideo) {
			if s.Headers == nil {
				s.Headers = map[string]string{}
			}
			if !hasHeader(s.Headers, k) {
				s.Headers[k] = h
			}
		}
	}
	return &s, nil
}

var _ video.VideoSource = (*VideoSource)(nil)
