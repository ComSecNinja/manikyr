# manikyr: A thumb specialist
manikyr provides a sensible and filesystem structure agnostic image thumbnailing automation.

## Installation
`go get github.com/ComSecNinja/manikyr`

## Documentation
[godoc.org/github.com/ComSecNinja/manikyr](http://godoc.org/github.com/ComSecNinja/manikyr)

## Usage
The following example watches `/home/timo/picshur-test/` and any direct child directory (`/home/timo/picshur-test/*/`) for changes.  
If a new image file is created (by e.g. copying) in one of the direct child directories, manikyr automatically creates a thumbnail to `/home/timo/picshur-test/*/thumbs` with the same name as the original file.  
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

	// When we come across of a file, should we try to thumbnail it?
	mk.ShouldCreateThumb = func(root, file string) bool {
		ok, _ := manikyr.NthSubdir(root, file, 1)
		return ok
	}

	// Create chan to receive and print errors
	evtChan := make(chan manikyr.Event)
	go func(c chan manikyr.Event){
		for {
			evt := <-c
			println(evt.String())
		}
	}(evtChan)

	// Add our root directory which holds the gallery directories
	err = mk.AddRoot(myRoot, evtChan)
	if err != nil {
		panic(err)
	}

	// Watch and thumbnail existing files like they were 
	// added after the root got watched
	err = mk.Init(myRoot)
	if err != nil {
		panic(err)
	}

	// Block forever
	done := make(chan struct{})
	<-done
}
```
