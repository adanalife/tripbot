package main

import (
  "fmt"
  "io/ioutil"
  "os"
  "path/filepath"
)

func getOpenfiles() (openfiles map[string]bool) {
    files, _ := ioutil.ReadDir("/proc")
    openfiles = make(map[string]bool)
    for _, f := range files {
        m, _ := filepath.Match("[0-9]*", f.Name())
        if f.IsDir() && m {
            fdpath := filepath.Join("/proc", f.Name(), "fd")
            ffiles, _ := ioutil.ReadDir(fdpath)
            for _, f := range ffiles {
                fpath, err := os.Readlink(filepath.Join(fdpath, f.Name()))
                if err != nil {
                    continue
                }
                openfiles[fpath] = true
            }
        }
    }
    return openfiles
}

func main() {
  fmt.Println(getOpenfiles())
}
