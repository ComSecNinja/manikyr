# manikyr
Specializes in thumbs.

Automate image thumbnailing.  
Any root directory currently watched should **not** be removed or renamed.

## Installation
`go get github.com/ComSecNinja/manikyr`

## Usage
The next example watches `/home/timo/picshur/` and any direct child directory (`/home/timo/picshur/*/`) for changes.  
If a new image file is created (by e.g. copying) in one of the direct child directories, manikyr automatically creates a thumbnail to `/home/timo/picshur/*/thumb` with the same name as the original file.  
If a file is deleted in a direct child directory and a file with the same name is present in the designated thumbnail directory, it gets deleted.  
Deleting direct child directories automatically unwatches them so you do not need to worry about that.
```
package main

import (
	"path"
	"github.com/ComSecNinja/manikyr"
)

const myRoot = "/home/timo/picshur-test"

func main() {
	// Create a new manikyr.Manikyr instance
	mk := manikyr.New()

	// Thumbnail directory is {root}/{gallery}/thumbs
	mk.ThumbDirGetter = func(p string) string {
		return path.Join(path.Dir(p), "thumbs")
	}

	// Create chan to receive and print errors
	rootErrChan := make(chan error)

	// Add our root directory which holds the gallery directories
	err := mk.AddRoot(myRoot, rootErrChan)
	if err != nil {
		panic(err)
	}

	// Watch every visible subdirectory in our root
	subdirs, err := manikyr.Subdirectories(myRoot)
	if err != nil {
		panic(err)
	}
	for _, sd := range subdirs {
		if path.Base(sd)[0] != '.' { // Exclude hidden directories
			err := mk.AddSubdir(myRoot, sd)
			if err != nil {
				panic(err)
			}
		}
	}

	println("Manikyr ready)
	for {
		for {
			if err := <-rootErrChan; err != nil {
				println(err.Error())
			}
		}
	}
}
```
