package main

import (
	"errors"
	"runtime"
	"strings"
	"time"

	"github.com/lpintes/walk/uia"
)

// cachedName returns the cached name of e, or an error text; e may be nil.
func cachedName(e *uia.Element) string {
	if e == nil {
		return "<nil>"
	}
	name, err := e.CachedName()
	if err != nil {
		return err.Error()
	}
	return name
}

// nameOf returns the name of e, or an error text; e may be nil.
func nameOf(e *uia.Element) string {
	if e == nil {
		return "<nil>"
	}
	name, err := e.Name()
	if err != nil {
		return err.Error()
	}
	return name
}

// cachedNames returns the cached names of elements.
func cachedNames(elements []*uia.Element) []string {
	var s []string
	for _, e := range elements {
		s = append(s, cachedName(e))
	}
	return s
}

// checkCache checks the cache requests (step 13 part 4).
func checkCache(a *uia.Automation, window, press, disabled *uia.Element) {
	byButton := uia.And(uia.ByControlType(uia.ControlTypeButton), uia.ByClassName("Button"))
	cache := uia.CacheRequest{Properties: []uia.PropertyID{
		uia.PropertyName, uia.PropertyControlType, uia.PropertyAutomationID, uia.PropertyClassName, uia.PropertyIsEnabled,
	}}

	buttons, err := window.FindAllBuildCache(uia.ScopeDescendants, byButton, cache)
	got := strings.Join(cachedNames(buttons), ",")
	if check2("FindAllBuildCache CachedName", err == nil && got == "Press me,Disabled", "%q, %v", got, err) {
		ct, err := buttons[0].CachedControlType()
		check("CachedControlType", err == nil && ct == uia.ControlTypeButton, "%v, %v", ct, err)
		class, err := buttons[0].CachedClassName()
		check("CachedClassName", err == nil && class == "Button", "%q, %v", class, err)
		id, err := buttons[0].CachedAutomationID()
		want, _ := press.AutomationID()
		check("CachedAutomationID", err == nil && id == want, "%q, %v, want %q", id, err, want)
		enabled, err := buttons[1].CachedIsEnabled()
		check("CachedIsEnabled", err == nil && !enabled, "%v, %v", enabled, err)
		_, err = buttons[0].CachedBoundingRectangle()
		check("Cached property not in the request", err != nil, "no error")
	}

	found, err := window.FindFirstBuildCache(uia.ScopeDescendants, uia.ByName("Press me"), cache)
	name := cachedName(found)
	check("FindFirstBuildCache", err == nil && same(found, press) && name == "Press me", "%v, %q", err, name)

	found, err = window.FindFirstBuildCache(uia.ScopeDescendants, uia.ByName("no such name"), cache)
	check("FindFirstBuildCache not found", err == nil && found == nil, "%v, found %v", err, found != nil)

	// PropertyValue and CachedPropertyValue.
	v, err := press.PropertyValue(uia.PropertyName)
	check("PropertyValue string", err == nil && v == "Press me", "%v (%T), %v", v, v, err)
	v, err = press.PropertyValue(uia.PropertyIsEnabled)
	check("PropertyValue bool", err == nil && v == true, "%v (%T), %v", v, v, err)
	v, err = press.PropertyValue(uia.PropertyControlType)
	check("PropertyValue ControlType", err == nil && v == uia.ControlTypeButton, "%v (%T), %v", v, v, err)
	pid, _ := press.ProcessID()
	v, err = press.PropertyValue(uia.PropertyProcessID)
	check("PropertyValue int", err == nil && v == pid, "%v (%T), %v, want %d", v, v, err, pid)
	v, err = press.PropertyValue(uia.PropertyLiveSetting)
	check("PropertyValue LiveSetting", err == nil && v == 0, "%v (%T), %v", v, v, err)

	// A cached value stays as it was until BuildUpdatedCache.
	cachedDisabled, err := disabled.BuildUpdatedCache(cache)
	if checkErr("BuildUpdatedCache", err) && check2("BuildUpdatedCache result", cachedDisabled != nil, "nil element") {
		onUI(func() { disabledButton.SetText("Disabled 2") })
		time.Sleep(200 * time.Millisecond)
		old, err1 := cachedDisabled.CachedName()
		current, err2 := cachedDisabled.Name()
		check("cached value after a change", err1 == nil && err2 == nil && old == "Disabled" && current == "Disabled 2", "cached %q, %v, current %q, %v", old, err1, current, err2)
		v, err = cachedDisabled.CachedPropertyValue(uia.PropertyName)
		check("CachedPropertyValue", err == nil && v == "Disabled", "%v, %v", v, err)
		updated, err := cachedDisabled.BuildUpdatedCache(cache)
		name := cachedName(updated)
		check("BuildUpdatedCache after a change", err == nil && name == "Disabled 2" && same(updated, disabled), "%v, %q", err, name)
		onUI(func() { disabledButton.SetText("Disabled") })
	}

	// The cached tree below the parent of the button.
	parent, err := a.ControlView().Parent(press)
	if !checkErr("Parent for the cached tree", err) || parent == nil {
		return
	}
	tree, err := parent.BuildUpdatedCache(uia.CacheRequest{
		Properties: []uia.PropertyID{uia.PropertyName},
		Scope:      uia.ScopeSubtree,
	})
	if checkErr("BuildUpdatedCache ScopeSubtree", err) && check2("BuildUpdatedCache ScopeSubtree result", tree != nil, "nil element") {
		children, err := tree.CachedChildren()
		var cachedPress *uia.Element
		for _, c := range children {
			if same(c, press) {
				cachedPress = c
			}
		}
		name := cachedName(cachedPress)
		check("CachedChildren", err == nil && name == "Press me", "%v, %q, %q", err, name, cachedNames(children))
		current, _ := a.ControlView().Children(parent)
		check("CachedChildren like the control view", len(children) == len(current), "%d cached, %d current", len(children), len(current))
		if cachedPress != nil {
			p, err := cachedPress.CachedParent()
			check("CachedParent", err == nil && same(p, parent), "%v, same %v", err, same(p, parent))
		}
		p, err := tree.CachedParent()
		check("CachedParent of the cache root", p == nil || err != nil, "%v, %v", p != nil, err)
	}

	// ScopeChildren without ScopeElement: the parent does not match the
	// filter, and with ScopeElement UI Automation would return no element.
	filtered, err := parent.BuildUpdatedCache(uia.CacheRequest{
		Properties: []uia.PropertyID{uia.PropertyName},
		Scope:      uia.ScopeChildren,
		Filter:     byButton,
	})
	if checkErr("BuildUpdatedCache with a filter", err) && check2("BuildUpdatedCache with a filter result", filtered != nil, "nil element") {
		children, err := filtered.CachedChildren()
		got := strings.Join(cachedNames(children), ",")
		check("CachedChildren with a filter", err == nil && got == "Press me,Disabled", "%q, %v", got, err)
	}

	children, err := press.CachedChildren()
	check("CachedChildren without a cache", err != nil || children == nil, "%d children", len(children))

	_, err = window.FindAllBuildCache(uia.ScopeDescendants, uia.Property(uia.PropertyName, 1.5), cache)
	check("FindAllBuildCache unsupported condition", err != nil, "no error")
}

