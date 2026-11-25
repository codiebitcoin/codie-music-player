// Fork of egoist/mygo-music as "Codie Music Player": stream sources can play
// on a paired codie device instead of the Mac (settings > Codie).
export default ({ command }: { command: string }) => ({
  name: "Codie Music Player",
  identifier: "com.codie.musicplayer",
  slug: "codie-music-player",
  // version tracks upstream: plugins check minAppVersion against it
  version: "0.1.28",
  out: "build",
  urlSchemes: [command === "dev" ? "codie-music-player-dev" : "codie-music-player"],
  macos: {
    icon: "icon/AppIcon.icon",
    minimumSystemVersion: "13.0",
  },
  linux: {
    comment: "A music player that plays on your codie device",
    categories: ["AudioVideo", "Audio", "Player"],
    webview: false,
  },
});
