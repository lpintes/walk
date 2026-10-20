// Command webview2 checks walk.WebView2 (step 7) on Windows: the loader
// without WebView2Loader.dll, the asynchronous creation, navigation to a
// local file and to a string, the title and URL events, a web message
// round trip, ExecuteScript, the history, and Dispose followed by a new
// WebView2.
//
// It prints PASS, FAIL and SKIP lines and exits with 1 if a check failed
// or did not finish in time. Without a WebView2 runtime it prints a SKIP
// line and exits with 0. The browser data goes to a temporary folder.
//
// With the argument "manual" it skips the checks and shows a window with a
// button, a WebView2 with a small form and another button, for checking
// Tab, Shift+Tab and F6 and what a screen reader says (see task 17 of
// TESTING_ON_WINDOWS.md).
package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lpintes/walk"
	"github.com/lpintes/walk/internal/win"
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

const pageScript = `<script>
window.chrome.webview.addEventListener("message", e => {
	window.chrome.webview.postMessage("echo:" + e.data);
});
</script>`

func writePage(dir, name, title string) string {
	path := filepath.Join(dir, name)
	html := "<!DOCTYPE html><html><head><title>" + title + "</title>" + pageScript +
		"</head><body><h1>" + title + "</h1><p>A paragraph.</p></body></html>"
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		fmt.Println("FAIL WriteFile:", err)
		os.Exit(1)
	}
	return (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(path)}).String()
}

// samePage reports whether two file URLs name the same file; WebView2 may
// escape or spell them differently.
func samePage(a, b string) bool {
	pa, errA := url.Parse(a)
	pb, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(pa.Scheme, pb.Scheme) && strings.EqualFold(pa.Path, pb.Path)
}

func runtimeMissing(err error) bool {
	return errors.Is(err, walk.ErrWebView2RuntimeNotFound)
}

func skip(reason string) {
	fmt.Println("SKIP webview2:", reason)
	os.Exit(0)
}

// step runs the checks one after another; each calls next when done.
type step func(next func())

