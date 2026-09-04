# Release Compatibility Fixtures

Each manifest in this directory identifies serialized sketch state published by a
specific release. Go and Python tests load the same bytes, check their digest and
observable state, and require byte-identical serialization after parsing.

Existing manifests and their wire digests are immutable. Add a new manifest when a
release introduces a new compatibility surface; do not rewrite an older manifest to
match a newer implementation.
