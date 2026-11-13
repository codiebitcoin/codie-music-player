module codie-music-player

go 1.27.1

require (
	github.com/dop251/goja v0.0.0-20261110204830-f493a2b600d6
	github.com/dop251/goja_nodejs v0.0.0-20260918173711-b481721df8a2
	github.com/egoist/mygo v0.4.5
	github.com/skip2/go-qrcode v0.0.0
)

replace github.com/skip2/go-qrcode => ./third_party/go-qrcode

require (
	github.com/dlclark/regexp2/v2 v2.11.0 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	golang.org/x/text v0.43.0 // indirect
)
