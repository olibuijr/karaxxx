package main

import "testing"

func TestBuildVideoPageURLUsesStoredProviderSlug(t *testing.T) {
	cases := []struct {
		name string
		v    Video
		want string
	}{
		{
			name: "tnaflix full path slug",
			v:    Video{ID: "6339217", Source: "tnaflix", Slug: "amateur-porn/Caught-teens-sucking-and-riding/video6339217"},
			want: "https://www.tnaflix.com/amateur-porn/Caught-teens-sucking-and-riding/video6339217",
		},
		{
			name: "drtuber id and slug",
			v:    Video{ID: "8223813", Source: "drtuber", Slug: "un-voyeur-plage-filme-des-femmes-en-topless"},
			want: "https://www.drtuber.com/video/8223813/un-voyeur-plage-filme-des-femmes-en-topless",
		},
		{
			name: "eporner hash",
			v:    Video{ID: "tjoELPsX5fE", Source: "eporner"},
			want: "https://www.eporner.com/video-tjoELPsX5fE/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildVideoPageURL(tc.v); got != tc.want {
				t.Fatalf("buildVideoPageURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMediaBackfillCapableSourcesIncludeYTDLPProviders(t *testing.T) {
	for _, source := range []string{"xnxx", "xhamster", "xvideos", "eporner", "tnaflix", "drtuber", "heavyfetish", "punishbang", "sunporno"} {
		if !isMediaBackfillCapableSource(source) {
			t.Fatalf("expected %s to be media-backfill capable", source)
		}
	}
	if isMediaBackfillCapableSource("unknown") {
		t.Fatal("unknown source should not be media-backfill capable")
	}
}

func TestApplyYTDLPFormatsKeepsBestPerQuality(t *testing.T) {
	v := Video{ID: "x", Source: "tnaflix", Title: "Old"}
	info := ytDLPInfo{
		Title:     "New Title",
		Thumbnail: "https://example.test/thumb.jpg",
		Duration:  123,
		Formats: []ytDLPFormat{
			{FormatID: "360p", Height: 360, URL: "https://cdn.example/360.mp4", Ext: "mp4"},
			{FormatID: "720p", Height: 720, URL: "https://cdn.example/720.mp4", Ext: "mp4"},
			{FormatID: "1080p", Height: 1080, URL: "https://cdn.example/1080.mp4", Ext: "mp4"},
			{FormatID: "hls", URL: "https://cdn.example/master.m3u8", Ext: "m3u8"},
		},
	}
	applyYTDLPInfo(&v, info)
	if v.Title != "New Title" || v.ThumbUUID != "https://example.test/thumb.jpg" || v.Duration != 123 {
		t.Fatalf("metadata not applied: %+v", v)
	}
	if v.URL360 != "https://cdn.example/360.mp4" || v.URL720 != "https://cdn.example/720.mp4" || v.URL1080 != "https://cdn.example/1080.mp4" {
		t.Fatalf("qualities not assigned correctly: 360=%q 720=%q 1080=%q", v.URL360, v.URL720, v.URL1080)
	}
	if v.HLSURL != "https://cdn.example/master.m3u8" {
		t.Fatalf("HLS not assigned: %q", v.HLSURL)
	}
}
