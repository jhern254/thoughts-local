# Licensed speech fixture

`1272-128104-0000.f32le` is derived from LibriSpeech's public audiobook
utterance `LibriSpeech/dev-clean/1272/128104/1272-128104-0000.flac`.
It is not a Thoughts user's recording.

Source: https://www.openslr.org/resources/12/dev-clean.tar.gz
Dataset and license: https://www.openslr.org/12/
License: Creative Commons Attribution 4.0 International,
https://creativecommons.org/licenses/by/4.0/legalcode

Attribution: Vassil Panayotov, Guoguo Chen, Daniel Povey and Sanjeev Khudanpur,
“LibriSpeech: an ASR corpus based on public domain audio books,” ICASSP 2015.
The corpus derives from public-domain LibriVox audiobooks.

The change is lossless conversion of the mono 16 kHz signed 16-bit FLAC samples
into normalized float32 little-endian PCM (`sample / 32768`). No trimming,
resampling or transcript modification was performed. There are 93,680 samples,
374,720 bytes and 5.855 seconds of audio. A decoder such as ffmpeg can reproduce it:

```sh
ffmpeg -i 1272-128104-0000.flac -c:a pcm_f32le -f f32le 1272-128104-0000.f32le
```

SHA-256 of original FLAC:
`4e25e22555cd16e90edb0a3b49fdcf1fe652b2a1250ab643634db33895c75b41`

SHA-256 of derived PCM:
`22d472b40f913206b6917115d136306be88df3b278e77f68be5910423250fa4d`

The authoritative reference is the matching archive transcript file:

> MISTER QUILTER IS THE APOSTLE OF THE MIDDLE CLASSES AND WE ARE GLAD TO WELCOME HIS GOSPEL

The native smoke test checks reference phrases, not a general accuracy metric.
Synthetic silence separately checks no-speech behavior and resource lifetimes.
