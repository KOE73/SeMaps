# Extractor contract

An extractor is a separate executable `semaps-extract-<lang>`. It reads sources and prints
facts to stdout. It knows nothing about the registry, rules, containers or views.

```json
{
  "language": "csharp",
  "symbols": [
    { "id": "", "kind": "type|interface|function|module|value", "nativeKind": "",
      "name": "", "namespace": "", "file": "", "line": 0 }
  ],
  "edges": [
    { "from": "", "to": "", "kind": "extends|implements|references|contains" }
  ]
}
```

`kind` is the shared minimal vocabulary. Language specifics go to `nativeKind` only.
