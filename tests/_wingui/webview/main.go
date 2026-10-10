// Command webview checks on Windows the COM objects that host the browser
// of walk.WebView (internal/com since step 6): in-place activation, the
// DWebBrowserEvents2 sink, garbage collection while the browser holds
// references to them, and Dispose followed by a new WebView.
//
// It loads two local HTML pages into one WebView, disposes it, loads the
// first page into a second WebView and closes the window. It prints PASS
// or FAIL lines and exits with 1 if a check failed or did not finish in
// time.
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lpintes/walk"
)

var failed bool

func check(name string, ok bool, format string, args ...any) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failed = true
	}
	fmt.Printf("%s %s: %s\n", status, name, fmt.Sprintf(format, args...))
}

func writePage(dir, name, title string) string {
	path := filepath.Join(dir, name)
	html := "<!DOCTYPE html><html><head><title>" + title + "</title></head><body><p>" + title + "</p></body></html>"
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		fmt.Println("FAIL WriteFile:", err)
		os.Exit(1)
	}
	return (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(path)}).String()
}

// samePage reports whether u names the page at pageURL. For file URLs the
// browser reports the local path (C:\...\page1.html) in its events.
func samePage(u, pageURL string) bool {
	if strings.EqualFold(u, pageURL) {
		return true
	}
	p, err := url.Parse(pageURL)
	if err != nil || p.Scheme != "file" {
		return false
	}
	return strings.EqualFold(u, filepath.FromSlash(strings.TrimPrefix(p.Path, "/")))
}

// load loads pageURL into wv and calls done on the GUI thread when the
// document is complete, after a garbage collection.
func load(name string, wv *walk.WebView, pageURL, title string, done func()) {
	navigating := false
	wv.Navigating().Attach(func(e *walk.WebViewNavigatingEventData) {
		if samePage(e.Url(), pageURL) {
			navigating = true
		}
	})
	completed := false
	wv.DocumentCompleted().Attach(func(u string) {
		if completed || !samePage(u, pageURL) {
			return
		}
		completed = true

		// The browser holds references to the site and the frame; they
		// must survive a collection.
		runtime.GC()
		runtime.GC()

		check(name+" Navigating", navigating, "Navigating for %s", pageURL)
		check(name+" title", wv.DocumentTitle() == title, "DocumentTitle %q, want %q", wv.DocumentTitle(), title)
		cur, err := wv.URL()
		check(name+" URL", err == nil && samePage(cur, pageURL), "URL %q, err %v", cur, err)

		// Let the resize hack of DocumentCompleted run first.
		time.AfterFunc(500*time.Millisecond, func() {
			wv.Synchronize(done)
		})
	})
	if err := wv.SetURL(pageURL); err != nil {
		check(name+" SetURL", false, "%v", err)
	}
}

func main() {
	dir, err := os.MkdirTemp("", "walk-webview")
	if err != nil {
		fmt.Println("FAIL MkdirTemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	page1 := writePage(dir, "page1.html", "Walk WebView page 1")
	page2 := writePage(dir, "page2.html", "Walk WebView page 2")

	mw, err := walk.NewMainWindow()
	if err != nil {
		fmt.Println("FAIL NewMainWindow:", err)
		os.Exit(1)
	}
	mw.SetTitle("webview test")
	// Without a layout SetVisible panics in ContainerBase.CreateLayoutItem.
	mw.SetLayout(walk.NewVBoxLayout())
	mw.SetSizePixels(walk.Size{Width: 600, Height: 400})

	wv1, err := walk.NewWebView(mw)
	if err != nil {
		fmt.Println("FAIL NewWebView:", err)
		os.Exit(1)
	}

	finished := false
	time.AfterFunc(60*time.Second, func() {
		mw.Synchronize(func() {
			if !finished {
				check("finished", false, "timeout")
				mw.Close()
			}
		})
	})

	mw.Starting().Attach(func() {
		load("page 1", wv1, page1, "Walk WebView page 1", func() {
			load("page 2", wv1, page2, "Walk WebView page 2", func() {
				check("CanGoBack", wv1.CanGoBack(), "CanGoBack after two pages")

				wv1.Dispose()
				runtime.GC()
				runtime.GC()

				wv2, err := walk.NewWebView(mw)
				if err != nil {
					check("second NewWebView", false, "%v", err)
					mw.Close()
					return
				}
				load("second WebView", wv2, page1, "Walk WebView page 1", func() {
					wv2.Dispose()
					runtime.GC()
					finished = true
					check("finished", true, "all pages loaded")
					mw.Close()
				})
			})
		})
	})

	mw.SetVisible(true)
	mw.Run()

	if failed || !finished {
		os.Exit(1)
	}
}
