# manikyr: A thumb specialist
Automate image thumbnailing.  
Root directories should be unwatched prior to renaming or removing.

## Installation
`go get github.com/ComSecNinja/manikyr`

## Usage
The next example watches `/home/timo/picshur-test/` and any direct child directory (`/home/timo/picshur-test/*/`) for changes.  
If a new image file is created (by e.g. copying) in one of the direct child directories, manikyr automatically creates a thumbnail to `/home/timo/picshur-test/*/thumb` with the same name as the original file.  
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
	var err error
	mk := manikyr.New()

	// Thumbnail directory is $myRoot/../{parentDir}/thumbs
	mk.ThumbDirGetter = func(p string) string {
		return path.Join(path.Dir(p), "thumbs")
	}

	// When we bump into a subdir, should we watch it?
	mk.ShouldWatchSubdir = func(root, subdir string) bool {
		ok, _ := manikyr.NthSubdir(root, subdir, 0)
		if ok && subdir[0] != '.' && subdir != mk.ThumbDirGetter(subdir) {
			return true
		}
		return false
	}

	// When we bump into a file, should we try to thumbnail it?
	mk.ShouldCreateThumb = func(root, file string) bool {
		ok, _ := manikyr.NthSubdir(root, file, 1)
		return ok
	}

	// Create chan to receive and print errors
	rootErrChan := make(chan error)

	// Add our root directory which holds the gallery directories
	err = mk.AddRoot(myRoot, rootErrChan)
	if err != nil {
		panic(err)
	}

	// Watch and thumbnail existing files like they were 
	// added after the program started
	err = mk.Init(myRoot)
	if err != nil {
		panic(err)
	}

	println("Manikyr ready")
	for {
		if err := <-rootErrChan; err != nil {
			println(err.Error())
		}
	}
}
```
