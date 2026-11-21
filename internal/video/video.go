// Package video is what the video window knows of videos: dramas, episodes
// and the like, whichever source they came from, and what a source has to do.
// A video source is usually a plugin: see internal/plugin. Errors are those
// of package music, which the plugin machinery already makes of what a
// plugin throws.
package video

// IDs are the source's own and mean nothing outside it.

// Item is a video as the lists show it: a card in a row of a browse page, or
// of what a search found.
type Item struct {
	ID string `json:"id"`
	// Source is the ID of the source the item came from, which plays it. The
	// source's own answers leave it out; it's filled in for them.
	Source   string `json:"source,omitempty"`
	Title    string `json:"title"`
	Cover    string `json:"cover,omitempty"`
	Subtitle string `json:"subtitle,omitempty"` // such as how many episodes, or who made it
}

// Episode is one part of a video, in order.
type Episode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Duration in milliseconds, if the source knows.
	Duration int64 `json:"duration,omitempty"`
	// Unavailable says the listener can't play it, for a source that knows.
	Unavailable bool `json:"unavailable,omitempty"`
}

// Video is a video and its episodes, as its page shows.
type Video struct {
	Item
	Intro    string    `json:"intro,omitempty"`
	Episodes []Episode `json:"episodes,omitempty"`
	// More says there are episodes past those answered, for a source that
	// lists them a page at a time.
	More bool `json:"more,omitempty"`
}

// Row is a heading over what a browse page lists of it.
type Row struct {
	Title string `json:"title,omitempty"`
	Items []Item `json:"items,omitempty"`
	// MorePage is the Browse page with the rest of them, if there is one.
	MorePage string `json:"morePage,omitempty"`
}

// Browse is a page a source makes up itself: what its Home shows, and any
// other its rows link to.
type Browse struct {
	Title string `json:"title,omitempty"`
	Rows  []Row  `json:"rows,omitempty"`
}

// VideoResult is a page of what a search found.
type VideoResult struct {
	Items []Item `json:"items,omitempty"`
	More  bool   `json:"more,omitempty"` // there is a next page
}

// Stream is a video the app plays itself: what a source answers for an
// episode. URL is the picture alone or the whole thing; AudioURL, when set,
// is the sound to play with it, which the app joins through ffmpeg before it
// plays, as a stream that keeps them apart needs.
type Stream struct {
	URL      string            `json:"url"`
	AudioURL string            `json:"audioUrl,omitempty"`
	Type     string            `json:"type"` // hls or mp4
	Headers  map[string]string `json:"headers,omitempty"`
}

// Info is how a video source introduces itself.
type Info struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// VideoSource is where videos come from. Everything but Info and Can may take
// a while and is called off the main thread. What a source doesn't export it
// doesn't do, and Can says so beforehand for the views to leave it out.
type VideoSource interface {
	Info() Info
	// Can reports whether the source does what's named, by the name of its
	// plugin's function, such as videoSearch or videoHome. It answers at
	// once, on the main thread, without starting anything.
	Can(what string) bool

	// Home is what the window shows first, for a source that makes up its own.
	Home() (*Browse, error)
	// Browse is any other page it makes up.
	Browse(page string) (*Browse, error)

	Search(query string, offset int) (*VideoResult, error)

	// Video is a video and its episodes.
	Video(id string) (*Video, error)

	// Play answers the stream of an episode of a video.
	Play(id, ep string) (*Stream, error)
}
