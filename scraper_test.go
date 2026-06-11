package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestReXhTagsClassifiesCategoriesPornstarsAndTags(t *testing.T) {
	const xhFixture = `{"videoModel":{"tags":[{"isCategory":true,"name":"Anal"},{"isPornstar":true,"name":"Jane Doe"},{"name":"hardcore"}]}}`

	var categories []string
	var tags []string
	var pornstars []string

	for _, tm := range reXhTags.FindAllStringSubmatch(xhFixture, -1) {
		if len(tm) < 4 {
			t.Fatalf("expected 4 submatches, got %d in %#v", len(tm), tm)
		}
		switch {
		case tm[1] != "":
			pornstars = append(pornstars, strings.TrimSpace(tm[1]))
		case tm[2] != "":
			categories = append(categories, strings.TrimSpace(tm[2]))
		case tm[3] != "":
			tags = append(tags, strings.TrimSpace(tm[3]))
		}
	}

	if !reflect.DeepEqual(categories, []string{"Anal"}) {
		t.Fatalf("categories = %#v, want %#v", categories, []string{"Anal"})
	}
	if !reflect.DeepEqual(pornstars, []string{"Jane Doe"}) {
		t.Fatalf("pornstars = %#v, want %#v", pornstars, []string{"Jane Doe"})
	}
	if !reflect.DeepEqual(tags, []string{"hardcore"}) {
		t.Fatalf("tags = %#v, want %#v", tags, []string{"hardcore"})
	}
}

func TestReTfCategoryCapturesDisplayName(t *testing.T) {
	const html = `<a data-category="big-tits">Big Tits</a>`

	match := reTfCategory.FindStringSubmatch(html)
	if len(match) != 3 {
		t.Fatalf("reTfCategory match len = %d, want 3", len(match))
	}
	if match[2] != "Big Tits" {
		t.Fatalf("reTfCategory display name = %q, want %q", match[2], "Big Tits")
	}
}

func TestReDtCatLinksCapturesCategoryName(t *testing.T) {
	const html = `<a href="/categories/teen">Teen</a>`

	match := reDtCatLinks.FindStringSubmatch(html)
	if len(match) != 2 {
		t.Fatalf("reDtCatLinks match len = %d, want 2", len(match))
	}
	if match[1] != "Teen" {
		t.Fatalf("reDtCatLinks category = %q, want %q", match[1], "Teen")
	}
}
