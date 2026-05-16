package staticasset

import "testing"

func TestIsVendoredStaticAsset(t *testing.T) {
	for _, pathValue := range []string{
		"/static/katex/katex.min.js",
		"/static/katex/fonts/KaTeX_Main-Regular.woff2",
		"/static/mermaid/mermaid.min.js?v=dev",
		"/static/svg-pan-zoom/svg-pan-zoom.min.js",
		"/static/site/tufte-css/tufte.min.css",
	} {
		t.Run(pathValue, func(t *testing.T) {
			if !IsVendored(pathValue) {
				t.Fatalf("expected %q to be vendored", pathValue)
			}
			if ShouldUseBuildVersion(pathValue) {
				t.Fatalf("expected %q not to use build version query", pathValue)
			}
		})
	}
}

func TestAppOwnedStaticAssetsUseBuildVersion(t *testing.T) {
	for _, pathValue := range []string{
		"/static/site/style.css",
		"/static/site/content-assets.js",
		"/static/site/main.js",
		"/static/sui/sui.css",
		"/static/dash/main.js",
		"/static/seditor/dist/seditor.min.js",
		"/static/ceditor/dist/ceditor.min.js",
		"/static/favicon.svg",
	} {
		t.Run(pathValue, func(t *testing.T) {
			if IsVendored(pathValue) {
				t.Fatalf("expected %q not to be vendored", pathValue)
			}
			if !ShouldUseBuildVersion(pathValue) {
				t.Fatalf("expected %q to use build version query", pathValue)
			}
		})
	}
}
