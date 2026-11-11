// Command labels checks on Windows that a Label names the field after it, so
// that screen readers read the label when the field gets the focus.
//
// The system proxies name an edit field, combo box or similar control
// without text of its own from the previous window in z-order only if that
// window has the class Static. A walk Label is a window of its own class
// with the Static inside, so without help from walk such fields have no
// name.
//
// The form has, in a two column grid as in examples/databinding, a label
// before each of these fields: LineEdit, DateEdit, ComboBox, editable
// ComboBox, NumberEdit, Slider, CheckBox without text, TextEdit, and a
// LineEdit whose name the program sets through Accessibility. Each field
// must have the label text as its UI Automation name (for the NumberEdit
// and the editable ComboBox also the edit control inside, which gets the
// focus); the field with a name of its own must keep it. Then the text of
// a label changes, and the name of its field must follow.
//
// Then the explicit API: a check box with text keeps its text, a NoPrefix
// label keeps its "&", a label with LabelFor names a widget further away (the
// button after it keeps its text), a NoLabelFor label names nothing, and
// SetLabelFor(nil) and SetLabelFor at run time move the name.
//
// LabeledBy is printed as an INFO line; Dynamic Annotation cannot set it.
// "labels manual" leaves the window open for a test with a screen reader.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/uia"
)

var (
	failed bool
	mw     *walk.MainWindow
)

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Println("PASS", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: "+format+"\n", append([]any{name}, args...)...)
}

// onUI runs f on the UI thread and waits for it.
func onUI(f func()) {
	done := make(chan struct{})
	mw.Synchronize(func() {
		defer close(done)
		f()
	})
	<-done
}

// field is a widget and the name a screen reader should read for it.
type field struct {
	desc   string
	widget walk.Widget
	want   string
	// inner is true for widgets whose focus goes to an edit control
	// inside them; that edit control is checked too.
	inner bool
}

