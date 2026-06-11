package main

import (
	"reflect"
	"testing"
)

func TestNormalizeCategoryListDropsEmptyAndDeduplicates(t *testing.T) {
	input := []string{" Anal ", "uncategorized", "", "ANAL", " Teen ", "teen"}

	got := normalizeCategoryList(input)
	want := []string{"anal", "teen"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCategoryList() = %#v, want %#v", got, want)
	}
}

func TestMergeCategoryListsFoldsTagsIntoCategories(t *testing.T) {
	got := mergeCategoryLists(
		[]string{"MILF", "uncategorized", "Anal"},
		[]string{" anal ", "POV", "", "Milf"},
		[]string{"pov", "Outdoor", "uncategorized"},
	)
	want := []string{"milf", "anal", "pov", "outdoor"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeCategoryLists() = %#v, want %#v", got, want)
	}
}

func TestJoinStoredCategoriesNormalizesOutput(t *testing.T) {
	got := joinStoredCategories([]string{" Teen ", "teen", "Anal"})
	want := "teen,anal"

	if got != want {
		t.Fatalf("joinStoredCategories() = %q, want %q", got, want)
	}
}

func TestJoinStoredCategoriesFallsBackToUncategorized(t *testing.T) {
	got := joinStoredCategories([]string{"", " uncategorized ", "UNCATEGORIZED"})

	if got != "uncategorized" {
		t.Fatalf("joinStoredCategories() = %q, want %q", got, "uncategorized")
	}
}
