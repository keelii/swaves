package view

import (
	"encoding/json"
	"errors"
	"fmt"
	HTML "html"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/shared/share"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	templatecore "github.com/gofiber/template/v2"
	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

type FiberView struct {
	env           *minijinja.Environment
	templateRoot  string
	clearOnRender bool
	mu            sync.Mutex
}

var (
	errFiberViewNil = errors.New("fiber view engine is nil")
)

func renderLucideIconSVG(name, size string) string {
	template, ok := lucideSVGByName[name]
	if !ok {
		return ""
	}
	svg := fmt.Sprintf(template, HTML.EscapeString(size))
	dataName := HTML.EscapeString(name)
	if strings.Contains(svg, "<svg ") {
		return strings.Replace(svg, "<svg ", fmt.Sprintf(`<svg data-name="%s" `, dataName), 1)
	}
	return svg
}

var lucideSVGByName = map[string]string{
	"trash-2":             `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-trash2-icon lucide-trash-2" aria-hidden="true"><path d="M10 11v6"/><path d="M14 11v6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>`,
	"trash":               `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-trash-icon lucide-trash"><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>`,
	"chevron-left":        `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-left-icon lucide-chevron-left"><path d="m15 18-6-6 6-6"/></svg>`,
	"chevron-right":       `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-right-icon lucide-chevron-right"><path d="m9 18 6-6-6-6"/></svg>`,
	"chevron-up":          `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-up-icon lucide-chevron-up"><path d="m18 15-6-6-6 6"/></svg>`,
	"chevron-down":        `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-down-icon lucide-chevron-down"><path d="m6 9 6 6 6-6"/></svg>`,
	"chevrons-up-down":    `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevrons-up-down-icon lucide-chevrons-up-down"><path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/></svg>`,
	"x":                   `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-x-icon lucide-x"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`,
	"import":              `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-import-icon lucide-import" aria-hidden="true"><path d="M12 3v12"/><path d="m8 11 4 4 4-4"/><path d="M8 5H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-4"/></svg>`,
	"list":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-list-icon lucide-list" aria-hidden="true"><path d="M3 5h.01"/> <path d="M3 12h.01"/> <path d="M3 19h.01"/> <path d="M8 5h13"/> <path d="M8 12h13"/> <path d="M8 19h13"/></svg>`,
	"list-tree":           `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-list-tree-icon lucide-list-tree" aria-hidden="true"><path d="M8 5h13"/> <path d="M13 12h8"/> <path d="M13 19h8"/> <path d="M3 10a2 2 0 0 0 2 2h3"/> <path d="M3 5v12a2 2 0 0 0 2 2h3"/></svg>`,
	"layout-dashboard":    `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-layout-dashboard-icon lucide-layout-dashboard"><rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/></svg>`,
	"image":               `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-image-icon lucide-image"><rect width="18" height="18" x="3" y="3" rx="2" ry="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"/></svg>`,
	"activity":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-activity-icon lucide-activity"><path d="M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2"/></svg>`,
	"database":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-database-icon lucide-database"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5V19A9 3 0 0 0 21 19V5"/><path d="M3 12A9 3 0 0 0 21 12"/></svg>`,
	"panel-left":          `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-panel-left-icon lucide-panel-left"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/></svg>`,
	"messages-square":     `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-messages-square-icon lucide-messages-square"><path d="M16 10a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 14.286V4a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/><path d="M20 9a2 2 0 0 1 2 2v10.286a.71.71 0 0 1-1.212.502l-2.202-2.202A2 2 0 0 0 17.172 19H10a2 2 0 0 1-2-2v-1"/></svg>`,
	"moon":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-moon-icon lucide-moon"><path d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"/></svg>`,
	"sun":                 `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-sun-icon lucide-sun"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>`,
	"quote":               `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-quote-icon lucide-quote"><path d="M16 3a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2 1 1 0 0 1 1 1v1a2 2 0 0 1-2 2 1 1 0 0 0-1 1v2a1 1 0 0 0 1 1 6 6 0 0 0 6-6V5a2 2 0 0 0-2-2z"/><path d="M5 3a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2 1 1 0 0 1 1 1v1a2 2 0 0 1-2 2 1 1 0 0 0-1 1v2a1 1 0 0 0 1 1 6 6 0 0 0 6-6V5a2 2 0 0 0-2-2z"/></svg>`,
	"bell-dot":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-bell-dot-icon lucide-bell-dot"><path d="M10.268 21a2 2 0 0 0 3.464 0"/><path d="M11.68 2.009A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673c-.824-.85-1.678-1.731-2.21-3.348"/><circle cx="18" cy="5" r="3"/></svg>`,
	"bell":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-bell-icon lucide-bell"><path d="M10.268 21a2 2 0 0 0 3.464 0"/><path d="M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326"/></svg>`,
	"heading-1":           `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-heading1-icon lucide-heading-1"><path d="M4 12h8"/><path d="M4 18V6"/><path d="M12 18V6"/><path d="m17 12 3-2v8"/></svg>`,
	"bold":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-bold-icon lucide-bold"><path d="M6 12h9a4 4 0 0 1 0 8H7a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h7a4 4 0 0 1 0 8"/></svg>`,
	"italic":              `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-italic-icon lucide-italic"><line x1="19" x2="10" y1="4" y2="4"/><line x1="14" x2="5" y1="20" y2="20"/><line x1="15" x2="9" y1="4" y2="20"/></svg>`,
	"underline":           `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-underline-icon lucide-underline"><path d="M6 4v6a6 6 0 0 0 12 0V4"/><line x1="4" x2="20" y1="20" y2="20"/></svg>`,
	"list-checks":         `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-list-checks-icon lucide-list-checks"><path d="M13 5h8"/><path d="M13 12h8"/><path d="M13 19h8"/><path d="m3 17 2 2 4-4"/><path d="m3 7 2 2 4-4"/></svg>`,
	"arrow-big-left":      `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-big-left-icon lucide-arrow-big-left" aria-hidden="true"><path d="M13 9a1 1 0 0 1-1-1V5.061a1 1 0 0 0-1.811-.75l-6.835 6.836a1.207 1.207 0 0 0 0 1.707l6.835 6.835a1 1 0 0 0 1.811-.75V16a1 1 0 0 1 1-1h6a1 1 0 0 0 1-1v-4a1 1 0 0 0-1-1z"/></svg>`,
	"arrow-left":          `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-icon lucide-arrow-left" aria-hidden="true"><path d="m12 19-7-7 7-7"/> <path d="M19 12H5"/> </svg>`,
	"arrow-right":         `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-right-icon lucide-arrow-right" aria-hidden="true"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>`,
	"arrow-left-to-line":  `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-to-line-icon lucide-arrow-left-to-line"><path d="M3 19V5"/><path d="m13 6-6 6 6 6"/><path d="M7 12h14"/></svg>`,
	"arrow-right-to-line": `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-right-to-line-icon lucide-arrow-right-to-line"><path d="M17 12H3"/><path d="m11 18 6-6-6-6"/><path d="M21 5v14"/></svg>`,
	"archive":             `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-archive-icon lucide-archive" aria-hidden="true"><rect width="20" height="5" x="2" y="3" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/></svg>`,
	"clipboard":           `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-clipboard-icon lucide-clipboard" aria-hidden="true"><rect width="8" height="4" x="8" y="2" rx="1" ry="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/></svg>`,
	"square-plus":         `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-square-plus-icon lucide-square-plus"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M8 12h8"/><path d="M12 8v8"/></svg>`,
	"square-check":        `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-square-check-icon lucide-square-check"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="m9 12 2 2 4-4"/></svg>`,
	"plus":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-plus-icon lucide-plus" aria-hidden="true"> <path d="M5 12h14"/> <path d="M12 5v14"/> </svg>`,
	"save":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-save-icon lucide-save" aria-hidden="true"> <path d="M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z"/> <path d="M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7"/> <path d="M7 3v4a1 1 0 0 0 1 1h7"/> </svg>`,
	"terminal":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-terminal-icon lucide-terminal" aria-hidden="true"> <path d="M12 19h8"/> <path d="m4 17 6-6-6-6"/> </svg>`,
	"undo":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-undo-icon lucide-undo" aria-hidden="true"> <path d="M3 7v6h6"/> <path d="M21 17a9 9 0 0 0-9-9 9 9 0 0 0-6 2.3L3 13"/> </svg>`,
	"square-pen":          `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-square-pen-icon lucide-square-pen" aria-hidden="true"> <path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/> <path d="M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z"/> </svg>`,
	"heart":               `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-heart-icon lucide-heart"><path d="M2 9.5a5.5 5.5 0 0 1 9.591-3.676.56.56 0 0 0 .818 0A5.49 5.49 0 0 1 22 9.5c0 2.29-1.5 4-3 5.5l-5.492 5.313a2 2 0 0 1-3 .019L5 15c-1.5-1.5-3-3.2-3-5.5"/></svg>`,
	"book-open-check":     `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-book-open-check-icon lucide-book-open-check"><path d="M12 21V7"/><path d="m16 12 2 2 4-4"/><path d="M22 6V4a1 1 0 0 0-1-1h-5a4 4 0 0 0-4 4 4 4 0 0 0-4-4H3a1 1 0 0 0-1 1v13a1 1 0 0 0 1 1h6a3 3 0 0 1 3 3 3 3 0 0 1 3-3h6a1 1 0 0 0 1-1v-1.3"/></svg>`,
	"link-2":              `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-link2-icon lucide-link-2"><path d="M9 17H7A5 5 0 0 1 7 7h2"/><path d="M15 7h2a5 5 0 1 1 0 10h-2"/><line x1="8" x2="16" y1="12" y2="12"/></svg>`,
	"link":                `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-link-icon lucide-link"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`,
	"setting":             `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-settings-icon lucide-settings"><path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915"/><circle cx="12" cy="12" r="3"/></svg>`,
	"log-out":             `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-log-out-icon lucide-log-out"><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/></svg>`,
}

func NewViewEngine(dir string, reload bool) (fiber.Views, func(app *fiber.App)) {
	urlForStore := share.NewURLForStore()
	view := newMiniJinjaView(dir, reload)
	registerViewFunc(view.env, urlForStore.URLFor)
	initURLResolver := func(app *fiber.App) {
		urlForStore.SetResolver(newURLForResolver(app))
	}
	return view, initURLResolver
}

func newMiniJinjaView(templateRoot string, clearOnRender bool) *FiberView {
	env := minijinja.NewEnvironment()
	env.SetDebug(true)
	env.SetUndefinedBehavior(minijinja.UndefinedLenient)
	env.SetDebug(clearOnRender)
	env.SetLoader(newMiniJinjaTemplateLoader(templateRoot))
	env.SetPathJoinCallback(resolveTemplateImportPath)
	return &FiberView{
		env:           env,
		templateRoot:  templateRoot,
		clearOnRender: clearOnRender,
	}
}

func (v *FiberView) Load() error {
	if v == nil || v.env == nil {
		return errFiberViewNil
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	templateNames, err := collectTemplateNames(v.templateRoot)
	if err != nil {
		return err
	}

	v.env.ClearTemplates()
	for _, name := range templateNames {
		if _, err := v.env.GetTemplate(name); err != nil {
			return fmt.Errorf("load template %q failed: %w", name, err)
		}
	}
	return nil
}

func (v *FiberView) Render(out io.Writer, name string, binding any, layout ...string) error {
	if v == nil || v.env == nil {
		return errFiberViewNil
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	acquired := templatecore.AcquireViewContext(binding)

	if v.clearOnRender {
		v.env.ClearTemplates()
	}

	templateName, err := normalizeTemplateName(name)
	if err != nil {
		return err
	}
	_ = layout

	tmpl, err := v.env.GetTemplate(templateName)
	if err != nil {
		return fmt.Errorf("load template %q failed: %w", templateName, err)
	}
	context := make(map[string]value.Value, len(acquired))
	for key, raw := range acquired {
		context[key] = value.FromAny(wrapMapLookup(raw))
	}
	return tmpl.RenderToWrite(context, out)
}

func newMiniJinjaTemplateLoader(templateRoot string) minijinja.LoaderFunc {
	return func(name string) (string, error) {
		normalizedName, err := normalizeTemplateName(name)
		if err != nil {
			return "", err
		}

		rootPath, err := filepath.Abs(templateRoot)
		if err != nil {
			return "", err
		}
		templatePath := filepath.Join(rootPath, filepath.FromSlash(normalizedName))
		cleanedPath := filepath.Clean(templatePath)
		if !strings.HasPrefix(cleanedPath, rootPath+string(filepath.Separator)) && cleanedPath != rootPath {
			return "", fmt.Errorf("template path escapes root: %s", name)
		}

		content, err := os.ReadFile(cleanedPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", minijinja.NewError(minijinja.ErrTemplateNotFound, normalizedName)
			}
			return "", err
		}
		return string(content), nil
	}
}

func resolveTemplateImportPath(name string, parent string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		return name
	}

	if strings.HasPrefix(name, "/") {
		cleaned := path.Clean(name)
		return strings.TrimPrefix(cleaned, "/")
	}

	if strings.HasPrefix(name, "./") || strings.HasPrefix(name, "../") || name == "." || name == ".." {
		parentDir := path.Dir(strings.TrimSpace(parent))
		cleaned := path.Clean(name)
		return path.Clean(path.Join(parentDir, cleaned))
	}

	cleaned := path.Clean(name)
	if cleaned == "." {
		return ""
	}

	if strings.Contains(cleaned, "/") {
		return cleaned
	}

	parentDir := path.Dir(strings.TrimSpace(parent))
	return path.Clean(path.Join(parentDir, cleaned))
}

func collectTemplateNames(templateRoot string) ([]string, error) {
	rootPath, err := filepath.Abs(templateRoot)
	if err != nil {
		return nil, err
	}

	var names []string
	err = filepath.WalkDir(rootPath, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".html") {
			return nil
		}
		relativePath, err := filepath.Rel(rootPath, filePath)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func normalizeTemplateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("template name is empty")
	}
	normalized := strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, "\\", "/")), "./")
	if normalized == "." || normalized == "" {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	if strings.HasPrefix(normalized, "../") || normalized == ".." {
		return "", fmt.Errorf("template name %q points outside root", name)
	}
	if path.Ext(normalized) != ".html" {
		return "", fmt.Errorf("template name %q must end with .html", name)
	}
	return normalized, nil
}

