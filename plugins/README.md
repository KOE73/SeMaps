# IDE plugins

Each IDE adapter has its own build and release artifact under `plugins/<ide>/`.
The consuming repository still keeps only its `.semaps` file and workspace.

- [Shared protocol](protocol/README.md): JSON envelopes, examples and transport checks.
- [Visual Studio](visual-studio/README.md): VSIX prototype and Experimental Instance.

IDE-specific drag formats, source parsing, symbol services and project UI belong to
the plugin. SeMaps keeps the model and its editing rules; plugins must not write
workspace files themselves or depend on a particular consuming source tree.
