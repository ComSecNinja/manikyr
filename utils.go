package manikyr

import (
	"image"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"
	
	"github.com/disintegration/imaging"
)

func openImageWhenReady(file string) (image.Image, error) {
	// Retry opening the image until err != image.ErrFormat
	// or the next retry would take over a minute.
	// FIXME
	
	var img image.Image
	var err error
	var retry int
	var t time.Duration

	for {
		t = time.Duration(1000 * (retry * 2))
		time.Sleep(time.Millisecond * t)

		img, err = imaging.Open(file)
		if err == image.ErrFormat {
			retry = retry + 1
			if retry*2 > 60 {
				break
			}
			continue
		}
		break
	}
	return img, err
}

// Subdirectories returns a list of absolute paths of subdirectories in a given directory.
// Returned error is non-nil if the directory provided could not be read.
func Subdirectories(root string) ([]string, error) {
	var dirs []string

	files, err := ioutil.ReadDir(root)
	if err != nil {
		return dirs, err
	}
	for _, file := range files {
		if file.IsDir() {
			dirs = append(dirs, path.Join(root, file.Name()))
		}
	}

	return dirs, nil
}

// NthSubdir returns true if dir is the nth-level subdirectory of root, else false.
// This function should never return a non-nil error, preserved for testing for now.
func NthSubdir(root, dir string, n int) (bool, error) {
	return path.Match(path.Join(root, "*" + strings.Repeat("/*", n)), dir)
}

func autoAdd(m *Manikyr, root, currentDir string) error {
	files, err := ioutil.ReadDir(currentDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		filePath := path.Join(root, file.Name())
		if file.IsDir() && m.ShouldWatchSubdir(currentDir, filePath) {
			err = m.AddSubdir(root, filePath)
			if err != nil {
				return err
			}
			err = autoAdd(m, root, filePath)
			if err != nil {
				return err
			}
		} else if !file.IsDir() && m.ShouldCreateThumb(root, filePath) {
			thumbLocation := path.Join(m.ThumbDirGetter(filePath), m.ThumbNameGetter(filePath))
			if _, err := os.Stat(thumbLocation); os.IsNotExist(err) {
				go m.createThumb(filePath, m.errChans[root])
			} else if err != nil {
				return err	
			}
		}
	}
	return nil
}