type templateMapLookup struct {
	data any
}

func (m *templateMapLookup) GetAttr(name string) value.Value {
	item, ok := safeLookup(m.data, name)
	if !ok {
		return value.Undefined()
	}
	return value.FromAny(wrapMapLookup(item))
}

func (m *templateMapLookup) GetItem(key value.Value) value.Value {
	item, ok := safeLookup(m.data, key.Raw())
	if !ok {
		return value.Undefined()
	}
	return value.FromAny(wrapMapLookup(item))
}

func wrapMapLookup(raw any) any {
	if raw == nil {
		return nil
	}

	target := reflect.ValueOf(raw)
	for target.Kind() == reflect.Interface || target.Kind() == reflect.Ptr {
		if target.IsNil() {
			return raw
		}
		target = target.Elem()
	}

	if target.Kind() != reflect.Map || !target.IsValid() || target.IsNil() {
		return raw
	}
	if target.Type().Key().Kind() == reflect.String {
		return raw
	}
	return &templateMapLookup{data: raw}
}

func registerViewFunc(env *minijinja.Environment, urlFor func(name string, params map[string]string, query map[string]string) string) {
	registerViewGlobals(env)
	registerViewFilters(env)
	registerViewFunctions(env, urlFor)
}

func renderHTMLAttrs(raw any) string {
	attrs := toStringAnyMap(raw)
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(attrs))
	for _, key := range keys {
		item := attrs[key]
		switch typed := item.(type) {
		case bool:
			if typed {
				parts = append(parts, key)
			}
			continue
		}
		text := HTML.EscapeString(toStringValue(item))
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, text))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func safeLookup(container any, key any) (any, bool) {
	if container == nil {
		return nil, false
	}
	parseIndex := func(raw any) (int, bool) {
		switch typed := raw.(type) {
		case int:
			return typed, true
		case int8:
			return int(typed), true
		case int16:
			return int(typed), true
		case int32:
			return int(typed), true
		case int64:
			return int(typed), true
		case uint:
			return int(typed), true
		case uint8:
			return int(typed), true
		case uint16:
			return int(typed), true
		case uint32:
			return int(typed), true
		case uint64:
			return int(typed), true
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err != nil {
				return 0, false
			}
			return parsed, true
		case json.Number:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed.String()))
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
	}

	switch values := container.(type) {
	case map[string]any:
		value, ok := values[fmt.Sprint(key)]
		return value, ok
	case map[string]string:
		value, ok := values[fmt.Sprint(key)]
		if !ok {
			return nil, false
		}
		return value, true
	case []any:
		idx, ok := parseIndex(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return values[idx], true
	case []string:
		idx, ok := parseIndex(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return values[idx], true
	case string:
		idx, ok := parseIndex(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return string(values[idx]), true
	}

	value := reflect.ValueOf(container)
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Map:
		mapKey := reflect.ValueOf(key)
		keyType := value.Type().Key()
		if !mapKey.IsValid() {
			return nil, false
		}
		if mapKey.Type().AssignableTo(keyType) {
			lookup := value.MapIndex(mapKey)
			if !lookup.IsValid() {
				return nil, false
			}
			return lookup.Interface(), true
		}
		if mapKey.Type().ConvertibleTo(keyType) {
			lookup := value.MapIndex(mapKey.Convert(keyType))
			if !lookup.IsValid() {
				return nil, false
			}
			return lookup.Interface(), true
		}
		if keyType.Kind() == reflect.String {
			lookup := value.MapIndex(reflect.ValueOf(fmt.Sprint(key)))
			if !lookup.IsValid() {
				return nil, false
			}
			return lookup.Interface(), true
		}
		return nil, false
	case reflect.Slice, reflect.Array:
		idx, ok := parseIndex(key)
		if !ok || idx < 0 || idx >= value.Len() {
			return nil, false
		}
		return value.Index(idx).Interface(), true
	case reflect.Struct:
		fieldName := strings.TrimSpace(fmt.Sprint(key))
		if fieldName == "" {
			return nil, false
		}
		field := value.FieldByName(fieldName)
		if !field.IsValid() || !field.CanInterface() {
			return nil, false
		}
		return field.Interface(), true
	default:
		return nil, false
	}
}

func toStringMap(raw interface{}) map[string]string {
	if raw == nil {
		return nil
	}

	result := map[string]string{}
	switch values := raw.(type) {
	case map[string]string:
		for k, v := range values {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			result[key] = strings.TrimSpace(v)
		}
	case map[string]interface{}:
		for k, v := range values {
			key := strings.TrimSpace(k)
			if key == "" || v == nil {
				continue
			}
			result[key] = strings.TrimSpace(fmt.Sprint(v))
		}
	case map[string]value.Value:
		for k, v := range values {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			rawValue := v.Raw()
			if rawValue == nil {
				continue
			}
			result[key] = strings.TrimSpace(fmt.Sprint(rawValue))
		}
	default:
		return nil
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func toStringAnyMap(raw interface{}) map[string]any {
	if raw == nil {
		return nil
	}

	result := map[string]any{}
	switch values := raw.(type) {
	case map[string]any:
		for key, item := range values {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			result[trimmed] = item
		}
	case map[string]string:
		for key, item := range values {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			result[trimmed] = item
		}
	case map[string]value.Value:
		for key, item := range values {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			result[trimmed] = item.Raw()
		}
	default:
		return nil
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func toStringValue(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if value, ok := raw.(string); ok {
		return value
	}
	if value, ok := raw.(fmt.Stringer); ok {
		return value.String()
	}
	return fmt.Sprint(raw)
}

// relativeTimeString 将 Unix 时间戳转为相对时间中文描述
func relativeTimeString(ts int64) string {
	now := time.Now().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
		if diff < 60 {
			return "刚刚"
		}
		return time.Unix(ts, 0).Format(config.BaseTimeFormat)
	}
	switch {
	case diff < 60:
		return "刚刚"
	case diff < 3600:
		return fmt.Sprintf("%d分钟前", diff/60)
	case diff < 86400:
		return fmt.Sprintf("%d小时前", diff/3600)
	case diff < 30*86400:
		return fmt.Sprintf("%d天前", diff/86400)
	case diff < 365*86400:
		return fmt.Sprintf("%d月前", diff/(30*86400))
	default:
		return fmt.Sprintf("%d年前", diff/(365*86400))
	}
}

func formatHumanSize(raw interface{}) string {
	bytes, ok := normalizeBytes(raw)
	if !ok {
		return "-"
	}
	if bytes < 1024 {
		return strconv.FormatInt(int64(bytes), 10) + " B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIdx := 0
	for bytes >= 1024 && unitIdx < len(units)-1 {
		bytes /= 1024
		unitIdx++
	}

	sizeText := strconv.FormatFloat(bytes, 'f', 2, 64)
	sizeText = strings.TrimRight(strings.TrimRight(sizeText, "0"), ".")
	return sizeText + " " + units[unitIdx]
}

func normalizeBytes(raw interface{}) (float64, bool) {
	switch v := raw.(type) {
	case int:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case int32:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case float64:
		if v < 0 {
			return 0, false
		}
		return v, true
	case *int64:
		if v == nil || *v < 0 {
			return 0, false
		}
		return float64(*v), true
	default:
		return 0, false
	}
}

func newURLForResolver(app *fiber.App) func(name string, params map[string]string, query map[string]string) (string, error) {
	return func(name string, params map[string]string, query map[string]string) (string, error) {
		route := app.GetRoute(strings.TrimSpace(name))
		if strings.TrimSpace(route.Name) == "" {
			return "", fmt.Errorf("route %q not found", name)
		}

		path := route.Path
		consumedKeys := map[string]struct{}{}
		for _, paramName := range route.Params {
			value := strings.TrimSpace(params[paramName])
			if value == "" {
				return "", fmt.Errorf("route %q missing param %q", name, paramName)
			}
			consumedKeys[paramName] = struct{}{}
			path = strings.ReplaceAll(path, ":"+paramName, url.PathEscape(value))
		}

		queryValues := url.Values{}
		for key, value := range params {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			if _, ok := consumedKeys[k]; ok {
				continue
			}
			queryValues.Set(k, value)
		}
		for key, value := range query {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			queryValues.Set(k, value)
		}
		encodedQuery := queryValues.Encode()
		if encodedQuery != "" {
			path += "?" + encodedQuery
		}
		return path, nil
	}
}
