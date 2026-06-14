
Possible features and affordances:


## Features

- count tokens in brief
- base or opinionated writer sets and structure


## Nits

- let's check the gitignore insert, is the `**/.obsidian/` pattern preferable to survive vault renames
- is it worth instantiating `manifest_path = ".memento/manifest.json"` in the `config.toml`? 
- do we just always live-resolve the location of the `.memento`, double check we're not putting it anywhere durable that won't survive renames
- The flow for writes should be to read the relevant style guide once, then make multiple edits. That belongs in agent guidance, not as an appendage to 
- post-write maybe feed back instructions to run the compile again, or do it automatically?




I'm back and forth on whether we need an mcp after all. Beads demonstrates that you get fine performance with the cli so likely no need. Just need to manage consistent injection of instructions.



