# SeMaps prerequisites

Read when SeMaps or its MCP is missing, the wrong workspace is connected, or
the user requests adoption. This skill is an instruction package: it does not
install or configure a server by itself.

## Binary and connection

Requires an installed `semaps` binary, available from
[SeMaps Releases](https://github.com/KOE73/SeMaps/releases), and a SeMaps MCP
server connected to the agent for the consuming repository.

Use the client's supported MCP configuration to run `semaps mcp` in that
repository. Reload the agent if necessary, then verify the actual connection
with `list_projects` and the intended view/reference. A configuration entry
alone does not prove the tools are connected to the right map.

If setup is requested, follow the current
[ADOPTING guide](https://github.com/KOE73/SeMaps/blob/main/docs/ADOPTING.md)
and [installation instructions](https://github.com/KOE73/SeMaps#getting-started).
Preserve existing workspace paths, other MCP servers and running human sessions.
Do not silently reinstall, retarget or migrate a project merely to inspect it.
Do not use a copied example configuration as though it targeted the user's map.

## Code extraction

Code extraction additionally requires the shipped language extractor and runtime.
Use `doctor` and the server's current instructions to diagnose missing components.
Report unavailable extraction instead of inventing structural facts or manually
substituting entities for extracted code records.

SeMaps documents real Windows editor and C# code-graph use. Linux/macOS builds
and TypeScript/Go extractors exist, but equivalent end-to-end validation is not
claimed. Local integration QA also exercised this skill on a copy of SeMaps'
Go core; that does not establish support for every project or agent client.
