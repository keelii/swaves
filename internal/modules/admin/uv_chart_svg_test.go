package admin

import (
	"strings"
	"testing"
)

func TestBuildUVChartSVG(t *testing.T) {
	svg, err := BuildUVChartSVG(UVChartUIData{
		Width:  240,
		Height: 32,
		Points: []UVChartPoint{
			{Label: "19 Jan", UV: 100},
			{Label: "20 Jan", UV: 120},
			{Label: "21 Jan", UV: 80},
		},
	})
	if err != nil {
		t.Fatalf("BuildUVChartSVG returned error: %v", err)
	}

	if !strings.Contains(svg, `viewBox="0 0 240 32"`) {
		t.Fatalf("unexpected viewBox, svg=%s", svg)
	}
	if strings.Count(svg, "<line ") != 3 {
		t.Fatalf("expected 3 grid lines, got %d", strings.Count(svg, "<line "))
	}
	if strings.Count(svg, "<circle ") != 3 {
		t.Fatalf("expected 3 circles, got %d", strings.Count(svg, "<circle "))
	}
	if strings.Count(svg, "<rect ") != 3 {
		t.Fatalf("expected 3 hover rects, got %d", strings.Count(svg, "<rect "))
	}
	if !strings.Contains(svg, `pointer-events="all"`) {
		t.Fatalf("expected hover rect pointer-events, svg=%s", svg)
	}
	if !strings.Contains(svg, `fill-opacity="0.001"`) {
		t.Fatalf("expected non-zero fill opacity on hover rects, svg=%s", svg)
	}
	if !strings.Contains(svg, `data-label="20 Jan"`) {
		t.Fatalf("expected label metadata in svg, svg=%s", svg)
	}
	if !strings.Contains(svg, `<title>20 Jan - 120</title>`) {
		t.Fatalf("expected hover tooltip title in svg, svg=%s", svg)
	}
	if !strings.Contains(svg, `x1="120"`) {
		t.Fatalf("expected middle x grid line at 120, svg=%s", svg)
	}
}

func TestBuildUVChartSVGDefaultsAndSinglePoint(t *testing.T) {
	svg, err := BuildUVChartSVG(UVChartUIData{
		Points: []UVChartPoint{{UV: 0}},
	})
	if err != nil {
		t.Fatalf("BuildUVChartSVG returned error: %v", err)
	}

	if !strings.Contains(svg, `viewBox="0 0 240 32"`) {
		t.Fatalf("expected default viewBox, svg=%s", svg)
	}
	if !strings.Contains(svg, `width="240"`) {
		t.Fatalf("expected single hover rect width 240, svg=%s", svg)
	}
	if strings.Contains(svg, "NaN") {
		t.Fatalf("svg should not contain NaN, svg=%s", svg)
	}
}

func TestBuildUVChartSVGValidation(t *testing.T) {
	if _, err := BuildUVChartSVG(UVChartUIData{}); err == nil {
		t.Fatal("expected error when points is empty")
	}

	if _, err := BuildUVChartSVG(UVChartUIData{Points: []UVChartPoint{{UV: -1}}}); err == nil {
		t.Fatal("expected error when uv is negative")
	}
}

func TestBuildUVChartSVGCustomTooltip(t *testing.T) {
	svg, err := BuildUVChartSVG(UVChartUIData{
		Points: []UVChartPoint{
			{Label: "A", UV: 1, Tooltip: "自定义提示"},
		},
	})
	if err != nil {
		t.Fatalf("BuildUVChartSVG returned error: %v", err)
	}
	if !strings.Contains(svg, `<title>自定义提示</title>`) {
		t.Fatalf("expected custom tooltip in svg, svg=%s", svg)
	}
}
