package manikyr

import (
	"image"
	"io/ioutil"
	"path"
	"strings"
	"time"
	
	"github.com/disintegration/imaging"
)

func openImageWhenReady(file string) (image.Image, error) {
	// Retry opening the image until err != image.ErrFormat
	// or next retry would take over a minute.
	// FIXME
	
	var img image.Image
	var err error
	var retry int
	var t time.Duration

	for {
		t = time.Duration(1000 * (retry * 2))
		time.Sleep(time.Millisecond * t)

		img, err = imaging.Open(parentFile)
		if err == image.ErrFormat {
			retry = retry + 1
			if retry*2 > 60 {
				break
			}
			continue
		}
		return img, err
	}
	
}

func Subdirectories(root string) ([]string, error) {
	var dirs []string

	files, err := ioutil.ReadDir(root)
	if err != nil {
		return dirs, err
	}
	for _, file := range files {
		if file.IsDir() {
			dirs = append(dirs, file.Name())
		}
	}

	return dirs, nil
}

func NthSubdir(root, dir string, n int) (bool, error) {
	return path.Match(path.Join(root, "*" + strings.Repeat("/*", n)), dir)
}
