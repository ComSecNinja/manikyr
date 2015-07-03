package manikyr

import (
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/go-fsnotify/fsnotify"
	"github.com/disintegration/imaging"
)

const thumbDir		= ".thumbs"
const thumbDirPerms	= 0777
const thumbWidth	= 100
const thumbHeight	= 100
const thumbAlgo		= imaging.NearestNeighbor

func removeThumb(filePath string) {
	thumbPath := path.Join(path.Dir(filePath), thumbDir, path.Base(filePath))
	err := os.Remove(thumbPath)
	if err != nil {
		log.Println(err.Error())
		return
	}
}

func createThumb(filePath string) {
	localThumbs := path.Join(path.Dir(filePath), thumbDir)
	thumbPath := path.Join(localThumbs, path.Base(filePath))

	img, err := imaging.Open(filePath)
	if err == image.ErrFormat {
		// There is a chance that the file is not yet completely created.
		// We need some sort of retry/wait functionality in here for production use.
		log.Println(err.Error())
		continue
	}
	if err != nil {
		log.Println(err.Error())
		continue
	}

	thumb := imaging.Thumbnail(img, thumbWidth, thumbHeight, thumbAlgo)

	_, err := os.Stat(localThumbs)
	if os.IsNotExist(err) {
		// Create a dir to hold thumbnails
		err := os.Mkdir(localThumbs, thumbDirPerms)
		if err != nil {
			log.Println(err.Error())
		}
	}

	// Save the thumbnail
	if err = imaging.Save(thumb, thumbPath); err != nil {
		log.Println(err.Error())
	}
}

func matchesSubpath(root, subpath, name string) bool {
	ok, err := path.Match(path.Join(root, subpath), name)
	if err != nil {
		log.Println(err.Error())
	}
	return ok
}

func Watch(root string) {
	// Create watcher
	w, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	} 
	defer w.Close()

	// Watch root dir
	err = w.Add(picshurRoot)
	if err != nil {
		panic(err)
	}

	// Watch subdirectories
	files, err := ioutil.ReadDir(root)
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		// "assets" contains scripts and styles for webpage
		// so we'll exclude that
		if file.IsDir() && file.Name != "assets" {
			err := w.Add(path.Join(root, file.Name()))
			if err != nil {
				panic(err)
			}
		}
	}

	// Event loop
	for {
		select {
		case evt := <- w.Events:
			if evt.Op == fsnotify.Create {
				// If a file was created

				// Get some info about the file
				info, err := os.Stat(evt.Name)
				if os.IsNotExist(err) {
					log.Println(err.Error())
					continue
				}

				switch mode := info.Mode(); {
				case mode.IsDir():
					// Watch the new dir if it's first-level
					if matchesSubpath(root, "*", evt.Name) {
						w.Add(evt.Name)
					}
				case mode.IsRegular():
					// Create thumbnail if the file is second-level regular
					if matchesSubpath(root, "*/*", evt.Name) {
						go createThumb(evt.Name)
					}
				}
			} else {
				// Something else happened to the file
				if _, err := os.Stat(evt.Name); os.IsNotExist(err) { // If file is gone
					// Try to delete thumb
					if matchesSubpath(root, "*/*", evt.Name) {
						go removeThumb(evt.Name)
					}
				}
			}
		case err := <- w.Errors:
			log.Println(err.Error())
		}
	}
}
