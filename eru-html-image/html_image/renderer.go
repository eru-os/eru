package html_image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	FormatPng                   = "png"
	FormatJpeg                  = "jpeg"
	DefaultWidth                = 1280
	DefaultHeight               = 1024
	DefaultScale                = 2.0
	DefaultQuality              = 90
	DefaultTimeoutSeconds       = 30
	DefaultMaxConcurrentRenders = 4
	MaxScale                    = 4.0
	MaxImageDimension           = 16384
)

type RenderRequest struct {
	Html            string  `json:"html"`
	Width           int     `json:"width,omitempty"`
	Height          int     `json:"height,omitempty"`
	Scale           float64 `json:"scale,omitempty"`
	Format          string  `json:"format,omitempty"`
	Quality         int     `json:"quality,omitempty"`
	Selector        string  `json:"selector,omitempty"`
	BackgroundColor string  `json:"background_color,omitempty"`
	TimeoutSeconds  int     `json:"timeout_seconds,omitempty"`
}

type Renderer struct {
	allocatorCtx    context.Context
	allocatorCancel context.CancelFunc
	browserCtx      context.Context
	browserCancel   context.CancelFunc
	semaphore       chan struct{}
	mu              sync.Mutex
}

func NewRenderer(ctx context.Context) (*Renderer, error) {
	logs.WithContext(ctx).Info("NewRenderer - Start")

	maxRenders := DefaultMaxConcurrentRenders
	if v := os.Getenv("MAX_CONCURRENT_RENDERS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxRenders = parsed
		} else {
			logs.WithContext(ctx).Warn(fmt.Sprint("invalid MAX_CONCURRENT_RENDERS ", v, " - using default"))
		}
	}

	r := &Renderer{
		semaphore: make(chan struct{}, maxRenders),
	}
	if err := r.start(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Renderer) start(ctx context.Context) error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("force-color-profile", "srgb"),
		chromedp.Flag("font-render-hinting", "none"),
	)
	if execPath := chromiumPath(); execPath != "" {
		logs.WithContext(ctx).Info(fmt.Sprint("using chromium at ", execPath))
		opts = append(opts, chromedp.ExecPath(execPath))
	}

	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocatorCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocatorCancel()
		logs.WithContext(ctx).Error(fmt.Sprint("failed to start chromium : ", err.Error()))
		return err
	}

	r.allocatorCtx = allocatorCtx
	r.allocatorCancel = allocatorCancel
	r.browserCtx = browserCtx
	r.browserCancel = browserCancel
	return nil
}

func (r *Renderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.browserCancel != nil {
		r.browserCancel()
	}
	if r.allocatorCancel != nil {
		r.allocatorCancel()
	}
}

func (r *Renderer) browser(ctx context.Context) (context.Context, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.browserCtx != nil && r.browserCtx.Err() == nil {
		return r.browserCtx, nil
	}
	logs.WithContext(ctx).Warn("chromium is not running - restarting")
	if r.browserCancel != nil {
		r.browserCancel()
	}
	if r.allocatorCancel != nil {
		r.allocatorCancel()
	}
	if err := r.start(ctx); err != nil {
		return nil, err
	}
	return r.browserCtx, nil
}

func chromiumPath() string {
	for _, env := range []string{"CHROMIUM_PATH", "CHROME_PATH"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}

func (rr *RenderRequest) setDefaults() {
	if rr.Width <= 0 {
		rr.Width = DefaultWidth
	}
	if rr.Height <= 0 {
		rr.Height = DefaultHeight
	}
	if rr.Scale <= 0 {
		rr.Scale = DefaultScale
	}
	if rr.Scale > MaxScale {
		rr.Scale = MaxScale
	}
	rr.Format = strings.ToLower(rr.Format)
	if rr.Format != FormatJpeg {
		rr.Format = FormatPng
	}
	if rr.Quality <= 0 || rr.Quality > 100 {
		rr.Quality = DefaultQuality
	}
	if rr.TimeoutSeconds <= 0 {
		rr.TimeoutSeconds = DefaultTimeoutSeconds
	}
}

func (r *Renderer) Render(ctx context.Context, req RenderRequest) (image []byte, format string, err error) {
	logs.WithContext(ctx).Debug("Render - Start")
	if strings.TrimSpace(req.Html) == "" {
		err = errors.New("html is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, "", err
	}
	req.setDefaults()

	select {
	case r.semaphore <- struct{}{}:
		defer func() { <-r.semaphore }()
	case <-ctx.Done():
		err = ctx.Err()
		logs.WithContext(ctx).Error(err.Error())
		return nil, "", err
	}

	browserCtx, err := r.browser(ctx)
	if err != nil {
		return nil, "", err
	}

	tabCtx, tabCancel := chromedp.NewContext(browserCtx)
	defer tabCancel()
	timeoutCtx, timeoutCancel := context.WithTimeout(tabCtx, time.Duration(req.TimeoutSeconds)*time.Second)
	defer timeoutCancel()

	err = chromedp.Run(timeoutCtx,
		chromedp.EmulateViewport(int64(req.Width), int64(req.Height)),
		chromedp.Navigate("about:blank"),
		setDocumentContent(req.Html),
		chromedp.WaitReady("body", chromedp.ByQuery),
		setBackgroundColor(req.BackgroundColor),
		waitForFonts(),
		captureScreenshot(&req, &image),
	)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("error while rendering html to image : ", err.Error()))
		return nil, "", err
	}
	return image, req.Format, nil
}

