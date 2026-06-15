package markdown

import "testing"

func TestExtractWikiLinks(t *testing.T) {
	source := []byte(`# Note

See [[Alpha]], [[Beta#Decision|friendly label]], [[#Local Heading]], and ![[Embeds/Thing]].
Ignore [markdown](link.md) and incomplete [[Nope.
`)

	got := ExtractWikiLinks(source)
	want := []WikiLink{
		{Target: "Alpha", Type: WikiLinkTypeWiki, Occurrence: 0},
		{Target: "Beta", Anchor: "Decision", Type: WikiLinkTypeWiki, Occurrence: 1},
		{Target: "Embeds/Thing", Type: WikiLinkTypeEmbed, Occurrence: 2},
	}

	if len(got) != len(want) {
		t.Fatalf("ExtractWikiLinks() len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractWikiLinks()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
