// Package manikyr provides a sensible and filesystem 
// structure agnostic image thumbnailing automation.
package manikyr

import (
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/disintegration/imaging"
	"github.com/go-fsnotify/fsnotify"
)

var (
	ErrRootNotWatched   = errors.New("root is not watched")
	ErrRootWatched      = errors.New("root is already watched")

	NearestNeighbor   = imaging.NearestNeighbor
	Box               = imaging.Box
	Linear            = imaging.Linear
	Hermite           = imaging.Hermite
	MitchellNetravali = imaging.MitchellNetravali
	CatmullRom        = imaging.CatmullRom
	BSpline           = imaging.BSpline
	Gaussian          = imaging.Gaussian
	Bartlett          = imaging.Bartlett
	Lanczos           = imaging.Lanczos
	Hann              = imaging.Hann
	Hamming           = imaging.Hamming
	Blackman          = imaging.Blackman
	Welch             = imaging.Welch
	Cosine            = imaging.Cosine
)

type EventType uint32
const (
	Error EventType = 1 << iota
	ThumbCreate
	ThumbRemove
	Watch
)
func (t EventType) String() string {
	switch t {
		case Error:
			return "Error"
		case ThumbCreate:
			return "ThumbCreate"
		case ThumbRemove:
			return "ThumbRemove"
		case Watch:
			return "Watch"
		default:
			return "Unknown"
	}
}

// Event represents a single event 
// considering watching and thumbnailing files
type Event struct {
	Root  string
	Path  string
	Type  EventType
	Error error
}

// String returns a string representation of the event
func (e Event) String() string {
	if e.Type == Error {
		return fmt.Sprintf("%s: %s @ %s \\%s", e.Type.String(), e.Error.Error(), e.Path, e.Root)
	}
	return fmt.Sprintf("%s: %s \\%s", e.Type.String(), e.Path, e.Root)
}

type rootWatcher struct {
	path    string
	watcher *fsnotify.Watcher
	events	chan Event
	done    chan struct{}
}

// Manikyr watches specified directory roots for changes.
// If a new file matching the rules the user has set is created,
// it will get watched (directory), or thumbnailed (regular file)
// to a dynamic location with the chosen dimensions and algorithm.
// Subdirectory unwatching on deletion is automatic.
type Manikyr struct {
	roots             []*rootWatcher
	thumbDirPerms     os.FileMode
	thumbWidth        int
	thumbHeight       int
	thumbAlgo         imaging.ResampleFilter
	ThumbDirGetter    func(string) string
	ThumbNameGetter   func(string) string
	ShouldCreateThumb func(string, string) bool
	ShouldWatchSubdir func(string, string) bool
}

