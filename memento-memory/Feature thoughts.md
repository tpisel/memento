
Possible features and affordances:


## Features

- count tokens in brief
- base or opinionated writer sets and structure


## Nits

- let's check the gitignore insert, is the `**/.obsidian/` pattern preferable to survive vault renames
- is it worth instantiating `manifest_path = ".memento/manifest.json"` in the `config.toml`? Can we just assume this is the case?
- do we just always live-resolve the location of the vault via `.memento`, double check we're not putting it anywhere durable that won't survive renames

