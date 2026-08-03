# DataSketches Oracle Comparison

This report compares `llm-sketchkit` weighted frequent-items query
semantics against Apache DataSketches Python. DataSketches is used as
a reference oracle only; it is not a runtime dependency, wire-format
authority, or HLL++ semantic authority for this project.

- DataSketches Python: `5.2.0`
- Fixture: `vectors/oracles/datasketches_frequent_items.json`

## Summary

| Workload | Total weight | Distinct keys | Threshold | Sketchkit NFN recall | DataSketches NFN recall | Sketchkit NFP valid | DataSketches NFP valid |
|---|---:|---:|---:|---:|---:|---|---|
| zipf_1_1_weighted_small | 233931 | 14301 | 1243 | 1.00 | 1.00 | yes | yes |
| tail_churn_weighted_small | 128728 | 24032 | 3272 | 1.00 | 1.00 | yes | yes |

## Workload Details

### `zipf_1_1_weighted_small`

- Profile: `small`
- Updates: `200000`
- Distinct keys: `14301`
- Total weight: `233931`
- Top-20 threshold: `>1243`
- Sketchkit map size: `512`
- DataSketches lg_max_k: `10` (nominal capacity `768`)

| Engine | Tracked items | Error signal | NFN candidates | NFP candidates | Top-k recall | NFP valid |
|---|---:|---:|---:|---:|---:|---|
| llm-sketchkit | 494 | 213 | 20 | 17 | 1.00 | yes |
| DataSketches | 616 | 800 | 20 | 18 | 1.00 | yes |

Partitioned merge orders all preserve top-k recall and no-false-positive
validity in both engines. Full per-order rows and top-item intervals are
in `reports/datasketches_oracle.json`.

### `tail_churn_weighted_small`

- Profile: `small`
- Updates: `120000`
- Distinct keys: `24032`
- Total weight: `128728`
- Top-20 threshold: `>3272`
- Sketchkit map size: `512`
- DataSketches lg_max_k: `10` (nominal capacity `768`)

| Engine | Tracked items | Error signal | NFN candidates | NFP candidates | Top-k recall | NFP valid |
|---|---:|---:|---:|---:|---:|---|
| llm-sketchkit | 463 | 49 | 24 | 0 | 1.00 | yes |
| DataSketches | 448 | 440 | 24 | 0 | 1.00 | yes |

Partitioned merge orders all preserve top-k recall and no-false-positive
validity in both engines. Full per-order rows and top-item intervals are
in `reports/datasketches_oracle.json`.
