package admin_app

import (
	"bytes"
	"errors"
	htmpl "html/template"
	"math"
	"strconv"
	"strings"
)

type UVChartPoint struct {
	Label     string `json:"label"`
	UV        int    `json:"uv"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Tooltip   string `json:"tooltip,omitempty"`
}

type UVChartUIData struct {
	Points []UVChartPoint `json:"points"`

	Width               float64 `json:"width"`
	Height              float64 `json:"height"`
	PlotPadding         float64 `json:"plotPadding"`
	PreserveAspectRatio string  `json:"preserveAspectRatio"`
	ClassName           string  `json:"className"`

	GridColor       string  `json:"gridColor"`
	AreaColor       string  `json:"areaColor"`
	LineColor       string  `json:"lineColor"`
	PointColor      string  `json:"pointColor"`
	GridStrokeWidth float64 `json:"gridStrokeWidth"`
	LineStrokeWidth float64 `json:"lineStrokeWidth"`
	PointRadius     float64 `json:"pointRadius"`
}

type uvChartLine struct {
	X string
}

type uvChartCircle struct {
	X string
	Y string
}

type uvChartHoverRect struct {
	X            string
	Width        string
	Index        int
	UV           int
	Timestamp    int64
	Label        string
	Tooltip      string
	HasTimestamp bool
	HasLabel     bool
	HasTooltip   bool
}

type uvChartTemplateData struct {
	Width               string
	Height              string
	GridTop             string
	GridBottom          string
	PreserveAspectRatio string
	ClassName           string
	GridColor           string
	AreaColor           string
	LineColor           string
	PointColor          string
	GridStrokeWidth     string
	LineStrokeWidth     string
	PointRadius         string
	AreaPoints          string
	LinePoints          string
	GridLines           []uvChartLine
	Circles             []uvChartCircle
	HoverRects          []uvChartHoverRect
}

var uvChartSVGTemplate = htmpl.Must(htmpl.New("uv-chart-svg").Parse(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {{ .Width }} {{ .Height }}" preserveAspectRatio="{{ .PreserveAspectRatio }}"{{ if .ClassName }} class="{{ .ClassName }}"{{ end }}>
  <g>
    <g stroke="{{ .GridColor }}" stroke-width="{{ .GridStrokeWidth }}">
      {{- range .GridLines }}
      <line x1="{{ .X }}" x2="{{ .X }}" y1="{{ $.GridTop }}" y2="{{ $.GridBottom }}"></line>
      {{- end }}
    </g>
    <g>
      <polyline fill="{{ .AreaColor }}" points="{{ .AreaPoints }}"></polyline>
      <polyline stroke="{{ .LineColor }}" stroke-width="{{ .LineStrokeWidth }}" fill="transparent" points="{{ .LinePoints }}"></polyline>
      <g fill="{{ .PointColor }}">
        {{- range .Circles }}
        <circle cx="{{ .X }}" cy="{{ .Y }}" r="{{ $.PointRadius }}"></circle>
        {{- end }}
      </g>
      <g>
        {{- range .HoverRects }}
        <rect x="{{ .X }}" y="0" width="{{ .Width }}" height="{{ $.Height }}" fill="#fff" fill-opacity="0.001" pointer-events="all" cursor="crosshair" data-index="{{ .Index }}" data-uv="{{ .UV }}"{{ if .HasTimestamp }} data-ts="{{ .Timestamp }}"{{ end }}{{ if .HasLabel }} data-label="{{ .Label }}"{{ end }}>{{ if .HasTooltip }}<title>{{ .Tooltip }}</title>{{ end }}</rect>
        {{- end }}
      </g>
    </g>
  </g>
</svg>
`))