func setDocumentContent(htmlDoc string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		frameTree, err := page.GetFrameTree().Do(ctx)
		if err != nil {
			return err
		}
		return page.SetDocumentContent(frameTree.Frame.ID, htmlDoc).Do(ctx)
	})
}

func setBackgroundColor(color string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if strings.TrimSpace(color) == "" {
			return nil
		}
		colorJson, err := json.Marshal(color)
		if err != nil {
			return err
		}
		script := fmt.Sprintf(`(() => { const c = %s; document.documentElement.style.background = c; document.body.style.background = c; return true })()`, string(colorJson))
		var applied bool
		return chromedp.Evaluate(script, &applied).Do(ctx)
	})
}

func waitForFonts() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		var ready bool
		return chromedp.Evaluate(`document.fonts.ready.then(() => true)`, &ready, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}).Do(ctx)
	})
}

func captureScreenshot(req *RenderRequest, image *[]byte) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		clip, err := contentClip(ctx, req.Selector)
		if err != nil {
			return err
		}

		scale := req.Scale
		if maxScale := maxScaleFor(clip.Width, clip.Height); scale > maxScale {
			logs.WithContext(ctx).Warn(fmt.Sprint("scale reduced from ", scale, " to ", maxScale, " to stay within image size limits"))
			scale = maxScale
		}
		clip.Scale = scale

		screenshotFormat := page.CaptureScreenshotFormatPng
		if req.Format == FormatJpeg {
			screenshotFormat = page.CaptureScreenshotFormatJpeg
		}

		capture := page.CaptureScreenshot().
			WithFormat(screenshotFormat).
			WithCaptureBeyondViewport(true).
			WithFromSurface(true).
			WithClip(clip)
		if req.Format == FormatJpeg {
			capture = capture.WithQuality(int64(req.Quality))
		}

		buf, err := capture.Do(ctx)
		if err != nil {
			return err
		}
		*image = buf
		return nil
	})
}

func contentClip(ctx context.Context, selector string) (*page.Viewport, error) {
	selectorJson, err := json.Marshal(selector)
	if err != nil {
		return nil, err
	}
	script := fmt.Sprintf(`(() => {
	const sel = %s;
	const el = sel ? document.querySelector(sel) : document.body;
	if (!el) { return null }
	const rect = el.getBoundingClientRect();
	const width = Math.max(rect.width, el.scrollWidth, el.offsetWidth);
	const height = Math.max(rect.height, el.scrollHeight, el.offsetHeight);
	return [rect.left + window.scrollX, rect.top + window.scrollY, width, height];
})()`, string(selectorJson))

	var rect []float64
	if err = chromedp.Evaluate(script, &rect).Do(ctx); err != nil {
		return nil, err
	}
	if len(rect) != 4 {
		return nil, errors.New(fmt.Sprint("element not found for selector ", selector))
	}
	width := math.Ceil(rect[2])
	height := math.Ceil(rect[3])
	if width <= 0 || height <= 0 {
		return nil, errors.New("rendered content has no visible size")
	}
	return &page.Viewport{X: rect[0], Y: rect[1], Width: width, Height: height}, nil
}

func maxScaleFor(width float64, height float64) float64 {
	maxScale := math.Min(MaxImageDimension/width, MaxImageDimension/height)
	if maxScale < 1 {
		return maxScale
	}
	return math.Floor(maxScale*100) / 100
}

func ContentType(format string) string {
	if strings.ToLower(format) == FormatJpeg {
		return "image/jpeg"
	}
	return "image/png"
}