func runSteps(steps []step, done func()) {
	if len(steps) == 0 {
		done()
		return
	}
	steps[0](func() { runSteps(steps[1:], done) })
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "manual" {
		manual()
		return
	}

	dir, err := os.MkdirTemp("", "walk-webview2")
	if err != nil {
		fmt.Println("FAIL MkdirTemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	page1 := writePage(dir, "page1.html", "Walk WebView2 page 1")
	options := walk.WebView2Options{UserDataFolder: filepath.Join(dir, "data")}

	mw, err := walk.NewMainWindow()
	if err != nil {
		fmt.Println("FAIL NewMainWindow:", err)
		os.Exit(1)
	}
	mw.SetTitle("webview2 test")
	mw.SetLayout(walk.NewVBoxLayout())
	mw.SetSizePixels(walk.Size{Width: 700, Height: 500})

	wv, err := walk.NewWebView2WithOptions(mw, options)
	if runtimeMissing(err) {
		skip("no WebView2 runtime installed: " + err.Error())
	}
	if err != nil {
		fmt.Println("FAIL NewWebView2:", err)
		os.Exit(1)
	}

	// Before the browser is ready: navigations are queued, the rest
	// returns ErrWebView2NotReady.
	check("not ready", !wv.IsReady(), "IsReady %v before Ready", wv.IsReady())
	err = wv.ExecuteScript("1", nil)
	check("ExecuteScript before Ready", errors.Is(err, walk.ErrWebView2NotReady), "error %v", err)
	urlChanges := 0
	wv.URLChanged().Attach(func() { urlChanges++ })
	err = wv.SetURL(page1)
	check("SetURL before Ready", err == nil && wv.URL() == page1, "error %v, URL %q", err, wv.URL())

	finished := false
	finish := func() {
		finished = true
		mw.Close()
	}
	time.AfterFunc(90*time.Second, func() {
		mw.Synchronize(func() {
			if !finished {
				check("finished", false, "timeout")
				finish()
			}
		})
	})

	wv.CreationFailed().Attach(func(err error) {
		if runtimeMissing(err) {
			skip("no WebView2 runtime installed: " + err.Error())
		}
		check("creation", false, "CreationFailed: %v", err)
		finish()
	})

	var started []string
	wv.NavigationStarting().Attach(func(u string) { started = append(started, u) })
	titleChanges := 0
	wv.TitleChanged().Attach(func() { titleChanges++ })
	var messages []string
	wv.WebMessageReceived().Attach(func(json string) { messages = append(messages, json) })

	// waitNavigation calls next after the next NavigationCompleted and
	// a short pause, in which the title may still change.
	waitNavigation := func(name string, next func(ok bool)) {
		wv.NavigationCompleted().Once(func(succeeded bool, status walk.WebView2WebErrorStatus) {
			check(name+" NavigationCompleted", succeeded, "succeeded %v, web error status %d", succeeded, status)
			runtime.GC()
			time.AfterFunc(300*time.Millisecond, func() {
				wv.Synchronize(func() { next(succeeded) })
			})
		})
	}

	steps := []step{
		// The navigation queued before Ready.
		func(next func()) {
			waitNavigation("page 1", func(bool) {
				check("page 1 NavigationStarting", len(started) > 0 && samePage(started[0], page1), "started %q", started)
				check("page 1 title", wv.Title() == "Walk WebView2 page 1", "Title %q", wv.Title())
				check("page 1 TitleChanged", titleChanges > 0, "%d TitleChanged events", titleChanges)
				check("page 1 URL", samePage(wv.URL(), page1), "URL %q, want %q", wv.URL(), page1)
				check("page 1 URLChanged", urlChanges > 0, "%d URLChanged events", urlChanges)
				next()
			})
		},

		// A web message to the page and its echo back.
		func(next func()) {
			messages = nil
			if err := wv.PostWebMessage("ping"); err != nil {
				check("PostWebMessage", false, "%v", err)
				next()
				return
			}
			deadline := time.Now().Add(10 * time.Second)
			var poll func()
			poll = func() {
				if len(messages) == 0 && time.Now().Before(deadline) {
					time.AfterFunc(100*time.Millisecond, func() { wv.Synchronize(poll) })
					return
				}
				check("web message", len(messages) == 1 && messages[0] == `"echo:ping"`, "received %q", messages)
				next()
			}
			poll()
		},

		// ExecuteScript with a number, a string and a script error.
		func(next func()) {
			err := wv.ExecuteScript("1 + 2", func(result string, err error) {
				check("ExecuteScript number", err == nil && result == "3", "result %q, error %v", result, err)
				err2 := wv.ExecuteScript("document.title", func(result string, err error) {
					check("ExecuteScript string", err == nil && result == `"Walk WebView2 page 1"`, "result %q, error %v", result, err)
					err3 := wv.ExecuteScript("throw new Error('x')", func(result string, err error) {
						// A script exception is not an error of ExecuteScript;
						// the result is null.
						fmt.Printf("INFO ExecuteScript exception: result %q, error %v\n", result, err)
						next()
					})
					if err3 != nil {
						check("ExecuteScript exception call", false, "%v", err3)
						next()
					}
				})
				if err2 != nil {
					check("ExecuteScript string call", false, "%v", err2)
					next()
				}
			})
			if err != nil {
				check("ExecuteScript call", false, "%v", err)
				next()
			}
		},

		// NavigateToString, then back to page 1.
		func(next func()) {
			waitNavigation("string", func(bool) {
				check("string title", wv.Title() == "Walk WebView2 string", "Title %q", wv.Title())
				check("CanGoBack", wv.CanGoBack(), "CanGoBack after two pages")
				waitNavigation("GoBack", func(bool) {
					check("GoBack URL", samePage(wv.URL(), page1), "URL %q", wv.URL())
					check("CanGoForward", wv.CanGoForward(), "CanGoForward after GoBack")
					next()
				})
				if err := wv.GoBack(); err != nil {
					check("GoBack", false, "%v", err)
					next()
				}
			})
			if err := wv.NavigateToString("<!DOCTYPE html><html><head><title>Walk WebView2 string</title></head><body><p>From a string.</p></body></html>"); err != nil {
				check("NavigateToString", false, "%v", err)
				next()
			}
		},

		// Focus: SetFocus on the widget moves it into the page. Only
		// reported: without the foreground the focus may stay elsewhere.
		func(next func()) {
			wv.SetFocus()
			time.AfterFunc(500*time.Millisecond, func() {
				wv.Synchronize(func() {
					focus := win.GetFocus()
					fmt.Printf("INFO focus after SetFocus: %#x, widget %#x, inside the widget %v\n",
						focus, wv.Handle(), focus != 0 && win.IsChild(wv.Handle(), focus))
					next()
				})
			})
		},

		// Dispose and a second WebView2 with the same user data folder.
		func(next func()) {
			wv.Dispose()
			runtime.GC()
			runtime.GC()
			wv2, err := walk.NewWebView2WithOptions(mw, options)
			if err != nil {
				check("second NewWebView2", false, "%v", err)
				next()
				return
			}
			wv2.CreationFailed().Attach(func(err error) {
				check("second WebView2", false, "CreationFailed: %v", err)
				next()
			})
			wv2.NavigationCompleted().Once(func(succeeded bool, status walk.WebView2WebErrorStatus) {
				check("second WebView2 NavigationCompleted", succeeded, "succeeded %v, web error status %d", succeeded, status)
				wv2.Dispose()
				runtime.GC()
				next()
			})
			wv2.NavigateToString("<!DOCTYPE html><title>second</title><p>Second WebView2.</p>")
		},
	}

	wv.Ready().Attach(func() {
		check("Ready", wv.IsReady(), "IsReady %v", wv.IsReady())
	})
	// The first step waits for the navigation queued before Ready.
	runSteps(steps, func() {
		check("finished", true, "all steps done")
		finish()
	})

	mw.SetVisible(true)
	mw.Run()

	if failed || !finished {
		os.Exit(1)
	}
}

const manualPage = `<!DOCTYPE html>
<html lang="en">
<head><title>WebView2 manual test</title>
<script>
window.chrome.webview.addEventListener("message", e => {
	document.getElementById("status").textContent = "Message from the program: " + e.data;
});
function send() {
	window.chrome.webview.postMessage(document.getElementById("name").value);
}
</script>
</head>
<body>
<h1>WebView2 manual test</h1>
<p>This is the first paragraph of the page.</p>
<p><a href="https://example.com/">A link to example.com</a></p>
<p><label for="name">Your name</label> <input id="name" type="text"></p>
<p><button onclick="send()">Send to the program</button></p>
<p id="status" role="status"></p>
</body>
</html>`

func manual() {
	mw, err := walk.NewMainWindow()
	if err != nil {
		fmt.Println("FAIL NewMainWindow:", err)
		os.Exit(1)
	}
	mw.SetTitle("walk webview2 manual test")
	mw.SetLayout(walk.NewVBoxLayout())
	mw.SetSizePixels(walk.Size{Width: 800, Height: 600})

	before, _ := walk.NewPushButton(mw)
	before.SetText("Button before the browser")

	wv, err := walk.NewWebView2(mw)
	if err != nil {
		fmt.Println("FAIL NewWebView2:", err)
		os.Exit(1)
	}

	after, _ := walk.NewPushButton(mw)
	after.SetText("Button after the browser")
	after.Clicked().Attach(func() {
		if err := wv.PostWebMessage("Hello from the button after the browser"); err != nil {
			fmt.Println("PostWebMessage:", err)
		}
	})

	wv.CreationFailed().Attach(func(err error) {
		fmt.Println("CreationFailed:", err)
		walk.MsgBox(mw, "webview2", "The browser could not be created: "+err.Error(), walk.MsgBoxIconError)
	})
	wv.Ready().Attach(func() { fmt.Println("Ready") })
	wv.TitleChanged().Attach(func() {
		fmt.Println("TitleChanged:", wv.Title())
		mw.SetTitle("walk webview2 manual test - " + wv.Title())
	})
	wv.URLChanged().Attach(func() { fmt.Println("URLChanged:", wv.URL()) })
	wv.NavigationCompleted().Attach(func(succeeded bool, status walk.WebView2WebErrorStatus) {
		fmt.Println("NavigationCompleted:", succeeded, status)
	})
	wv.WebMessageReceived().Attach(func(json string) {
		fmt.Println("WebMessageReceived:", json)
		walk.Announce(mw, "The page sent "+json, walk.AnnounceAll)
	})
	wv.NavigateToString(manualPage)

	fmt.Println("Tab from the first button into the page, through the page and on to the second button;")
	fmt.Println("Shift+Tab back. Activate the second button to post a message to the page.")
	mw.SetVisible(true)
	mw.Run()
}
