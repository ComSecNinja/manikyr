package manikyr

import (
  "io/ioutil"
  "path"
  "strings"
)

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

func NthSubdir(root, dir string, n int) bool {
	return path.Match(path.Join(root, "*" + strings.Repeat("/*", n)), dir)
}