// elementName returns the UI Automation name of e, waiting up to a second
// for it to become want.
func elementName(e *uia.Element, want string) (string, error) {
	var name string
	var err error
	for i := 0; i < 10; i++ {
		if name, err = e.Name(); err == nil && name == want {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return name, err
}

func checkField(a *uia.Automation, f field) {
	e, err := a.ElementFromHandle(uintptr(f.widget.Handle()))
	if err != nil {
		check(f.desc, false, "ElementFromHandle: %v", err)
		return
	}
	defer e.Release()

	name, err := elementName(e, f.want)
	check(f.desc+" name", err == nil && name == f.want, "name %q (error %v), want %q", name, err, f.want)

	if l, err := e.LabeledBy(); err != nil {
		fmt.Printf("INFO %s LabeledBy: %v\n", f.desc, err)
	} else if l == nil {
		fmt.Printf("INFO %s LabeledBy: none\n", f.desc)
	} else {
		n, _ := l.Name()
		fmt.Printf("INFO %s LabeledBy: %q\n", f.desc, n)
		l.Release()
	}

	if !f.inner {
		return
	}
	edit, err := e.FindFirst(uia.ScopeDescendants, uia.ByControlType(uia.ControlTypeEdit))
	if err != nil || edit == nil {
		check(f.desc+" inner edit", false, "no edit control inside (error %v)", err)
		return
	}
	defer edit.Release()
	name, err = elementName(edit, f.want)
	check(f.desc+" inner edit name", err == nil && name == f.want, "name %q (error %v), want %q", name, err, f.want)
}

func main() {
	manual := len(os.Args) > 1 && os.Args[1] == "manual"

	var (
		nameLE, customLE *walk.LineEdit
		dateDE           *walk.DateEdit
		speciesCB        *walk.ComboBox
		foodCB           *walk.ComboBox
		weightNE         *walk.NumberEdit
		speedSl          *walk.Slider
		domesticCB       *walk.CheckBox
		remarksTE        *walk.TextEdit
		nameLabel        *walk.Label
		speciesLabel     *walk.Label
		optionCB         *walk.CheckBox
		rdLE, unusedLE   *walk.LineEdit
		phoneLE          *walk.LineEdit
		dialPB           *walk.PushButton
	)

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "Labels test",
		Size:     Size{Width: 400, Height: 450},
		Layout:   Grid{Columns: 2},
		Children: []Widget{
			Label{AssignTo: &nameLabel, Text: "Name:"},
			LineEdit{AssignTo: &nameLE, Text: "Rex"},
			Label{Text: "Arrival Date:"},
			DateEdit{AssignTo: &dateDE},
			Label{AssignTo: &speciesLabel, Text: "Species:"},
			ComboBox{AssignTo: &speciesCB, Model: []string{"Dog", "Cat"}, CurrentIndex: 0},
			Label{Text: "Preferred &Food:"},
			ComboBox{AssignTo: &foodCB, Editable: true, Model: []string{"Fruit", "Meat"}},
			Label{Text: "Weight:"},
			NumberEdit{AssignTo: &weightNE, Decimals: 2, Suffix: " kg"},
			Label{Text: "Speed:"},
			Slider{AssignTo: &speedSl},
			Label{Text: "Domesticated:"},
			CheckBox{AssignTo: &domesticCB},
			Label{Text: "Remarks:"},
			TextEdit{AssignTo: &remarksTE},
			Label{Text: "Ignored label:"},
			LineEdit{AssignTo: &customLE, Accessibility: Accessibility{Name: "Custom name"}},
			Label{Text: "Option:"},
			CheckBox{AssignTo: &optionCB, Text: "Enabled"},
			Label{Text: "R&D:", NoPrefix: true},
			LineEdit{AssignTo: &rdLE},
			Label{Text: "Phone:", LabelFor: "phone"},
			PushButton{AssignTo: &dialPB, Text: "Dial"},
			Label{Text: "Unused:", NoLabelFor: true},
			LineEdit{AssignTo: &unusedLE},
			LineEdit{AssignTo: &phoneLE, Name: "phone", ColumnSpan: 2},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if manual {
			return
		}
		defer mw.Synchronize(func() { mw.Close() })

		a, err := uia.New()
		if err != nil {
			check("uia.New", false, "%v", err)
			return
		}
		defer a.Close()

		fields := []field{
			{"LineEdit", nameLE, "Name:", false},
			{"DateEdit", dateDE, "Arrival Date:", false},
			{"ComboBox", speciesCB, "Species:", false},
			{"editable ComboBox", foodCB, "Preferred Food:", true},
			{"NumberEdit", weightNE, "Weight:", true},
			{"Slider", speedSl, "Speed:", false},
			{"CheckBox without text", domesticCB, "Domesticated:", false},
			{"TextEdit", remarksTE, "Remarks:", false},
			{"LineEdit with its own name", customLE, "Custom name", false},
			{"CheckBox with text", optionCB, "Enabled", false},
			{"LineEdit after a NoPrefix label", rdLE, "R&D:", false},
			{"PushButton after a label for another widget", dialPB, "Dial", false},
			{"LineEdit after a NoLabelFor label", unusedLE, "", false},
			{"LineEdit named by LabelFor", phoneLE, "Phone:", false},
		}
		for _, f := range fields {
			checkField(a, f)
		}

		onUI(func() { nameLabel.SetText("Full name:") })
		checkField(a, field{"LineEdit after the label text changed", nameLE, "Full name:", false})

		var err2 error
		onUI(func() { err2 = speciesLabel.SetLabelFor(nil) })
		check("SetLabelFor(nil)", err2 == nil, "%v", err2)
		checkField(a, field{"ComboBox after SetLabelFor(nil)", speciesCB, "", false})

		onUI(func() { err2 = speciesLabel.SetLabelFor(unusedLE) })
		check("SetLabelFor at run time", err2 == nil, "%v", err2)
		checkField(a, field{"LineEdit after SetLabelFor at run time", unusedLE, "Species:", false})
	}()
	mw.Run()
	<-done

	if failed {
		os.Exit(1)
	}
}
