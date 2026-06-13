
- Currently we dump a flat list, but the actual flow of going from the brief to looking further at a specific item is a little painful (have to supply the exact file name). Instead, brief should return each entry with a numeric reference, so that the follow-up command becomes `memento read 4` or perhaps even `memento read 4 2` for slug referencing, and to reference these in the brief in the same way:
  
  instead of:

```md
Headings: Context; Decision; Ignore Semantics; Consequences
```

  we can have:

```md
Headings:

1 Context
2 Decision
3   Ignore Semantics
4 Consequences
```

(perhaps making clear which are h3 via indentation or other method in this way)

- Probably worth having a `memento brief --index` which is only the top-level index list above. If the agent is directed to consult a particular doc and needs only find the reference, for example

- I think the folder structure is important to surface, this can be done with section headers, e.g. 
  
  ```md
  1 agent-human review boundaries
  2 spec
  
  ## Architecture decision record
  3 adr-0001-vault-naming
  4 adr-0002-marker-based-vault-discovery
  ```
  
  
  - `memento read` may want to be mildly more clever in due course
    
hyperlinks, e.g. [[spec]] could be re-rendered with the index reference `[[spec @ 1]]`, same for an in-links list if we want to append that.

