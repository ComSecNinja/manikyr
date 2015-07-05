// Package manikyr provides a sensible and filesystem 
// structure agnostic image thumbnailing automation.
package manikyr

import (
	"errors"
	"os"
	"path"
	"runtime"

	"github.com/disintegration/imaging"
	"github.com/go-fsnotify/fsnotify"
)

var (
	ErrRootNotWatched   = errors.New("root is not watched")
	ErrRootWatched      = errors.New("root is already watched")
	ErrSubdirNotWatched = errors.New("subdir is not watched")
	ErrSubdirWatched    = errors.New("subdir is already watched")

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

// Manikyr watches specified directory roots for changes.
// If a new file matching the rules the user has set is created,
// it will get watched (directory), or thumbnailed (regular file)
// to a dynamic location with the chosen dimensions and algorithm.
// Subdirectory unwatching on deletion is automatic.
type Manikyr struct {
	roots             map[string]*fsnotify.Watcher
	subdirs           map[string][]string
	errChans          map[string]chan error
	doneChans         map[string]chan bool
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

// Root returns a list of currently watched root directory paths.
func (m *Manikyr) Roots() []string {
	keys := make([]string, len(m.roots))
	i := 0
	for k := range m.roots {
		keys[i] = k
		i = i + 1
	}
	return keys
}

// AddRoot adds and watches specified path as a new root, piping future errors to given channel.
// The error returned considers the watcher creation, not function.
func (m *Manikyr) AddRoot(root string, errChan chan error) error {
	if _, ok := m.roots[root]; ok {
		return ErrRootWatched
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	doneChan := make(chan bool)

	m.roots[root] = w
	m.errChans[root] = errChan
	m.doneChans[root] = doneChan
	go m.watch(root, errChan, doneChan)

	return m.roots[root].Add(root)
}

// RemoveRoot removes the named root directory path and unwatches it. 
// A root should always be unwatched this way prior to actual
// path deletion in the filesystem.
// If the named path was not previously specified to be a root,
// a non-nil error is returned.
func (m *Manikyr) RemoveRoot(root string) error {
	if _, ok := m.roots[root]; !ok {
		return ErrRootNotWatched
	}
	m.roots[root].Close()
	m.doneChans[root] <- true

	delete(m.roots, root)
	delete(m.errChans, root)
	delete(m.doneChans, root)
	delete(m.subdirs, root)
	return nil
}

func (m *Manikyr) watch(root string, errChan chan error, doneChan chan bool) {
	w, ok := m.roots[root]
	if !ok {
		errChan <- ErrRootNotWatched
		return
	}

	defer w.Close()
	for {
		select {
		case evt := <-w.Events:
			if evt.Op == fsnotify.Create {
				// If a file was created

				// Get some info about the file
				info, err := os.Stat(evt.Name)
				if os.IsNotExist(err) {
					errChan <- err
					continue
				}

				switch mode := info.Mode(); {
				case mode.IsDir():
					if m.ShouldWatchSubdir(root, evt.Name) {
						w.Add(evt.Name)
					}
				case mode.IsRegular():
					if m.ShouldCreateThumb(root, evt.Name) {
						go m.createThumb(evt.Name, errChan)
					}
				}
			} else {
				// Something else happened to the file
				_, err := os.Stat(evt.Name)
				if os.IsNotExist(err) {
					// Try to delete thumb.
					// Error is useless here because the file could
					// have been a directory or a non-image file.
					m.removeThumb(evt.Name)
				} else if err != nil {
					errChan <- err
					continue
				}
			}
		case err := <-w.Errors:
			errChan <- err
		case <-doneChan:
			break
		}
	}
}

// AddSubdir adds a subdirectory to a root watcher. 
// Both paths should be absolute.
func (m *Manikyr) AddSubdir(root, subdir string) error {
	if _, ok := m.roots[root]; !ok {
		return ErrRootNotWatched
	}
	for i := range m.subdirs[root] {
		if m.subdirs[root][i] == subdir {
			return ErrSubdirWatched
		}
	}

	err := m.roots[root].Add(subdir)
	if err != nil {
		return err
	}
	m.subdirs[root] = append(m.subdirs[root], subdir)
	return nil
}

// RemoveSubdir removes a subdirectory from a root watcher.
// Both paths should be absolute.
func (m *Manikyr) RemoveSubdir(root, subdir string) error {
	if _, ok := m.roots[root]; !ok {
		return ErrRootNotWatched
	}

	for i := range m.subdirs[root] {
		if m.subdirs[root][i] == subdir {
			m.subdirs[root] = append(m.subdirs[root][:i], m.subdirs[root][i+1:]...) // Keep indexes <i || >i
			return m.roots[root].Remove(subdir)
		}
	}
	return ErrSubdirNotWatched
}

func (m *Manikyr) removeThumb(parentFile string) error {
	thumbPath := path.Join(m.ThumbDirGetter(parentFile), m.ThumbNameGetter(parentFile))
	return os.Remove(thumbPath)
}

func (m *Manikyr) createThumb(parentFile string, errChan chan error) {
	img, err := openImageWhenReady(parentFile)
	if err != nil {
		errChan <- err
		return
	}

	localThumbs := m.ThumbDirGetter(parentFile)
	_, err = os.Stat(localThumbs)
	// If thumbDir does not exist...
	if os.IsNotExist(err) {
		// ..create it
		err := os.Mkdir(localThumbs, m.thumbDirPerms)
		if err != nil {
			errChan <- err
			return
		}
	} else if err != nil {
		errChan <- err
		return
	}

	// Save the thumbnail
	thumb := imaging.Thumbnail(img, m.thumbWidth, m.thumbHeight, m.thumbAlgo)
	thumbPath := path.Join(localThumbs, m.ThumbNameGetter(parentFile))
	if err = imaging.Save(thumb, thumbPath); err != nil {
		errChan <- err
	}
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
	if _, ok := m.roots[root]; !ok {
		return ErrRootNotWatched
	}
	return autoAdd(m, root, root)
}

// New creates a new Manikyr instance which holds a set of
// preferences and directory roots to apply those rules to.
// Default values should be kept safe, as in not doing
// any damage to the filesystem integrity if the instance is
// initialized as is. 
func New() *Manikyr {
	return &Manikyr{
		roots:         make(map[string]*fsnotify.Watcher),
		subdirs:       make(map[string][]string),
		errChans:      make(map[string]chan error),
		doneChans:     make(map[string]chan bool),
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
