// Command shell checks on Windows the shell functions of step 12 part 2:
//
//   - KnownFolderPath: every known folder has a path, and the folders with
//     an environment variable match it.
//   - MoveToRecycleBin: a file and a folder in the temporary folder are
//     moved to the Recycle Bin (their names start with walk-shell-test, so
//     they can be found and deleted there), and a missing file is an error.
//   - ShellLink: a shortcut with every field set is saved, loaded back and
//     compared, and loading a missing file is an error.
//
// It needs no window. It prints PASS and FAIL lines and exits with code 1
// if a check failed.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lpintes/walk"
)

var failed bool

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Println("PASS", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
}

// expand replaces %NAME% with the value of the environment variable NAME,
// as the shell may store paths that way.
func expand(s string) string {
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			return s
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			return s
		}
		s = s[:i] + os.Getenv(s[i+1:i+1+j]) + s[i+2+j:]
	}
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(expand(a)), filepath.Clean(expand(b)))
}

func testKnownFolders() {
	folders := []struct {
		name   string
		folder walk.KnownFolder
		env    string // environment variable with the same path, if any
	}{
		{"Desktop", walk.KnownFolderDesktop, ""},
		{"Documents", walk.KnownFolderDocuments, ""},
		{"Downloads", walk.KnownFolderDownloads, ""},
		{"Music", walk.KnownFolderMusic, ""},
		{"Pictures", walk.KnownFolderPictures, ""},
		{"Videos", walk.KnownFolderVideos, ""},
		{"Profile", walk.KnownFolderProfile, "USERPROFILE"},
		{"RoamingAppData", walk.KnownFolderRoamingAppData, "APPDATA"},
		{"LocalAppData", walk.KnownFolderLocalAppData, "LOCALAPPDATA"},
		{"ProgramData", walk.KnownFolderProgramData, "ProgramData"},
		{"ProgramFiles", walk.KnownFolderProgramFiles, "ProgramFiles"},
		{"Programs", walk.KnownFolderPrograms, ""},
		{"Startup", walk.KnownFolderStartup, ""},
		{"Windows", walk.KnownFolderWindows, "SystemRoot"},
		{"System", walk.KnownFolderSystem, ""},
	}
	for _, f := range folders {
		path, err := walk.KnownFolderPath(f.folder)
		fmt.Printf("INFO KnownFolderPath(%s) = %q\n", f.name, path)
		ok := err == nil && path != ""
		if ok && f.env != "" {
			ok = samePath(path, os.Getenv(f.env))
		}
		check("KnownFolderPath "+f.name, ok, "path %q, err %v, %%%s%% %q", path, err, f.env, os.Getenv(f.env))
	}

	personal, _ := walk.PersonalPath()
	docs, _ := walk.KnownFolderPath(walk.KnownFolderDocuments)
	check("KnownFolderPath Documents equals PersonalPath", samePath(docs, personal), "%q and %q", docs, personal)

	_, err := walk.KnownFolderPath(walk.KnownFolder(1000))
	check("KnownFolderPath invalid folder", err != nil, "no error")
}

func testRecycleBin() {
	dir, err := os.MkdirTemp("", "walk-shell-test-")
	if err != nil {
		check("MoveToRecycleBin setup", false, "%v", err)
		return
	}
	file := filepath.Join(os.TempDir(), "walk-shell-test-file.txt")
	if err := os.WriteFile(file, []byte("walk shell test"), 0o644); err != nil {
		check("MoveToRecycleBin setup", false, "%v", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "inner.txt"), []byte("walk shell test"), 0o644); err != nil {
		check("MoveToRecycleBin setup", false, "%v", err)
		return
	}

	err = walk.MoveToRecycleBin(nil, file, dir)
	_, errFile := os.Stat(file)
	_, errDir := os.Stat(dir)
	check("MoveToRecycleBin file and folder", err == nil && os.IsNotExist(errFile) && os.IsNotExist(errDir),
		"err %v, file %v, folder %v", err, errFile, errDir)

	err = walk.MoveToRecycleBin(nil, filepath.Join(os.TempDir(), "walk-shell-test-missing.txt"))
	check("MoveToRecycleBin missing file", err != nil, "no error")
}

func testShellLink() {
	dir, err := os.MkdirTemp("", "walk-shell-test-")
	if err != nil {
		check("ShellLink setup", false, "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	exe, _ := os.Executable()

	want := walk.ShellLink{
		TargetPath:       exe,
		Arguments:        `one "two three"`,
		WorkingDirectory: dir,
		Description:      "walk shell test, ľščťžýáíé",
		IconPath:         filepath.Join(os.Getenv("SystemRoot"), "System32", "shell32.dll"),
		IconIndex:        3,
		Hotkey:           walk.Shortcut{Modifiers: walk.ModControl | walk.ModAlt, Key: walk.KeyF12},
	}
	path := filepath.Join(dir, "test.lnk")
	err = want.Save(path)
	check("ShellLink.Save", err == nil, "%v", err)

	got, err := walk.LoadShellLink(path)
	if err != nil {
		check("LoadShellLink", false, "%v", err)
	} else {
		fmt.Printf("INFO loaded %+v\n", *got)
		check("LoadShellLink TargetPath", samePath(got.TargetPath, want.TargetPath), "%q", got.TargetPath)
		check("LoadShellLink Arguments", got.Arguments == want.Arguments, "%q", got.Arguments)
		check("LoadShellLink WorkingDirectory", samePath(got.WorkingDirectory, want.WorkingDirectory), "%q", got.WorkingDirectory)
		check("LoadShellLink Description", got.Description == want.Description, "%q", got.Description)
		check("LoadShellLink icon", samePath(got.IconPath, want.IconPath) && got.IconIndex == want.IconIndex,
			"%q %d", got.IconPath, got.IconIndex)
		check("LoadShellLink Hotkey", got.Hotkey == want.Hotkey, "%+v", got.Hotkey)
	}

	_, err = walk.LoadShellLink(filepath.Join(dir, "missing.lnk"))
	check("LoadShellLink missing file", err != nil, "no error")
}

func main() {
	testKnownFolders()
	testRecycleBin()
	testShellLink()
	if failed {
		os.Exit(1)
	}
}
