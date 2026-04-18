// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package pigo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// End-to-end test: boot a Site against testdata/site/, issue HTTP requests via
// httptest, and assert on rendered output.
func TestSiteRenderTwig(t *testing.T) {
	site, err := New(Options{RootDir: "testdata/site"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	cases := []struct {
		path     string
		status   int
		contains []string
	}{
		{"/", 200, []string{"TITLE:Home", "SITE:Test Site", "CONTENT:<p>Welcome to <strong>pigo</strong>", "PAGES:about,index,sub/page,"}},
		{"/about", 200, []string{"TITLE:About"}},
		{"/sub/page", 200, []string{"TITLE:Sub Page"}},
		{"/missing", 404, []string{"TITLE:Not Found"}},
	}
	for _, c := range cases {
		res, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d\nbody:\n%s", c.path, res.StatusCode, c.status, body)
			continue
		}
		for _, want := range c.contains {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s: body missing %q\nbody:\n%s", c.path, want, body)
			}
		}
	}
}

func TestSiteRenderGoTemplate(t *testing.T) {
	// Same site, but switch engine + theme.
	site, err := New(Options{
		RootDir: "testdata/site",
	})
	if err != nil {
		t.Fatal(err)
	}
	site.cfg.TemplateEngine = "go"
	site.cfg.Theme = "test-theme-go"
	site.themeDir = "testdata/site/themes/test-theme-go"

	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d\nbody:\n%s", res.StatusCode, body)
	}
	for _, want := range []string{"TITLE:Home", "SITE:Test Site", "CONTENT:<p>Welcome to <strong>pigo</strong>"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body missing %q\nbody:\n%s", want, body)
		}
	}
}