func BuildUVChartSVG(data UVChartUIData) (string, error) {
	if len(data.Points) == 0 {
		return "", errors.New("uv chart points is required")
	}

	for _, point := range data.Points {
		if point.UV < 0 {
			return "", errors.New("uv value must be >= 0")
		}
	}

	setUVChartDefaults(&data)

	plotLeft := data.PlotPadding
	plotRight := data.Width - data.PlotPadding
	plotTop := data.PlotPadding
	plotBottom := data.Height - data.PlotPadding
	plotWidth := plotRight - plotLeft
	plotHeight := plotBottom - plotTop

	pointCount := len(data.Points)
	step := 0.0
	if pointCount > 1 {
		step = plotWidth / float64(pointCount-1)
	}

	maxUV := 0
	for _, point := range data.Points {
		if point.UV > maxUV {
			maxUV = point.UV
		}
	}

	type chartPoint struct {
		x float64
		y float64
	}

	points := make([]chartPoint, pointCount)
	gridLines := make([]uvChartLine, 0, pointCount)
	circles := make([]uvChartCircle, 0, pointCount)
	for i, point := range data.Points {
		x := (plotLeft + plotRight) / 2
		if pointCount > 1 {
			x = plotLeft + float64(i)*step
		}

		y := plotBottom
		if maxUV > 0 {
			y = plotBottom - (float64(point.UV)/float64(maxUV))*plotHeight
		}

		points[i] = chartPoint{x: x, y: y}
		xText := svgFloat(x)
		gridLines = append(gridLines, uvChartLine{X: xText})
		circles = append(circles, uvChartCircle{X: xText, Y: svgFloat(y)})
	}

	linePoints := make([]string, 0, pointCount)
	for _, point := range points {
		linePoints = append(linePoints, svgFloat(point.x)+" "+svgFloat(point.y))
	}

	areaPoints := make([]string, 0, pointCount+2)
	areaPoints = append(areaPoints, svgFloat(plotLeft)+" "+svgFloat(plotBottom))
	areaPoints = append(areaPoints, linePoints...)
	areaPoints = append(areaPoints, svgFloat(plotRight)+" "+svgFloat(plotBottom))

	hoverRects := make([]uvChartHoverRect, 0, pointCount)
	for i, point := range points {
		rectLeft := 0.0
		rectRight := data.Width
		if pointCount > 1 {
			if i > 0 {
				rectLeft = (points[i-1].x + point.x) / 2
			}
			if i < pointCount-1 {
				rectRight = (point.x + points[i+1].x) / 2
			}
			if i == 0 {
				rectLeft = 0
			}
			if i == pointCount-1 {
				rectRight = data.Width
			}
		}
		if rectRight < rectLeft {
			rectRight = rectLeft
		}
		label := strings.TrimSpace(data.Points[i].Label)
		hasLabel := label != ""
		tooltip := strings.TrimSpace(data.Points[i].Tooltip)
		if tooltip == "" {
			tooltip = strconv.Itoa(data.Points[i].UV)
			if hasLabel {
				tooltip = label + " - " + tooltip
			}
		}
		hasTooltip := tooltip != ""
		hoverRects = append(hoverRects, uvChartHoverRect{
			X:            svgFloat(rectLeft),
			Width:        svgFloat(rectRight - rectLeft),
			Index:        i,
			UV:           data.Points[i].UV,
			Timestamp:    data.Points[i].Timestamp,
			Label:        label,
			Tooltip:      tooltip,
			HasTimestamp: data.Points[i].Timestamp > 0,
			HasLabel:     hasLabel,
			HasTooltip:   hasTooltip,
		})
	}

	templateData := uvChartTemplateData{
		Width:               svgFloat(data.Width),
		Height:              svgFloat(data.Height),
		GridTop:             svgFloat(plotTop),
		GridBottom:          svgFloat(plotBottom),
		PreserveAspectRatio: data.PreserveAspectRatio,
		ClassName:           strings.TrimSpace(data.ClassName),
		GridColor:           data.GridColor,
		AreaColor:           data.AreaColor,
		LineColor:           data.LineColor,
		PointColor:          data.PointColor,
		GridStrokeWidth:     svgFloat(data.GridStrokeWidth),
		LineStrokeWidth:     svgFloat(data.LineStrokeWidth),
		PointRadius:         svgFloat(data.PointRadius),
		AreaPoints:          strings.Join(areaPoints, " "),
		LinePoints:          strings.Join(linePoints, " "),
		GridLines:           gridLines,
		Circles:             circles,
		HoverRects:          hoverRects,
	}

	var buf bytes.Buffer
	if err := uvChartSVGTemplate.Execute(&buf, templateData); err != nil {
		return "", err
	}

	return strings.TrimSpace(buf.String()), nil
}

func setUVChartDefaults(data *UVChartUIData) {
	if data.Width <= 0 {
		data.Width = 240
	}
	if data.Height <= 0 {
		data.Height = 32
	}
	if data.PreserveAspectRatio == "" {
		data.PreserveAspectRatio = "none"
	}
	if data.GridColor == "" {
		data.GridColor = "#ecf4ff"
	}
	if data.AreaColor == "" {
		data.AreaColor = "#b9d6ff"
	}
	if data.LineColor == "" {
		data.LineColor = "#0051c3"
	}
	if data.PointColor == "" {
		data.PointColor = "#4693ff"
	}
	if data.GridStrokeWidth <= 0 {
		data.GridStrokeWidth = 0.5
	}
	if data.LineStrokeWidth <= 0 {
		data.LineStrokeWidth = 0.5
	}
	if data.PointRadius <= 0 {
		data.PointRadius = 1.3
	}
	if data.PlotPadding <= 0 {
		data.PlotPadding = data.PointRadius + data.LineStrokeWidth
	}
	maxPadding := math.Min(data.Width, data.Height) / 2
	if data.PlotPadding > maxPadding {
		data.PlotPadding = maxPadding
	}
}

func svgFloat(v float64) string {
	v = math.Round(v*1e6) / 1e6
	if math.Abs(v) < 1e-9 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