// gridNames returns the names of elements, for the grid checks.
func gridNames(elements []*uia.Element, err error) string {
	if err != nil {
		return err.Error()
	}
	return strings.Join(names(elements), ",")
}

// checkGrid checks the Grid, GridItem, Table, TableItem and Scroll
// patterns on the TableView (step 13 part 4). Its normal list view has
// the two columns; the frozen one has none.
func checkGrid(window, press *uia.Element) {
	lists, err := window.FindAll(uia.ScopeDescendants, uia.ByClassName("SysListView32"))
	var grid *uia.Element
	var rows, columns int
	for _, l := range lists {
		if r, c, err := l.GridSize(); err == nil && c == 2 {
			grid, rows, columns = l, r, c
		}
	}
	if !check2("TableView list view with the Grid pattern", err == nil && grid != nil, "%v, %d list views", err, len(lists)) {
		return
	}
	check("GridSize", rows == tableRows && columns == 2, "%d rows, %d columns", rows, columns)

	first, err := grid.GridItem(1, 0)
	name := nameOf(first)
	check("GridItem column 0", err == nil && strings.Contains(name, "Item 02"), "%q, %v", name, err)
	cell, err := grid.GridItem(1, 1)
	name = nameOf(cell)
	if check2("GridItem column 1", err == nil && name == "Value 02", "%q, %v", name, err) {
		pos, err := cell.GridPosition()
		check("GridPosition", err == nil && pos == uia.GridPosition{Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1}, "%+v, %v", pos, err)
		g, err := cell.ContainingGrid()
		check("ContainingGrid", err == nil && same(g, grid), "%v", err)
		got := gridNames(cell.ColumnHeaderItems())
		check("ColumnHeaderItems", got == "Value", "%q", got)
		_, err = cell.RowHeaderItems()
		check("RowHeaderItems", err == nil || errors.Is(err, uia.ErrPatternNotSupported), "%v", err)
	}
	_, err = grid.GridItem(tableRows, 0)
	check("GridItem out of range", err != nil, "no error")

	got := gridNames(grid.ColumnHeaders())
	check("ColumnHeaders", got == "Name,Value", "%q", got)
	_, err = grid.RowHeaders()
	check("RowHeaders", err == nil || errors.Is(err, uia.ErrPatternNotSupported), "%v", err)
	order, err := grid.TableOrder()
	check("TableOrder", err == nil && order >= uia.RowMajor && order <= uia.TableOrderIndeterminate, "%v, %v", order, err)

	// Scroll the 30 rows, of which only a few are visible.
	si, err := grid.ScrollInfo()
	check("ScrollInfo", err == nil && si.VerticallyScrollable && si.VerticalPercent == 0 && si.VerticalViewSize > 0 && si.VerticalViewSize < 100,
		"%+v, %v", si, err)
	err = grid.Scroll(uia.ScrollNone, uia.ScrollLargeIncrement)
	si, err2 := grid.ScrollInfo()
	check("Scroll", err == nil && err2 == nil && si.VerticalPercent > 0, "%v, %+v, %v", err, si, err2)
	err = grid.SetScrollPercent(uia.NoScroll, 100)
	si, err2 = grid.ScrollInfo()
	if runtime.GOARCH == "arm64" {
		check("SetScrollPercent on arm64", err != nil, "no error")
	} else {
		check("SetScrollPercent 100", err == nil && err2 == nil && si.VerticalPercent > 99, "%v, %+v, %v", err, si, err2)
		err = grid.SetScrollPercent(uia.NoScroll, 0)
		si, err2 = grid.ScrollInfo()
		check("SetScrollPercent 0", err == nil && err2 == nil && si.VerticalPercent == 0, "%v, %+v, %v", err, si, err2)
	}

	_, _, err = press.GridSize()
	check("GridSize of a button", errors.Is(err, uia.ErrPatternNotSupported), "%v, want ErrPatternNotSupported", err)
	_, err = press.ScrollInfo()
	check("ScrollInfo of a button", errors.Is(err, uia.ErrPatternNotSupported), "%v, want ErrPatternNotSupported", err)
	check("TableOrder String", uia.RowMajor.String() == "row major" && uia.TableOrder(9).String() == "TableOrder(9)",
		"%q %q", uia.RowMajor.String(), uia.TableOrder(9).String())
}
