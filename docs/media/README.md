# Go-To-Python Walkthrough

[Download the 90-second MP4](https://raw.githubusercontent.com/llm-measurement/llm-sketchkit/main/docs/media/walkthrough.mp4).
No account is needed. Open the downloaded file in a video player; GitHub does not
provide an embedded player for this file.

This silent, captioned video uses actual plots from the executed
[Go-to-Python notebook](../../examples/go-to-python/README.md), captured on 2026-09-04.
It is an edited walkthrough of synthetic results, not a real-time screen recording
or a performance benchmark.

## Transcript

- **0:00-0:30:** Go produces service-local summaries. Python checks byte-for-byte
  round trips, rejects incompatible profiles, and merges two shards in each of two
  windows. The first plot compares distinct estimates with known synthetic counts.
- **0:30-0:45:** HLL++ returns an estimate, not a list of identities. Its measured
  characterization envelope is not a per-estimate confidence interval.
- **0:45-1:15:** Weighted frequent-items returns deterministic token intervals.
  Green crosses are exact synthetic validation values. Each interval is normalized
  to its own upper estimate; the axis is not a share of total token volume.
- **1:15-1:30:** Hashes remain linkable under one secret. The notebook cannot name a
  user without a separately authorized mapping. Start with
  `python -m pip install llm-sketchkit` and the notebook's setup instructions.

The collector dashboard is a separate workflow. The connector currently exports
metrics and bounded structured logs, not the serialized sketch files used here.

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
