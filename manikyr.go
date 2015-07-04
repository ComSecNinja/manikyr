package manikyr

import (
	"errors"
	"image"
	"os"
	"path"

	"github.com/go-fsnotify/fsnotify"
	"github.com/disintegration/imaging"
)

var (
	ErrRootNotWatched 	= errors.New("root is not watched")
	ErrRootWatched 		= errors.New("root is already watched")
	ErrSubdirNotWatched	= errors.New("subdir is not watched")
	ErrSubdirWatched	= errors.New("subdir is already watched")

	NearestNeighbor		= imaging.NearestNeighbor
	Box			= imaging.Box
	Linear			= imaging.Linear
	Hermite			= imaging.Hermite
	MitchellNetravali	= imaging.MitchellNetravali
	CatmullRom		= imaging.CatmullRom
	BSpline			= imaging.BSpline
	Gaussian		= imaging.Gaussian
	Bartlett		= imaging.Bartlett
	Lanczos			= imaging.Lanczos
	Hann			= imaging.Hann
	Hamming			= imaging.Hamming
	Blackman		= imaging.Blackman
	Welch			= imaging.Welch
	Cosine			= imaging.Cosine
)

type Manikyr struct {
	roots			map[string]*fsnotify.Watcher
	subdirs			map[string][]string
	errChans		map[string]chan error
	thumbDirPerms		os.FileMode
	thumbWidth		int
	thumbHeight		int
	thumbAlgo		imaging.ResampleFilter
	ThumbDirGetter		func(string) string
	ThumbNameGetter		func(string) string
	ShouldCreateThumb	func(string, string) bool
	ShouldWatchSubdir	func(string, string) bool
}
func (m *Manikyr) Roots() []string {
	keys := make([]string, len(m.roots))
	i := 0
	for k := range m.roots {
		keys[i] = k
		i = i + 1
	}
	return keys
}
func (m *Manikyr) AddRoot(root string, errChan chan error) error {
	if _, ok := m.roots[root]; ok {
		return ErrRootWatched
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	m.roots[root] = w
	m.errChans[root] = errChan
	go m.watch(root, errChan)

	err = m.roots[root].Add(root)
	if err != nil {
		return err
	}
	return nil
}
func (m *Manikyr) RemoveRoot(root string) error {
	if _, ok := m.roots[root]; !ok {
		return ErrRootNotWatched
	}
	m.roots[root].Close()
	delete(m.roots, root)
	delete(m.errChans, root)
	delete(m.subdirs, root)
	return nil
}
func (m *Manikyr) watch(root string, errChan chan error) {
	w, ok := m.roots[root]
	if !ok {
		errChan <-ErrRootNotWatched
	}

	defer w.Close()
	for {
		select {
		case evt := <- w.Events:
			if evt.Op == fsnotify.Create {
				// If a file was created

				// Get some info about the file
				info, err := os.Stat(evt.Name)
				if os.IsNotExist(err) {
					errChan <-err
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
					errChan <-err
					continue
				}
			}
		case err := <- w.Errors:
			errChan <-err
		}
	}
}
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
func (m *Manikyr) RemoveSubdir(root, subdir string) error {
	if _, ok := m.roots[root]; !ok {
		return ErrRootNotWatched
	}

	for i := range m.subdirs[root] {
		if m.subdirs[root][i] == subdir {
			m.subdirs[root] = append(m.subdirs[root][:i], m.subdirs[root][i+1:]...) // Keep indexes <i || >i
			err := m.roots[root].Remove(subdir)
			if err != nil {
				return err
			}
			return nil
		}
	}
	return ErrSubdirNotWatched
}
func (m *Manikyr) Subdirs(root string) ([]string, error) {
	subdirs := make([]string, len(m.subdirs[root]))
	
	if _, ok := m.roots[root]; !ok {
		return subdirs, ErrRootNotWatched
	}
	
	for i := range m.subdirs[root] {
		subdirs[i] = m.subdirs[root][i]
	}
	return subdirs, nil
}
func (m *Manikyr) removeThumb(parentFile string) error {
	thumbPath := path.Join(m.ThumbDirGetter(parentFile), m.ThumbNameGetter(parentFile))
	return os.Remove(thumbPath)
}
func (m *Manikyr) createThumb(parentFile string, errChan chan error) {
	img, err := imaging.Open(parentFile)
	if err == image.ErrFormat {
		// There is a chance that the file is not yet completely created.
		// We need some sort of retry/wait functionality in here for production use.
		errChan <-err
		return
	} else if err != nil {
		errChan <-err
		return
	}

	localThumbs := m.ThumbDirGetter(parentFile)
	_, err = os.Stat(localThumbs)
	// If thumbDir does not exist...
	if os.IsNotExist(err) {
		// ..create it
		err := os.Mkdir(localThumbs, m.thumbDirPerms)
		if err != nil {
			errChan <-err
			return
		}
	} else if err != nil {
		errChan <-err
		return
	}

	// Save the thumbnail
	thumb := imaging.Thumbnail(img, m.thumbWidth, m.thumbHeight, m.thumbAlgo)
	thumbPath := path.Join(localThumbs, m.ThumbNameGetter(parentFile))
	if err = imaging.Save(thumb, thumbPath); err != nil {
		errChan <-err
		return
	}
}
func (m *Manikyr) ThumbSize() (int, int) {
	return m.thumbWidth, m.thumbHeight
}
func (m *Manikyr) SetThumbSize(w, h int) {
	// Dimensions must be positive

	if w < 1 {
		w = 1
	}
	m.thumbWidth = w

	if h < 1 {
		h = 1
	}
	m.thumbHeight = h
}
func (m *Manikyr) ThumbDirFileMode() uint32 {
	return uint32(m.thumbDirPerms)
}
func (m *Manikyr) SetThumbDirFileMode(fm uint32) {
	m.thumbDirPerms = os.FileMode(fm)
}
func (m *Manikyr) ThumbAlgorithm() imaging.ResampleFilter {
	return m.thumbAlgo
}
func (m *Manikyr) SetThumbAlgorithm(filter imaging.ResampleFilter) {
	m.thumbAlgo = filter
}

func New() *Manikyr {
	// Sensible defaults
	return &Manikyr{
		roots:			make(map[string]*fsnotify.Watcher),
		subdirs:		make(map[string][]string),
		errChans:		make(map[string]chan error),
		thumbWidth: 		100,
		thumbHeight: 		100,
		thumbAlgo:		NearestNeighbor,
		thumbDirPerms:		0777,
		ThumbDirGetter: func(parentFile string) string {
			return path.Join(path.Dir(parentFile), ".thumbs")
		},
		ThumbNameGetter: func(parentFile string) string {
			return path.Base(parentFile)
		},
		ShouldCreateThumb: func(root, parentFile string) bool {
			ok, _ := NthSubdir(root, parentFile, 1)
			if ok {
				return true
			}
			return false
		},
		ShouldWatchSubdir: func(root, parentFile string) bool {
			ok, _ := NthSubdir(root, parentFile, 0)
			if ok && parentFile[0] != '.' {
				return true
			}
			return false
		},
	}
}
