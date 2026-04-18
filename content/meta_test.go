// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package content

import "testing"

func TestSplitFrontMatter(t *testing.T) {
	raw := "---\nTitle: Foo\nAuthor: Bob\n---\nHello **world**.\n"
	y, body := SplitFrontMatter(raw)
	if y != "Title: Foo\nAuthor: Bob" {
		t.Errorf("yaml: got %q", y)
	}
	if body != "Hello **world**.\n" {
		t.Errorf("body: got %q", body)
	}
}

func TestSplitFrontMatterCStyle(t *testing.T) {
	raw := "/*\nTitle: Foo\n*/\nBody"
	y, body := SplitFrontMatter(raw)
	if y != "Title: Foo" {
		t.Errorf("yaml: got %q", y)
	}
	if body != "Body" {
		t.Errorf("body: got %q", body)
	}
}

func TestSplitFrontMatterBOM(t *testing.T) {
	raw := "\uFEFF---\nTitle: Foo\n---\nBody"
	y, _ := SplitFrontMatter(raw)
	if y != "Title: Foo" {
		t.Errorf("BOM: got %q", y)
	}
}

func TestSplitFrontMatterNone(t *testing.T) {
	raw := "# Just Markdown\n"
	y, body := SplitFrontMatter(raw)
	if y != "" {
		t.Errorf("yaml should be empty, got %q", y)
	}
	if body != raw {
		t.Errorf("body should equal input")
	}
}

func TestParseMetaLowercasesKeys(t *testing.T) {
	m, err := ParseMeta("Title: Foo\nAuthor: Bob\nCustom-Key: Yes", "")
	if err != nil {
		t.Fatal(err)
	}
	if m["title"] != "Foo" {
		t.Errorf("title: got %v", m["title"])
	}
	if m["author"] != "Bob" {
		t.Errorf("author: got %v", m["author"])
	}
	if _, has := m["custom-key"]; !has {
		t.Errorf("custom-key should be preserved lowercased")
	}
}

func TestParseMetaDateDerivesTime(t *testing.T) {
	m, err := ParseMeta("Title: X\nDate: 2020-01-02", "%Y")
	if err != nil {
		t.Fatal(err)
	}
	if m["time"].(int64) == 0 {
		t.Errorf("time should be derived")
	}
	if m["date_formatted"] != "2020" {
		t.Errorf("date_formatted: got %v", m["date_formatted"])
	}
}

func TestParseMetaHiddenCoercion(t *testing.T) {
	m, _ := ParseMeta("Title: X\nHidden: true", "")
	if m["hidden"] != true {
		t.Errorf("hidden true: got %v", m["hidden"])
	}
	m, _ = ParseMeta("Title: X\nHidden: false", "")
	if m["hidden"] != false {
		t.Errorf("hidden false: got %v", m["hidden"])
	}
}
