# flacframe

The `frame` package of [mewkiz/flac](https://github.com/mewkiz/flac) v1.0.14
(public domain, see LICENSE), with the internal packages it needs, copied so
that a frame can decode into the sample buffers of the frame before
(`Frame.Reuse`). The library made new buffers for every frame: some 0.35 MB a
second of a CD-quality song, all garbage, which showed as memory climbing
while a song played. The encoder half of `internal/bits` is left out.

The rest of the library (stream info, metadata) is still used from the module.
