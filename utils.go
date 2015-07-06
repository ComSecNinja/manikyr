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

func NthSubdir(root, dir string, n int) (bool, error) {
	return path.Match(path.Join(root, "*" + strings.Repeat("/*", n)), dir)
}

func autoAdd(m *Manikyr, root, currentDir string) {
	files, err := ioutil.ReadDir(currentDir)
	if err != nil {
		m.EmitEvent(root, Error, currentDir, err)
		return
	}

	for _, file := range files {
		filePath := path.Join(root, file.Name())
		if file.IsDir() && m.ShouldWatchSubdir(currentDir, filePath) {
			m.AddSubdir(root, filePath)
			autoAdd(m, root, filePath)
		} else if !file.IsDir() && m.ShouldCreateThumb(root, filePath) {
			println(2)
			thumbLocation := path.Join(m.ThumbDirGetter(filePath), m.ThumbNameGetter(filePath))
			if _, err := os.Stat(thumbLocation); os.IsNotExist(err) {
				go m.createThumb(root, filePath)
			} else if err != nil {
				m.EmitEvent(root, Error, filePath, err)
				continue
			}
		}
	}
}