func init() {
	// Utilize all CPU cores for performance
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func (m *Manikyr) EmitEvent(root string, t EventType, path string, err error) {
	for _, rw := range m.roots {
		if rw.path == root {
			rw.events <-Event{
				Root: rw.path,
				Type: t,
				Path: path,
				Error: err,
			}
			return
		}
	}
}

// Root returns a list of currently watched root directory paths.
func (m *Manikyr) Roots() []string {
	roots := make([]string, len(m.roots))
	for i, rw := range m.roots {
		roots[i] = rw.path
	}
	return roots
}

func (m *Manikyr) HasRoot(root string) bool {
	for _, rw := range m.roots {
		if rw.path == root {
			return true
		}
	}
	return false
}

// AddRoot adds and watches specified path as a new root, piping future errors to given channel.
// The error returned considers the watcher creation, not function.
func (m *Manikyr) AddRoot(root string, evtChan chan Event) error {
	if m.HasRoot(root) {
		return ErrRootWatched
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	doneChan := make(chan struct{})

	rw := rootWatcher{
		path: root,
		events: evtChan,
		done: doneChan,
		watcher: w,
	}

	m.roots = append(m.roots, &rw)
	go m.watch(&rw)

	return rw.watcher.Add(root)
}

// RemoveRoot removes the named root directory path and unwatches it. 
// A root should always be unwatched this way prior to actual
// path deletion in the filesystem.
// If the named path was not previously specified to be a root,
// a non-nil error is returned.
func (m *Manikyr) RemoveRoot(root string) error {
	if !m.HasRoot(root) {
		return ErrRootNotWatched
	}

	for i, rw := range m.roots {
		if rw.path == root {
			rw.watcher.Close()
			rw.done <- struct{}{}
			m.roots = append(m.roots[:i], m.roots[i+1:]...)
			break
		}
	}

	return nil
}

func (m *Manikyr) watch(rw *rootWatcher) {
	defer rw.watcher.Close()
	for {
		select {
		case evt := <-rw.watcher.Events:
			if evt.Op == fsnotify.Create {
				// If a file was created

				// Get some info about the file
				info, err := os.Stat(evt.Name)
				if os.IsNotExist(err) {
					m.EmitEvent(rw.path, Error, evt.Name, err)
					continue
				}

				switch mode := info.Mode(); {
				case mode.IsDir():
					if m.ShouldWatchSubdir(rw.path, evt.Name) {
						rw.watcher.Add(evt.Name)
					}
				case mode.IsRegular():
					if m.ShouldCreateThumb(rw.path, evt.Name) {
						go m.createThumb(rw.path, evt.Name)
					}
				}
			} else {
				// Something else happened to the file
				_, err := os.Stat(evt.Name)
				if os.IsNotExist(err) {
					// Try to delete thumb.
					m.removeThumb(rw.path, evt.Name)
					continue
				} else if err != nil {
					m.EmitEvent(rw.path, Error, evt.Name, err)
					continue
				}
			}
		case err := <-rw.watcher.Errors:
			m.EmitEvent(rw.path, Error, "", err)
		case <-rw.done:
			break
		}
	}
}

// AddSubdir adds a subdirectory to a root watcher. 
// Both paths should be absolute.
func (m *Manikyr) AddSubdir(root, subdir string) {
	for _, rw := range m.roots {
		if rw.path == root {
			err := rw.watcher.Add(subdir)
			if err != nil {
				m.EmitEvent(root, Error, subdir, err)
				return
			}
			m.EmitEvent(root, Watch, subdir, nil)
			return
		}
	}

	m.EmitEvent(root, Error, subdir, ErrRootNotWatched)
}

// RemoveSubdir removes a subdirectory from a root watcher.
// Both paths should be absolute.
func (m *Manikyr) RemoveSubdir(root, subdir string) error {
	for _, rw := range m.roots {
		if rw.path == root {
			return rw.watcher.Remove(subdir)
		}
	}

	return ErrRootNotWatched
}

func (m *Manikyr) removeThumb(root, parentFile string) {
	thumbPath := path.Join(m.ThumbDirGetter(parentFile), m.ThumbNameGetter(parentFile))
	err := os.Remove(thumbPath)

	if os.IsNotExist(err) {
		return
	} else if err != nil {
		m.EmitEvent(root, Error, thumbPath, err)
		return
	}

	m.EmitEvent(root, ThumbRemove, thumbPath, nil)
}

func (m *Manikyr) createThumb(root, parentFile string) {
	img, err := openImageWhenReady(parentFile)
	if err != nil {
		m.EmitEvent(root, Error, parentFile, err)
		return
	}

	localThumbs := m.ThumbDirGetter(parentFile)
	_, err = os.Stat(localThumbs)
	// If thumbDir does not exist...
	if os.IsNotExist(err) {
		// ..create it
		err := os.Mkdir(localThumbs, m.thumbDirPerms)
		if err != nil {
			m.EmitEvent(root, Error, localThumbs, err)
			return
		}
	} else if err != nil {
		m.EmitEvent(root, Error, localThumbs, err)
		return
	}

	// Save the thumbnail
	thumb := imaging.Thumbnail(img, m.thumbWidth, m.thumbHeight, m.thumbAlgo)
	thumbPath := path.Join(localThumbs, m.ThumbNameGetter(parentFile))
	if err = imaging.Save(thumb, thumbPath); err != nil {
		m.EmitEvent(root, Error, thumbPath, err)
		return
	}

	m.EmitEvent(root, ThumbCreate, thumbPath, nil)
}

// Get the currently set thumbnail dimensions
func (m *Manikyr) ThumbSize() (int, int) {
	return m.thumbWidth, m.thumbHeight
}

// Set thumbnail dimensions. 
// Dimensions should be positive.
func (m *Manikyr) SetThumbSize(w, h int) {
	if w < 1 {
		w = 1
	}
	m.thumbWidth = w

	if h < 1 {
		h = 1
	}
	m.thumbHeight = h
}

// ThumbDirFileMode gets the currently set filemode for thumbnail directories.
func (m *Manikyr) ThumbDirFileMode() os.FileMode {
	return m.thumbDirPerms
}

// SetThumbDirFileMode sets the filemode for thumbnail directories.
func (m *Manikyr) SetThumbDirFileMode(fm uint32) {
	m.thumbDirPerms = os.FileMode(fm)
}

// ThumbAlgorithm gets the currently used algorithm for thumbnail creation.
func (m *Manikyr) ThumbAlgorithm() imaging.ResampleFilter {
	return m.thumbAlgo
}

// SetThumbAlgorithm sets the used algorithm for thumbnail creation.
// See http://godoc.org/github.com/disintegration/imaging#ResampleFilter for more info.
func (m *Manikyr) SetThumbAlgorithm(filter imaging.ResampleFilter) {
	m.thumbAlgo = filter
}

// Init watches and thumbnail existing files as if they
// were added after the root directory got watched.
// Regular files are checked for corresponding thumbnails
// before creating a new one. 
func (m *Manikyr) Init(root string) error {
	if !m.HasRoot(root) {
		return ErrRootNotWatched
	}
	autoAdd(m, root, root)
	return nil
}

// New creates a new Manikyr instance which holds a set of
// preferences and directory roots to apply those rules to.
// Default values should be kept safe, as in not doing
// any damage to the filesystem integrity if the instance is
// initialized as is. 
func New() *Manikyr {
	return &Manikyr{
		roots:         []*rootWatcher{},
		thumbWidth:    128,
		thumbHeight:   128,
		thumbAlgo:     NearestNeighbor,
		thumbDirPerms: 0777,
		ThumbDirGetter: func(parentFile string) string {
			return os.TempDir()
		},
		ThumbNameGetter: func(parentFile string) string {
			return path.Base(parentFile)
		},
		ShouldCreateThumb: func(root, parentFile string) bool {
			return false
		},
		ShouldWatchSubdir: func(root, subdir string) bool {
			return false
		},
	}
}

