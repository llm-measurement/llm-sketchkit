# Go-To-Python Walkthrough

Watch Go summaries become distinct-user estimates and token-volume intervals in
Python, with synthetic reference counts checking the results.

[Watch the 90-second MP4](https://raw.githubusercontent.com/llm-measurement/llm-sketchkit/main/docs/media/walkthrough.mp4)
in your video player, without an account. This silent, captioned walkthrough uses
plots from the executed [notebook](../../examples/go-to-python/README.md), captured
on 2026-09-04. Edited scene timing is for presentation.

## Transcript

- **0:00-0:30:** Go produces service-local summaries. Python checks byte-for-byte
  round trips, rejects incompatible profiles, and merges two shards in each of two
  windows. The first plot compares distinct estimates with known synthetic counts.
- **0:30-0:45:** HLL++ estimates distinct users with fixed-size state. This plot
  checks estimates against known synthetic counts; error is statistical.
- **0:45-1:15:** Weighted frequent-items returns deterministic token intervals.
  Green crosses are exact synthetic validation values. Each interval is normalized
  to its own upper estimate to show uncertainty width. See
  [how to read the axis](../../examples/go-to-python/README.md#see-the-result).
- **1:15-1:30:** Known keys can be named through an authorized local mapping.
  Protect the secret and control access to summaries. Start with
  `python -m pip install llm-sketchkit` and the notebook's setup instructions.

The collector dashboard is a separate workflow. The connector exports metrics,
bounded structured logs, and optional summary envelopes containing sketch state.
This notebook reads its own producer's manifest and individual sketch files;
use [summary exchange](../../examples/summary-exchange/README.md) for collector envelopes.

## Reproduce It

After installing the [notebook prerequisites](../../examples/go-to-python/README.md):

```sh
python -m pip install nbclient
python examples/go-to-python/render.py
python docs/media/render.py docs/media/scenes.json --ffmpeg ffmpeg
```

The first script executes every notebook cell and exports its two plots without
adding outputs to the source notebook. Ephemeral secrets mean hashes and estimates
can differ between runs. The second uses Python's standard library and an installed
FFmpeg with `drawtext` and H.264 support; it was checked with FFmpeg 7.1.
Caption text, image sources, and timing are recorded in [scenes.json](scenes.json).
