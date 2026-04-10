# gomod-age

Check Go module dependencies for minimum release age. Blocks dependencies that were published too recently, as a defense against supply chain attacks.

## Usage

```bash
go run ./tools/gomod-age/cmd/gomod-age --config .gomod-age.json
```

## Configuration

Create a `.gomod-age.json` file (compatible with [fchimpan/gomod-age](https://github.com/fchimpan/gomod-age)):

```json
{
  "age": "7d",
  "ignore": ["buf.build/*"],
  "allow": [
    {
      "module": "github.com/some/pkg",
      "version": "v1.2.0",
      "reason": "security patch, reviewed in PR #42"
    }
  ]
}
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All dependencies pass |
| 1 | Violations found (dependencies too new) |
| 2 | Errors (proxy failures, bad config) |

## Attribution

This tool is inspired by and config-compatible with [gomod-age](https://github.com/fchimpan/gomod-age) by [@fchimpan](https://github.com/fchimpan). We built our own implementation to avoid depending on a still new/small third-party tool in CI, while using the same config format and design decisions.
